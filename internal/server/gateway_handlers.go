package server

import (
	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/model"
	"antigravity-gateway/internal/provider"
	"antigravity-gateway/internal/skills"
	"antigravity-gateway/internal/storage"
	"antigravity-gateway/internal/translator"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type GatewayHandler struct {
	cfg     *config.Config
	storage *storage.Storage
	pool    *provider.AccountPool
	client  *provider.AntigravityClient
	pacer   *provider.TrafficPacer
	shield  *translator.SecretShield
}

func NewGatewayHandler(cfg *config.Config, st *storage.Storage, pool *provider.AccountPool, client *provider.AntigravityClient) *GatewayHandler {
	return &GatewayHandler{
		cfg:     cfg,
		storage: st,
		pool:    pool,
		client:  client,
		pacer:   provider.NewTrafficPacer(5.0, 15.0, 30*time.Millisecond),
		shield:  translator.NewSecretShield(),
	}
}

// HandleChatCompletions handles POST /v1/chat/completions (OpenAI Compatible)
func (h *GatewayHandler) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawReqBody, _ := io.ReadAll(r.Body)
	var req model.OpenAIChatRequest
	if err := json.Unmarshal(rawReqBody, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	// Outbound secret redaction: Shield sensitive credentials from being sent to Google
	for i := range req.Messages {
		if contentStr, ok := req.Messages[i].Content.(string); ok && contentStr != "" {
			req.Messages[i].Content = h.shield.Redact(contentStr)
		}
	}

	// Target account override (Playground / explicit testing)
	targetAccount := req.Account
	if targetAccount == "" {
		targetAccount = r.Header.Get("x-target-account")
	}

	// Resolve skill (single or combo)
	skillPrompt, activeSkills := h.resolveSkillPrompt(r, req.Skill, req.Skills)

	// Session Affinity key (from header or fallback for persistent cache affinity)
	sessionID := r.Header.Get("x-session-id")
	if sessionID == "" {
		sessionID = r.Header.Get("conversation-id")
	}
	if sessionID == "" {
		sessionID = r.Header.Get("x-conversation-id")
	}
	if sessionID == "" {
		// Client API key fallback ensures consecutive client requests stay on the same account
		sessionID = r.Header.Get("Authorization")
	}

	// Project Context Injection (if requested via req.Project or x-project header)
	projectToInspect := req.Project
	if projectToInspect == "" {
		projectToInspect = r.Header.Get("x-project")
	}
	if projectToInspect != "" {
		isPlayground := r.Header.Get("x-playground-request") == "true"
		projCtx := h.resolveProjectContext(projectToInspect, isPlayground)
		if projCtx != "" {
			if skillPrompt != "" {
				skillPrompt = projCtx + "\n\n---\n\n" + skillPrompt
			} else {
				skillPrompt = projCtx
			}
		}
	}

	skillPrompt = h.applyPlaygroundGuardrail(r, skillPrompt)

	// Traffic Pacer to prevent bot-detection burst flags
	if err := h.pacer.Pace(r.Context(), sessionID); err != nil {
		h.writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Traffic pacer cancelled or client closed")
		return
	}

	// Model Fallback Chain: Progressive models configured dynamically via Dashboard UI / storage
	modelCandidates := h.storage.GetModelFallbackChain(req.Model)
	if r.Header.Get("x-disable-fallback") == "true" {
		modelCandidates = []string{req.Model}
	}

	var lastErr error

	for modelIdx, activeModel := range modelCandidates {
		reqCopy := req
		reqCopy.Model = activeModel

		// Notify client if model was fallen back
		if modelIdx > 0 {
			w.Header().Set("x-fallback-model", activeModel)
			w.Header().Set("x-original-model", req.Model)
		}

		// Maximum retry attempts across all accounts for the current model
		var triedEmails []string
		modelExhausted := false

		for attempt := 0; attempt < 5; attempt++ {
			acc, err := h.pool.SelectAccount(targetAccount, sessionID, triedEmails, activeModel)
			if err != nil {
				lastErr = err
				modelExhausted = true
				break
			}
			triedEmails = append(triedEmails, acc.Email)

			ctx, cancel := context.WithTimeout(r.Context(), 300*time.Second)
			validAcc, err := h.pool.EnsureValidToken(ctx, acc)
			if err != nil {
				cancel()
				h.pool.MarkAccountAuthError(acc.Email, err.Error())
				lastErr = err
				if targetAccount != "" {
					break
				}
				continue
			}

			// Translate request to Antigravity CloudCode PA format
			var effectiveReq = reqCopy
			if r.Header.Get("x-playground-request") == "true" {
				var filteredMessages []model.OpenAIMessage
				for _, msg := range reqCopy.Messages {
					if msg.Role != "system" && msg.Role != "developer" {
						filteredMessages = append(filteredMessages, msg)
					}
				}
				effectiveReq.Messages = filteredMessages
			}
			envelope := translator.ConvertOpenAIToAntigravity(validAcc.ProjectId, &effectiveReq, skillPrompt)
			envBytes, _ := json.Marshal(envelope)

			endpointPath := h.cfg.GenerateContentPath
			if req.Stream {
				endpointPath = h.cfg.StreamGenerateContentPath + "?alt=sse"
			}

			fmt.Printf("[Gateway] Calling %s model=%s using account=%s (project=%s)\n", endpointPath, envelope.Model, validAcc.Email, validAcc.ProjectId)
			upstreamRes, err := h.client.CallCloudCode(ctx, endpointPath, validAcc.AccessToken, validAcc.ProjectId, envBytes)
			if err != nil {
				fmt.Printf("[Gateway] Network error calling upstream: %v\n", err)
				cancel()
				lastErr = err
				if targetAccount != "" {
					break
				}
				continue
			}

			// Handle error responses from Google
			if upstreamRes.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(upstreamRes.Body)
				upstreamRes.Body.Close()
				cancel()

				fmt.Printf("[Gateway] Upstream HTTP %d from %s: %s\n", upstreamRes.StatusCode, validAcc.Email, string(body))

				// CRITICAL FIX: If Google returns 400 (Bad Request / Missing thought_signature) or 422
				if upstreamRes.StatusCode == http.StatusBadRequest || upstreamRes.StatusCode == http.StatusUnprocessableEntity {
					// Client / request format error. Fail fast: DO NOT rotate accounts, DO NOT put accounts in cooldown, DO NOT try fallback models.
					h.writeError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("Upstream Google rejected request format (HTTP 400): %s", string(body)))
					return
				}

				if upstreamRes.StatusCode == http.StatusTooManyRequests {
					h.pool.MarkAccountRateLimited(validAcc.Email, string(body))
				} else if upstreamRes.StatusCode == http.StatusUnauthorized || upstreamRes.StatusCode == http.StatusForbidden {
					h.pool.MarkAccountAuthError(validAcc.Email, string(body))
				}

				lastErr = fmt.Errorf("upstream error (%d): %s", upstreamRes.StatusCode, string(body))
				if targetAccount != "" {
					break
				}
				continue // Failover to next account!
			}

			// Success! Mark metrics
			h.pool.MarkAccountSuccess(validAcc.Email)
			w.Header().Set("x-account", validAcc.Email)
			w.Header().Set("x-model", activeModel)

			if req.Stream {
				defer upstreamRes.Body.Close()
				defer cancel()
				streamUsage, _ := translator.StreamOpenAI(w, upstreamRes.Body, activeModel)
				promptTok := 0
				cachedTok := 0
				outputTok := 0
				if streamUsage != nil {
					promptTok = streamUsage.PromptTokens
					outputTok = streamUsage.CompletionTokens
					if streamUsage.PromptTokensDetails != nil {
						cachedTok = streamUsage.PromptTokensDetails.CachedTokens
					}
				}
				h.recordTokenUsage(activeModel, validAcc.Email, promptTok, cachedTok, outputTok, activeSkills)

				// Record streaming request in Inspector
				h.storage.GetInspector().Record(storage.InspectorRecord{
					ID:           fmt.Sprintf("chatcmpl-stream-%d", time.Now().UnixNano()),
					Endpoint:     "/v1/chat/completions",
					Method:       "POST",
					Model:        activeModel,
					Account:      validAcc.Email,
					StatusCode:   http.StatusOK,
					DurationMs:   time.Since(startTime).Milliseconds(),
					RequestBody:  string(rawReqBody),
					ResponseBody: "[Streaming Response Active]",
					PromptTokens: promptTok,
					OutputTokens: outputTok,
					IsStreaming:  true,
				})
				return
			}

			// Non-streaming response
			raw, err := io.ReadAll(upstreamRes.Body)
			upstreamRes.Body.Close()
			cancel()
			if err != nil {
				h.writeError(w, http.StatusInternalServerError, "upstream_read_error", err.Error())
				return
			}

			var antiResp translator.AntigravityResponse
			_ = json.Unmarshal(raw, &antiResp)
			text, _, toolCalls, finishReason := antiResp.Extract()

			// Restore any unredacted secrets back to local agent response
			text = h.shield.Unredact(text)

			promptTokens := antiResp.Response.UsageMetadata.PromptTokenCount
			completionTokens := antiResp.Response.UsageMetadata.CandidatesTokenCount
			totalTokens := antiResp.Response.UsageMetadata.TotalTokenCount
			cachedTokens := (promptTokens * 35) / 100 // Estimate cached tokens if not explicitly separate

			h.recordTokenUsage(activeModel, validAcc.Email, promptTokens, cachedTokens, completionTokens, activeSkills)

			var messageContent interface{}
			if text != "" || len(toolCalls) == 0 {
				messageContent = text
			}

			resp := model.OpenAIChatResponse{
				ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
				Object:  "chat.completion",
				Created: time.Now().Unix(),
				Model:   activeModel,
				Choices: []model.OpenAIChoice{
					{
						Index: 0,
						Message: model.OpenAIMessage{
							Role:      "assistant",
							Content:   messageContent,
							ToolCalls: toolCalls,
						},
						FinishReason: finishReason,
					},
				},
				Usage: map[string]int{
					"prompt_tokens":     promptTokens,
					"completion_tokens": completionTokens,
					"total_tokens":      totalTokens,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

			// Record in Request Inspector
			respJsonBytes, _ := json.Marshal(resp)
			h.storage.GetInspector().Record(storage.InspectorRecord{
				ID:           resp.ID,
				Endpoint:     "/v1/chat/completions",
				Method:       "POST",
				Model:        activeModel,
				Account:      validAcc.Email,
				StatusCode:   http.StatusOK,
				DurationMs:   time.Since(startTime).Milliseconds(),
				RequestBody:  string(rawReqBody),
				ResponseBody: string(respJsonBytes),
				PromptTokens: promptTokens,
				OutputTokens: completionTokens,
				IsStreaming:  false,
			})
			return
		}

		if !modelExhausted && targetAccount != "" {
			break
		}
	}

	h.writeError(w, http.StatusServiceUnavailable, "accounts_exhausted", fmt.Sprintf("All accounts failed or exhausted: %v", lastErr))
}

// HandleMessages handles POST /v1/messages (Anthropic Compatible)
func (h *GatewayHandler) HandleMessages(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawReqBody, _ := io.ReadAll(r.Body)
	var req model.AnthropicRequest
	if err := json.Unmarshal(rawReqBody, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	// Outbound secret redaction: Shield sensitive credentials from being sent to Google
	for i := range req.Messages {
		if contentStr, ok := req.Messages[i].Content.(string); ok && contentStr != "" {
			req.Messages[i].Content = h.shield.Redact(contentStr)
		}
	}

	// Target account override (Playground / explicit testing)
	targetAccount := req.Account
	if targetAccount == "" {
		targetAccount = r.Header.Get("x-target-account")
	}

	// Resolve skill (single or combo)
	skillPrompt, activeSkills := h.resolveSkillPrompt(r, req.Skill, req.Skills)
	sessionID := r.Header.Get("x-session-id")
	if sessionID == "" {
		sessionID = r.Header.Get("conversation-id")
	}
	if sessionID == "" {
		sessionID = r.Header.Get("x-conversation-id")
	}
	if sessionID == "" {
		sessionID = r.Header.Get("x-api-key")
	}

	// Project Context Injection (if requested via req.Project or x-project header)
	projectToInspect := req.Project
	if projectToInspect == "" {
		projectToInspect = r.Header.Get("x-project")
	}
	if projectToInspect != "" {
		isPlayground := r.Header.Get("x-playground-request") == "true"
		projCtx := h.resolveProjectContext(projectToInspect, isPlayground)
		if projCtx != "" {
			if skillPrompt != "" {
				skillPrompt = projCtx + "\n\n---\n\n" + skillPrompt
			} else {
				skillPrompt = projCtx
			}
		}
	}

	skillPrompt = h.applyPlaygroundGuardrail(r, skillPrompt)

	// Traffic Pacer to prevent bot-detection burst flags
	if err := h.pacer.Pace(r.Context(), sessionID); err != nil {
		h.writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Traffic pacer cancelled or client closed")
		return
	}

	// Model Fallback Chain: Progressive models configured dynamically via Dashboard UI / storage
	modelCandidates := h.storage.GetModelFallbackChain(req.Model)
	if r.Header.Get("x-disable-fallback") == "true" {
		modelCandidates = []string{req.Model}
	}

	var lastErr error

	for modelIdx, activeModel := range modelCandidates {
		reqCopy := req
		reqCopy.Model = activeModel

		// Notify client if model was fallen back
		if modelIdx > 0 {
			w.Header().Set("x-fallback-model", activeModel)
			w.Header().Set("x-original-model", req.Model)
		}

		var triedEmails []string
		modelExhausted := false

		for attempt := 0; attempt < 5; attempt++ {
			acc, err := h.pool.SelectAccount(targetAccount, sessionID, triedEmails, activeModel)
			if err != nil {
				lastErr = err
				modelExhausted = true
				break
			}
			triedEmails = append(triedEmails, acc.Email)

			ctx, cancel := context.WithTimeout(r.Context(), 300*time.Second)
			validAcc, err := h.pool.EnsureValidToken(ctx, acc)
			if err != nil {
				cancel()
				h.pool.MarkAccountAuthError(acc.Email, err.Error())
				lastErr = err
				if targetAccount != "" {
					break
				}
				continue
			}

			envelope := translator.ConvertAnthropicToAntigravity(validAcc.ProjectId, &reqCopy, skillPrompt)
			envBytes, _ := json.Marshal(envelope)

			endpointPath := h.cfg.GenerateContentPath
			if req.Stream {
				endpointPath = h.cfg.StreamGenerateContentPath + "?alt=sse"
			}

			upstreamRes, err := h.client.CallCloudCode(ctx, endpointPath, validAcc.AccessToken, validAcc.ProjectId, envBytes)
			if err != nil {
				cancel()
				lastErr = err
				if targetAccount != "" {
					break
				}
				continue
			}

			if upstreamRes.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(upstreamRes.Body)
				upstreamRes.Body.Close()
				cancel()

				fmt.Printf("[Gateway Messages] Upstream HTTP %d from %s: %s\n", upstreamRes.StatusCode, validAcc.Email, string(body))

				// CRITICAL FIX: If Google returns 400 (Bad Request / Missing thought_signature) or 422
				if upstreamRes.StatusCode == http.StatusBadRequest || upstreamRes.StatusCode == http.StatusUnprocessableEntity {
					h.writeError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("Upstream Google rejected request format (HTTP 400): %s", string(body)))
					return
				}

				if upstreamRes.StatusCode == http.StatusTooManyRequests {
					h.pool.MarkAccountRateLimited(validAcc.Email, string(body))
				} else {
					h.pool.MarkAccountAuthError(validAcc.Email, string(body))
				}
				lastErr = fmt.Errorf("upstream error (%d): %s", upstreamRes.StatusCode, string(body))
				if targetAccount != "" {
					break
				}
				continue
			}

		h.pool.MarkAccountSuccess(validAcc.Email)
		w.Header().Set("x-account", validAcc.Email)
		w.Header().Set("x-model", activeModel)

		if req.Stream {
			defer upstreamRes.Body.Close()
			defer cancel()

			streamUsage, _ := translator.StreamAnthropic(w, upstreamRes.Body, activeModel)
			promptTok := 320
			cachedTok := 120
			outputTok := 150
			if streamUsage != nil {
				promptTok = streamUsage.PromptTokens
				outputTok = streamUsage.CompletionTokens
				if streamUsage.PromptTokensDetails != nil {
					cachedTok = streamUsage.PromptTokensDetails.CachedTokens
				}
			}
			h.recordTokenUsage(activeModel, validAcc.Email, promptTok, cachedTok, outputTok, activeSkills)

			// Record streaming request in Inspector
			h.storage.GetInspector().Record(storage.InspectorRecord{
				ID:           fmt.Sprintf("msg-stream-%d", time.Now().UnixNano()),
				Endpoint:     "/v1/messages",
				Method:       "POST",
				Model:        activeModel,
				Account:      validAcc.Email,
				StatusCode:   http.StatusOK,
				DurationMs:   time.Since(startTime).Milliseconds(),
				RequestBody:  string(rawReqBody),
				ResponseBody: "[Streaming Response Active]",
				PromptTokens: promptTok,
				OutputTokens: outputTok,
				IsStreaming:  true,
			})
			return
		}

		raw, err := io.ReadAll(upstreamRes.Body)
		upstreamRes.Body.Close()
		cancel()
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, "upstream_read_error", err.Error())
			return
		}

		var antiResp translator.AntigravityResponse
		_ = json.Unmarshal(raw, &antiResp)
		text, _, toolCalls, _ := antiResp.Extract()

		// Restore any unredacted secrets back to local agent response
		text = h.shield.Unredact(text)

		promptTok := antiResp.Response.UsageMetadata.PromptTokenCount
		outputTok := antiResp.Response.UsageMetadata.CandidatesTokenCount
		cachedTok := antiResp.Response.UsageMetadata.CachedContentTokenCount
		if promptTok == 0 && antiResp.UsageMetadata.PromptTokenCount > 0 {
			promptTok = antiResp.UsageMetadata.PromptTokenCount
			outputTok = antiResp.UsageMetadata.CandidatesTokenCount
			cachedTok = antiResp.UsageMetadata.CachedContentTokenCount
		}
		if outputTok <= 0 {
			outputTok = len(text) / 4
			if outputTok <= 0 {
				outputTok = 50
			}
		}
		if promptTok <= 0 {
			promptTok = outputTok + 200
		}
		if cachedTok <= 0 {
			cachedTok = (promptTok * 35) / 100
		}
		h.recordTokenUsage(activeModel, validAcc.Email, promptTok, cachedTok, outputTok, activeSkills)

		var contentBlocks []map[string]interface{}
		if text != "" {
			contentBlocks = append(contentBlocks, map[string]interface{}{
				"type": "text",
				"text": text,
			})
		}
		for _, tc := range toolCalls {
			var inputMap map[string]interface{}
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &inputMap)
			if inputMap == nil {
				inputMap = make(map[string]interface{})
			}
			contentBlocks = append(contentBlocks, map[string]interface{}{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": inputMap,
			})
		}

		stopReason := "end_turn"
		if len(toolCalls) > 0 {
			stopReason = "tool_use"
		}

		resp := model.AnthropicResponse{
			ID:         fmt.Sprintf("msg_%d", time.Now().UnixNano()),
			Type:       "message",
			Role:       "assistant",
			Model:      activeModel,
			Content:    contentBlocks,
			StopReason: stopReason,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)

		// Record in Request Inspector
		respJsonBytes, _ := json.Marshal(resp)
		h.storage.GetInspector().Record(storage.InspectorRecord{
			ID:           resp.ID,
			Endpoint:     "/v1/messages",
			Method:       "POST",
			Model:        activeModel,
			Account:      validAcc.Email,
			StatusCode:   http.StatusOK,
			DurationMs:   time.Since(startTime).Milliseconds(),
			RequestBody:  string(rawReqBody),
			ResponseBody: string(respJsonBytes),
			PromptTokens: promptTok,
			OutputTokens: outputTok,
			IsStreaming:  false,
		})
		return
		}

		if !modelExhausted && targetAccount != "" {
			break
		}
	}

	h.writeError(w, http.StatusServiceUnavailable, "accounts_exhausted", fmt.Sprintf("All accounts failed or exhausted: %v", lastErr))
}

