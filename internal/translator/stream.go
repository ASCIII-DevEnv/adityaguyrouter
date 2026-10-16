package translator

import (
	"antigravity-gateway/internal/model"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	thoughtSigMu        sync.RWMutex
	thoughtSigByID      = make(map[string]string)
	thoughtSigByName    = make(map[string]string)
	lastKnownThoughtSig string
)

func SetThoughtSignature(callID, funcName, sig string) {
	if sig == "" {
		return
	}
	thoughtSigMu.Lock()
	defer thoughtSigMu.Unlock()
	lastKnownThoughtSig = sig
	if callID != "" {
		thoughtSigByID[callID] = sig
	}
	if funcName != "" {
		thoughtSigByName[funcName] = sig
	}
}

func GetThoughtSignature(callID, funcName string) string {
	thoughtSigMu.RLock()
	defer thoughtSigMu.RUnlock()
	if callID != "" {
		if sig, ok := thoughtSigByID[callID]; ok && sig != "" {
			return sig
		}
	}
	if funcName != "" {
		if sig, ok := thoughtSigByName[funcName]; ok && sig != "" {
			return sig
		}
	}
	if lastKnownThoughtSig != "" {
		return lastKnownThoughtSig
	}
	// Google Gemini 3 official sentinel value to bypass validation when signature is absent
	return "skip_thought_signature_validator"
}

// Antigravity Candidate Response Structures
type AntigravityCandidatePart struct {
	Text             string                 `json:"text"`
	Thought          *bool                  `json:"thought,omitempty"`
	ThoughtSignature string                 `json:"thoughtSignature,omitempty"`
	ThoughtSigSnake  string                 `json:"thought_signature,omitempty"`
	FunctionCall     *struct {
		ID   string                 `json:"id,omitempty"`
		Name string                 `json:"name"`
		Args map[string]interface{} `json:"args"`
	} `json:"functionCall,omitempty"`
}

type AntigravityCandidate struct {
	Content struct {
		Parts []AntigravityCandidatePart `json:"parts"`
		Role  string                     `json:"role"`
	} `json:"content"`
	FinishReason string `json:"finishReason"`
}

