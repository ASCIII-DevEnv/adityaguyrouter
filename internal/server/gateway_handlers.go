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
		pacer:   provider.NewTrafficPacer(0.5, 5.0, 150*time.Millisecond),
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

			ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
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
			envelope := translator.ConvertOpenAIToAntigravity(validAcc.ProjectId, &reqCopy, skillPrompt)
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

			if req.Stream {
				defer upstreamRes.Body.Close()
				defer cancel()
				h.recordTokenUsage(activeModel, validAcc.Email, 320, 120, 150, activeSkills)

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
					PromptTokens: 320,
					OutputTokens: 150,
					IsStreaming:  true,
				})

				if err := translator.StreamOpenAI(w, upstreamRes.Body, activeModel); err != nil {
					return
				}
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

			ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
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

		if req.Stream {
			defer upstreamRes.Body.Close()
			defer cancel()
			h.recordTokenUsage(activeModel, validAcc.Email, 320, 120, 150, activeSkills)

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
				PromptTokens: 320,
				OutputTokens: 150,
				IsStreaming:  true,
			})

			if err := translator.StreamAnthropic(w, upstreamRes.Body, activeModel); err != nil {
				return
			}
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

		outputTok := len(text) / 4
		if outputTok <= 0 {
			outputTok = 50
		}
		promptTok := outputTok + 200
		cachedTok := (promptTok * 35) / 100
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

	// If no skills explicitly requested, automatically apply combo 5 skills linked to API Key!
	const allComboSkills = "rtk,caveman,ponytail,context_mode,codegraph"
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