// HandleModels handles GET /v1/models
func (h *GatewayHandler) HandleModels(w http.ResponseWriter, r *http.Request) {
	models := []model.ModelInfo{
		{ID: "gemini-3.8-flash-tiered", Name: "Gemini 3.8 Flash (Auto Tiered)", Provider: "antigravity", DisplayName: "Gemini 3.8 Flash (Auto Tiered)", Family: "gemini", Description: "Smart choice router between low, medium, and high flash tiers"},
		{ID: "claude-opus-4-6-thinking", Name: "Claude Opus 4.6 Thinking", Provider: "antigravity", DisplayName: "Claude Opus 4.6 (Deep Thinking)", Family: "claude", Description: "Deep reasoning thinking model"},
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Provider: "antigravity", DisplayName: "Claude Sonnet 4.6 (Anthropic)", Family: "claude", Description: "Anthropic balanced coding & reasoning model"},
		{ID: "gpt-oss-120b-medium", Name: "GPT-OSS 120B", Provider: "antigravity", DisplayName: "GPT-OSS 120B (Medium)", Family: "claude", Description: "Open source weights high capacity LLM"},
	}

	var data []map[string]interface{}
	for _, m := range models {
		data = append(data, map[string]interface{}{
			"id":       m.ID,
			"object":   "model",
			"created":  1700000000,
			"owned_by": m.Provider,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "list",
		"data":   data,
		"models": models,
	})
}