type AntigravityResponse struct {
	Response struct {
		Candidates    []AntigravityCandidate `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount        int `json:"promptTokenCount"`
			CandidatesTokenCount    int `json:"candidatesTokenCount"`
			TotalTokenCount         int `json:"totalTokenCount"`
			CachedContentTokenCount int `json:"cachedContentTokenCount"`
		} `json:"usageMetadata"`
	} `json:"response"`
	Candidates    []AntigravityCandidate `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		TotalTokenCount         int `json:"totalTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
	} `json:"usageMetadata"`
}

func (r *AntigravityResponse) GetCandidates() []AntigravityCandidate {
	if len(r.Response.Candidates) > 0 {
		return r.Response.Candidates
	}
	return r.Candidates
}

func (r *AntigravityResponse) Extract() (text string, reasoningText string, toolCalls []model.OpenAIToolCall, finishReason string) {
	candidates := r.GetCandidates()
	if len(candidates) == 0 {
		return "", "", nil, "stop"
	}

	c := candidates[0]
	var sb strings.Builder
	var rsb strings.Builder
	for _, p := range c.Content.Parts {
		if p.FunctionCall != nil {
			argsBytes, _ := json.Marshal(p.FunctionCall.Args)
			idx := len(toolCalls)
			callID := p.FunctionCall.ID
			if callID == "" {
				callID = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), idx)
			}
			sig := p.ThoughtSignature
			if sig == "" {
				sig = p.ThoughtSigSnake
			}
			if sig != "" {
				SetThoughtSignature(callID, p.FunctionCall.Name, sig)
			}
			toolCalls = append(toolCalls, model.OpenAIToolCall{
				Index: &idx,
				ID:    callID,
				Type:  "function",
				Function: model.OpenAIToolCallFunction{
					Name:      p.FunctionCall.Name,
					Arguments: string(argsBytes),
				},
			})
		} else if p.Thought != nil && *p.Thought {
			rsb.WriteString(p.Text)
		} else if p.Text != "" {
			sb.WriteString(p.Text)
		}
	}

	finishReason = "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	} else if c.FinishReason != "" {
		finishReason = mapFinishReason(c.FinishReason)
	}

	text = sb.String()
	reasoningText = rsb.String()
	if text == "" && reasoningText != "" {
		text = reasoningText
	}

	return text, reasoningText, toolCalls, finishReason
}

func (r *AntigravityResponse) ExtractText() (string, string) {
	text, _, _, finishReason := r.Extract()
	return text, finishReason
}

func mapFinishReason(fr string) string {
	switch strings.ToUpper(fr) {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "BLOCKLIST", "RECITATION":
		return "content_filter"
	case "FUNCTION_CALL":
		return "tool_calls"
	default:
		return strings.ToLower(fr)
	}
}

// StreamOpenAI converts upstream SSE stream from Antigravity to OpenAI SSE format
func StreamOpenAI(w http.ResponseWriter, upstream io.Reader, modelName string) (*model.OpenAIUsage, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	scanner := bufio.NewScanner(upstream)
	// Allocate 1MB initial buffer up to 16MB max line length to prevent ErrTooLong on big chunks/metadata
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 16*1024*1024)

	reqID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	hasToolCalls := false
	var lastUsage *model.OpenAIUsage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if dataStr == "" || dataStr == "[DONE]" {
			continue
		}

		var antiResp AntigravityResponse
		if err := json.Unmarshal([]byte(dataStr), &antiResp); err != nil {
			continue
		}

		// Track token usage reported by Google CloudCode
		um := antiResp.Response.UsageMetadata
		if um.PromptTokenCount == 0 && antiResp.UsageMetadata.PromptTokenCount > 0 {
			um = antiResp.UsageMetadata
		}
		if um.PromptTokenCount > 0 || um.CandidatesTokenCount > 0 {
			lastUsage = &model.OpenAIUsage{
				PromptTokens:     um.PromptTokenCount,
				CompletionTokens: um.CandidatesTokenCount,
				TotalTokens:      um.TotalTokenCount,
			}
			if um.CachedContentTokenCount > 0 {
				lastUsage.PromptTokensDetails = &model.PromptTokensDetails{
					CachedTokens: um.CachedContentTokenCount,
				}
			}
		}

		text, reasoningText, toolCalls, finishReason := antiResp.Extract()
		if text == "" && reasoningText == "" && len(toolCalls) == 0 && finishReason == "" {
			continue
		}

		if len(toolCalls) > 0 {
			hasToolCalls = true
		}

		var reasonPtr *string
		if finishReason != "" && finishReason != "stop" {
			reasonPtr = &finishReason
		}

		delta := model.OpenAIStreamDelta{}
		if text != "" {
			delta.Content = text
		}
		if reasoningText != "" {
			delta.ReasoningContent = reasoningText
		}
		if len(toolCalls) > 0 {
			delta.ToolCalls = toolCalls
		}

		chunk := model.OpenAIStreamChunk{
			ID:      reqID,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   modelName,
			Choices: []model.OpenAIStreamChoice{
				{
					Index:        0,
					Delta:        delta,
					FinishReason: reasonPtr,
				},
			},
		}

		chunkBytes, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
		flusher.Flush()
	}

	// Final stop chunk
	finalStop := "stop"
	if hasToolCalls {
		finalStop = "tool_calls"
	}
	finalChunk := model.OpenAIStreamChunk{
		ID:      reqID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   modelName,
		Choices: []model.OpenAIStreamChoice{
			{
				Index:        0,
				Delta:        model.OpenAIStreamDelta{},
				FinishReason: &finalStop,
			},
		},
		Usage: lastUsage,
	}
	chunkBytes, _ := json.Marshal(finalChunk)
	fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	return lastUsage, scanner.Err()
}

// StreamAnthropic converts upstream SSE stream from Antigravity to Anthropic SSE format
func StreamAnthropic(w http.ResponseWriter, upstream io.Reader, modelName string) (*model.OpenAIUsage, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	var lastUsage *model.OpenAIUsage

	// 1. message_start
	fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", fmt.Sprintf(`{"type":"message_start","message":{"id":"%s","type":"message","role":"assistant","model":"%s","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}`, msgID, modelName))
	// 2. content_block_start
	fmt.Fprintf(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
	flusher.Flush()

	scanner := bufio.NewScanner(upstream)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 16*1024*1024)

	blockIdx := 0
	hasToolUse := false

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if dataStr == "" || dataStr == "[DONE]" {
			continue
		}

		var antiResp AntigravityResponse
		if err := json.Unmarshal([]byte(dataStr), &antiResp); err != nil {
			continue
		}

		// Track token usage reported by Google CloudCode
		um := antiResp.Response.UsageMetadata
		if um.PromptTokenCount == 0 && antiResp.UsageMetadata.PromptTokenCount > 0 {
			um = antiResp.UsageMetadata
		}
		if um.PromptTokenCount > 0 || um.CandidatesTokenCount > 0 {
			lastUsage = &model.OpenAIUsage{
				PromptTokens:     um.PromptTokenCount,
				CompletionTokens: um.CandidatesTokenCount,
				TotalTokens:      um.TotalTokenCount,
			}
			if um.CachedContentTokenCount > 0 {
				lastUsage.PromptTokensDetails = &model.PromptTokensDetails{
					CachedTokens: um.CachedContentTokenCount,
				}
			}
		}

		text, reasoningText, toolCalls, _ := antiResp.Extract()
		if text != "" {
			escText, _ := json.Marshal(text)
			fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":%s}}\n\n", string(escText))
			flusher.Flush()
		}
		if reasoningText != "" {
			escThought, _ := json.Marshal(reasoningText)
			fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":%s}}\n\n", string(escThought))
			flusher.Flush()
		}

		for _, tc := range toolCalls {
			hasToolUse = true
			blockIdx++
			// content_block_start for tool_use
			fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"tool_use","id":"%s","name":"%s","input":{}}}`, blockIdx, tc.ID, tc.Function.Name))
			// content_block_delta with json
			escArgs, _ := json.Marshal(tc.Function.Arguments)
			fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":%d,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":%s}}\n\n", blockIdx, string(escArgs))
			// content_block_stop
			fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", blockIdx)
			flusher.Flush()
		}
	}

	stopReason := "end_turn"
	if hasToolUse {
		stopReason = "tool_use"
	}

	outTok := 20
	if lastUsage != nil && lastUsage.CompletionTokens > 0 {
		outTok = lastUsage.CompletionTokens
	}
	// 3. content_block_stop & message_delta & message_stop
	fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
	fmt.Fprintf(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"%s\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":%d}}\n\n", stopReason, outTok)
	fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	flusher.Flush()

	return lastUsage, scanner.Err()
}