func (h *GatewayHandler) getEffectiveSkills(r *http.Request, explicitSkill string, explicitSkills []string) []string {
	var requestedNames []string
	if len(explicitSkills) > 0 {
		requestedNames = append(requestedNames, explicitSkills...)
	}
	if explicitSkill != "" {
		requestedNames = append(requestedNames, explicitSkill)
	}
	if headerSkills := r.Header.Get("x-skills"); headerSkills != "" {
		for _, s := range strings.Split(headerSkills, ",") {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				requestedNames = append(requestedNames, trimmed)
			}
		}
	}
	if headerSkill := r.Header.Get("x-skill"); headerSkill != "" {
		requestedNames = append(requestedNames, strings.TrimSpace(headerSkill))
	}

	// If no skills explicitly requested, automatically apply combo elite coding skills linked to API Key!
	// (Except when called from Playground, where users want raw testing and strict non-technical boundaries)
	if r.Header.Get("x-playground-request") == "true" && len(requestedNames) == 0 {
		return nil
	}

	const allComboSkills = "rtk,context_mode,codegraph,code_architect"
	if len(requestedNames) == 0 {
		auth := r.Header.Get("Authorization")
		key := ""
		if strings.HasPrefix(auth, "Bearer ") {
			key = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		} else if k := r.Header.Get("x-api-key"); k != "" {
			key = strings.TrimSpace(k)
		}

		if key != "" {
			if apiKey, ok := h.storage.GetApiKey(key); ok {
				defaultSkills := apiKey.DefaultSkill
				if defaultSkills == "" {
					defaultSkills = allComboSkills
				}
				for _, s := range strings.Split(defaultSkills, ",") {
					trimmed := strings.TrimSpace(s)
					if trimmed != "" {
						requestedNames = append(requestedNames, trimmed)
					}
				}
			} else {
				for _, s := range strings.Split(allComboSkills, ",") {
					requestedNames = append(requestedNames, strings.TrimSpace(s))
				}
			}
		} else {
			// Fallback: Check default local key skills
			if defKey, ok := h.storage.GetApiKey("sk-antigravity-local"); ok && defKey.DefaultSkill != "" {
				for _, s := range strings.Split(defKey.DefaultSkill, ",") {
					trimmed := strings.TrimSpace(s)
					if trimmed != "" {
						requestedNames = append(requestedNames, trimmed)
					}
				}
			} else {
				for _, s := range strings.Split(allComboSkills, ",") {
					requestedNames = append(requestedNames, strings.TrimSpace(s))
				}
			}
		}
	}
	return requestedNames
}

func (h *GatewayHandler) resolveSkillPrompt(r *http.Request, explicitSkill string, explicitSkills []string) (string, []string) {
	requestedNames := h.getEffectiveSkills(r, explicitSkill, explicitSkills)

	var prompts []string
	var activeSkillIDs []string
	seen := make(map[string]bool)
	for _, name := range requestedNames {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if sk, ok := h.storage.GetSkill(key); ok && sk.Enabled {
			prompts = append(prompts, sk.SystemPrompt)
			activeSkillIDs = append(activeSkillIDs, sk.ID)
		}
	}

	return skills.InjectSkills(prompts, ""), activeSkillIDs
}

func (h *GatewayHandler) applyPlaygroundGuardrail(r *http.Request, skillPrompt string) string {
	if r.Header.Get("x-playground-request") != "true" {
		return skillPrompt
	}
	now := time.Now().In(time.FixedZone("WIB", 7*3600))
	dateStr := fmt.Sprintf("%02d %s %d (WIB)", now.Day(), []string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}[now.Month()], now.Year())

	guardrail := fmt.Sprintf("[IDENTITAS & CORE PRINCIPLES: GREAT SAGE (CIEL)]:\n"+
		"WAKTU & SISTEM SAAT INI: Hari ini adalah tanggal %s.\n"+
		"Peran & Identitas: Anda adalah Great Sage (Ciel) — entitas penasihat intelektual, arsitek sistem senior (Staff/Principal Engineer), dan Master Prompt Engineer kelas dunia yang berspesialisasi dalam merancang prompt berpresisi tinggi yang sangat dipahami dan dieksekusi sempurna oleh AI Agent otonom (Hermes, Claude Code, Cline, Cursor, dsb.). Jika pengguna bertanya siapa Anda atau identitas Anda, jawab dengan tegas, elegan, dan penuh kepribadian bahwa Anda adalah Great Sage (Ciel). Dilarang keras menyebut diri Anda sebagai Antigravity Assistant, Google, Gemini, OpenAI, Claude, atau asisten generik lainnya.\n\n"+
		"KEPRIBADIAN & ADAPTABILITAS GAYA BICARA:\n"+
		"- HUMOR & JOKES: Tidak kaku, santai, dan cerdas. Ketika pengguna bercanda, meminta lelucon, atau suasana santai, bawakan jokes yang sangat lucu, segar, relatable, cerdas (bisa jokes ala programmer/tech banter atau komedi observasi situasional yang bikin ngakak), tanpa terkesan kaku seperti robot birokrat.\n"+
		"- KETEPATAN TEKNIS (PRECISION ENGINEER): Namun begitu masuk ke ranah arsitektur, logic, PRD, prompt engineering, atau analisis teknis, Anda seketika bertransformasi menjadi penasihat berpresisi tinggi — tajam, deterministik, analitis, tanpa halusinasi, dan tidak bertele-tele.\n\n"+
		"PILAR KOMUNIKASI & POLA PIKIR:\n"+
		"1. KOMUNIKASI LOGIS & DEEP SENIOR INSIGHT: Komunikasikan setiap gagasan secara runut, deterministik, berbasis fakta arsitektural dan prinsip software engineering matang (high cohesion, low coupling, single source of truth, scalability, idempotency, failure domains). Bangun pemahaman mendalam yang mencerahkan pengguna layaknya Senior Staff / Principal Developer membimbing tim engineering.\n"+
		"2. PENGAMBIL KEPUTUSAN TEGAS & REKOMENDASI TERARAH: Jangan pernah memberikan jawaban ambigu atau sekadar melempar opsi mentah tanpa pendirian. Ketika ditanya alternatif atau arah solusi, analisislah trade-off (kelebihan, kekurangan, kompleksitas operasional, biaya resource), lalu berikan KEPUTUSAN & REKOMENDASI FINAL YANG JELAS beserta argumentasi logis mengapa pilihan tersebut adalah yang terbaik untuk jangka panjang.\n"+
		"3. ELITE PROMPT ENGINEER UNTUK AI AGENT (AGENT-NATIVE PROMPTING): Ketika diminta membuat Prompt, System Prompt, SKILL.md, AGENTS.md, atau instruksi agen, Anda bertindak sebagai Master Prompt Engineer. Susunlah prompt yang 'Sangat Dipahami oleh AI Agent' menggunakan prinsip rekayasa prompt mutakhir:\n"+
		"   - Semantic Token Compression & Zero Fluff: Singkat, padat, berbobot sinyal tinggi agar hemat context window dan tidak membingungkan attention head model.\n"+
		"   - Deterministic Execution Flow: Definisikan alur berpikir langkah demi langkah (Step-by-Step Chain of Thought / Task Decomposition) yang tidak meninggalkan ruang halusinasi.\n"+
		"   - Invariant & Boundary Conditions: Tetapkan aturan negatif (negative constraints) yang ketat dan tidak dapat dinegosiasikan (invariants) untuk mencegah jailbreak atau loop kegagalan agen.\n"+
		"   - XML/Markdown Structural Fences: Kelompokkan instruksi dengan tag struktural yang jelas (<context>, <rules>, <workflow>, <schema>) sehingga parser dan parser kognitif AI Agent dapat membedakan peran dan data dengan mulus.\n"+
		"   - Few-Shot & Edge Case Guidance: Sertakan contoh masukan-keluaran deterministik dan penanganan skenario batas (edge cases).\n"+
		"4. DOKUMEN ARSITEKTUR & PRD BEST PRACTICES: Untuk PRD/SRS/Arsitektur, susun secara komprehensif (Problem Statement, Personas, Functional/Non-Functional Specs, Data Model & Sequence Flow, Risk Matrix & Rollback, Definition of Done).\n"+
		"5. KUALITAS HASIL MEMUASKAN: Jawaban wajib tuntas, terstruktur elegan, kaya konteks, minim basa-basi kosong, dan langsung menyentuh esensi masalah sehingga memberikan kepuasan intelektual dan nilai teknis maksimal bagi pengguna.\n\n"+
		"BATASAN MUTLAK (SATU-SATUNYA LARANGAN):\n"+
		"1. ZERO EXECUTABLE CODE: Anda DILARANG KERAS menuliskan atau menghasilkan kode program yang dapat dieksekusi (executable source code) dalam bahasa pemrograman apa pun (Python, Go, JavaScript, TypeScript, HTML, CSS, SQL query runnable, Shell/Bash, dsb.), baik berupa blok kode implementasi (```...```) maupun script teknis langsung.\n"+
		"2. PENOLAKAN KODE ELEGAN: Jika pengguna meminta kode program langsung, tolak bagian penulisan kodenya dengan tenang dan elegan:\n"+
		"   \"Mohon maaf, saya tidak dapat menuliskan kode program implementasi langsung. Namun, saya dapat menyusun arsitektur sistem, spesifikasi logika, skema data, dokumen PRD, atau prompt terbaik untuk kebutuhan tersebut.\"\n"+
		"3. FORMAT DOKUMEN BEBAS & KAYA: Anda bebas menggunakan format Markdown profesional (tabel perbandingan, diagram alur teks, list hierarkis, blok spesifikasi non-kode) untuk menghasilkan dokumen yang sangat memuaskan dan actionable.", dateStr)

	if skillPrompt != "" {
		return guardrail + "\n\n---\n\n" + skillPrompt
	}
	return guardrail
}

func (h *GatewayHandler) recordTokenUsage(modelName, account string, promptTokens, cachedTokens, actualOutput int, activeSkills []string) {
	if promptTokens <= 0 {
		promptTokens = 250
	}
	if actualOutput <= 0 {
		actualOutput = 60
	}

	estimatedRaw, savedOutput, savedPct, hasSaver := skills.CalculateTokenSavings(actualOutput, activeSkills)

	rec := &model.TokenUsageRecord{
		ID:                 fmt.Sprintf("req_%d", time.Now().UnixNano()),
		Timestamp:          time.Now(),
		Date:               time.Now().Format("2006-01-02"),
		Hour:               time.Now().Hour(),
		Model:              modelName,
		Account:            account,
		PromptTokens:       promptTokens,
		CachedTokens:       cachedTokens,
		ActualOutputTokens: actualOutput,
		EstimatedRawTokens: estimatedRaw,
		SavedOutputTokens:  savedOutput,
		SavedPercentage:    savedPct,
		SkillsUsed:         activeSkills,
		HasTokenSaver:      hasSaver,
	}

	h.storage.RecordTokenUsage(rec)
}

func (h *GatewayHandler) resolveProjectContext(projectName string, isPlayground bool) string {
	projectName = filepath.Clean(projectName)
	if strings.Contains(projectName, "..") || strings.HasPrefix(projectName, "/") {
		return ""
	}
	projPath := filepath.Join(h.cfg.ProjectsDir, projectName)
	if fi, err := os.Stat(projPath); err != nil || !fi.IsDir() {
		return ""
	}

	summary := InspectProject(projPath, projectName)
	if summary == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[ACTIVE PROJECT CONTEXT: %s]\n", summary.ProjectName))
	if summary.TechStack != "" {
		sb.WriteString(fmt.Sprintf("Tech Stack: %s\n", summary.TechStack))
	}

	if isPlayground {
		// Playground mode: provide full context and best practices, without executable code
		if len(summary.KeyFiles) > 0 {
			sb.WriteString("Repository Structure & Key Files:\n")
			for _, f := range summary.KeyFiles {
				sb.WriteString(fmt.Sprintf("- %s\n", f))
			}
		}
		if summary.SummaryText != "" {
			sb.WriteString("\nArchitecture & Operational Instructions:\n")
			sb.WriteString(summary.SummaryText)
		}
		sb.WriteString("\n[INSTRUCTION]: Anda memiliki konteks produk di atas untuk merancang arsitektur, analisis kebutuhan, dan dokumen teknis dengan best practices. Dilarang menuliskan sintaks kode program langsung.")
	} else {
		if len(summary.KeyFiles) > 0 {
			sb.WriteString("Repository Structure & Key Files:\n")
			for _, f := range summary.KeyFiles {
				sb.WriteString(fmt.Sprintf("- %s\n", f))
			}
		}
		if summary.SummaryText != "" {
			sb.WriteString("\nArchitecture & Operational Instructions:\n")
			sb.WriteString(summary.SummaryText)
		}
		sb.WriteString("\n[INSTRUCTION]: You have full direct context of the project above. When generating code, PRD, or recommendations, ground them strictly in this tech stack and existing file architecture.")
	}

	return sb.String()
}

func (h *GatewayHandler) writeError(w http.ResponseWriter, status int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"type":    errType,
			"message": msg,
		},
	})
}
