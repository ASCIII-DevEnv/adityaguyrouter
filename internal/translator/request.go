package translator

import (
	"antigravity-gateway/internal/model"
	"antigravity-gateway/internal/skills"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Gemini Request Envelope for Antigravity CloudCode PA
type GeminiPart struct {
	Text             string                  `json:"text,omitempty"`
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	InlineData       *GeminiInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *GeminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
}

type GeminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64 encoded
}

type GeminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args,omitempty"`
	ID   string                 `json:"id,omitempty"`
}

type GeminiFunctionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type GeminiContent struct {
	Role  string       `json:"role"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiFunctionDeclaration struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type GeminiThinkingConfig struct {
	ThinkingBudget *int `json:"thinkingBudget,omitempty"`
}

type GeminiTool struct {
	FunctionDeclarations []GeminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
	GoogleSearch         map[string]interface{}      `json:"googleSearch,omitempty"`
}

type GeminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"` // "AUTO", "ANY", "NONE"
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type GeminiToolConfig struct {
	FunctionCallingConfig *GeminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type GeminiGenerationConfig struct {
	MaxOutputTokens *int                  `json:"maxOutputTokens,omitempty"`
	Temperature     *float64              `json:"temperature,omitempty"`
	TopP            *float64              `json:"topP,omitempty"`
	TopK            *int                  `json:"topK,omitempty"`
	StopSequences   []string              `json:"stopSequences,omitempty"`
	ThinkingConfig  *GeminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

type GeminiInnerRequest struct {
	Contents          []GeminiContent         `json:"contents"`
	SystemInstruction *GeminiContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *GeminiGenerationConfig `json:"generationConfig,omitempty"`
	Tools             []GeminiTool            `json:"tools,omitempty"`
	ToolConfig        *GeminiToolConfig       `json:"toolConfig,omitempty"`
}

type AntigravityEnvelope struct {
	Project   string             `json:"project"`
	Model     string             `json:"model"`
	UserAgent string             `json:"userAgent"`
	Request   GeminiInnerRequest `json:"request"`
}

// ConvertOpenAIToAntigravity translates OpenAI Chat request to CloudCode PA envelope
func ConvertOpenAIToAntigravity(projectID string, req *model.OpenAIChatRequest, skillPrompt string) *AntigravityEnvelope {
	var systemParts []string
	var contents []GeminiContent
	toolCallMap := make(map[string]string) // call_id -> function_name

	// Process messages
	for _, m := range req.Messages {
		if m.Role == "system" || m.Role == "developer" {
			txt := extractMessageContent(m.Content)
			if txt != "" {
				systemParts = append(systemParts, txt)
			}
			continue
		}

		if m.Role == "assistant" {
			var parts []GeminiPart
			txt := extractMessageContent(m.Content)
			if txt != "" && txt != "<nil>" {
				parts = append(parts, GeminiPart{Text: txt})
			}
			for _, tc := range m.ToolCalls {
				toolCallMap[tc.ID] = tc.Function.Name
				var args map[string]interface{}
				if tc.Function.Arguments != "" {
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				}
				if args == nil {
					args = make(map[string]interface{})
				}
				sig := GetThoughtSignature(tc.ID, tc.Function.Name)
				if sig == "" {
					sig = "skip_thought_signature_validator"
				}
				parts = append(parts, GeminiPart{
					ThoughtSignature: sig,
					FunctionCall: &GeminiFunctionCall{
						Name: tc.Function.Name,
						Args: args,
						ID:   tc.ID,
					},
				})
			}
			if len(parts) > 0 {
				contents = appendOrMergeContent(contents, "model", parts)
			}
			continue
		}

		if m.Role == "tool" {
			fnName := m.Name
			if fnName == "" && m.ToolCallID != "" {
				fnName = toolCallMap[m.ToolCallID]
			}
			if fnName == "" {
				fnName = "tool_response"
			}
			contentStr := extractMessageContent(m.Content)
			// Smart Context Compaction: Prune oversized historical tool responses (>2500 chars)
			contentStr = PruneToolOutput(contentStr, 2500)

			var respObj map[string]interface{}
			if err := json.Unmarshal([]byte(contentStr), &respObj); err != nil {
				respObj = map[string]interface{}{"output": contentStr}
			}
			part := GeminiPart{
				FunctionResponse: &GeminiFunctionResponse{
					Name:     fnName,
					Response: respObj,
				},
			}
			contents = appendOrMergeContent(contents, "user", []GeminiPart{part})
			continue
		}

		// User message with text and multi-modal support
		var parts []GeminiPart
		switch c := m.Content.(type) {
		case string:
			if c != "" {
				parts = append(parts, GeminiPart{Text: c})
			}
		case []interface{}:
			for _, item := range c {
				if block, ok := item.(map[string]interface{}); ok {
					bType, _ := block["type"].(string)
					switch bType {
					case "text":
						if txt, ok := block["text"].(string); ok && txt != "" {
							parts = append(parts, GeminiPart{Text: txt})
						}
					case "image_url":
						if img, ok := block["image_url"].(map[string]interface{}); ok {
							if u, ok := img["url"].(string); ok {
								if mime, data, ok := parseDataURL(u); ok {
									parts = append(parts, GeminiPart{
										InlineData: &GeminiInlineData{
											MimeType: mime,
											Data:     data,
										},
									})
								}
							}
						}
					}
				}
			}
		default:
			txt := extractMessageContent(m.Content)
			if txt != "" && txt != "<nil>" {
				parts = append(parts, GeminiPart{Text: txt})
			}
		}
		if len(parts) > 0 {
			contents = appendOrMergeContent(contents, "user", parts)
		}
	}

	mergedSystem := strings.Join(systemParts, "\n\n")
	if skillPrompt != "" {
		mergedSystem = skills.InjectSkill(skillPrompt, mergedSystem)
	}

	var sysInstruction *GeminiContent
	if mergedSystem != "" {
		sysInstruction = &GeminiContent{
			Role: "system",
			Parts: []GeminiPart{
				{Text: mergedSystem},
			},
		}
	}

	// Smart Tiered Model Resolution
	complexity := ClassifyPromptComplexity(contents, mergedSystem)
	modelID := ResolveModelWithComplexity(req.Model, complexity)

	var genConfig *GeminiGenerationConfig
	if req.Temperature != nil || req.MaxTokens != nil || req.TopP != nil || req.TopK != nil || req.Stop != nil || req.ThinkingBudget != nil {
		genConfig = &GeminiGenerationConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
			TopP:            req.TopP,
			TopK:            req.TopK,
		}
		if req.ThinkingBudget != nil {
			genConfig.ThinkingConfig = &GeminiThinkingConfig{
				ThinkingBudget: req.ThinkingBudget,
			}
		}
		if stops, ok := req.Stop.([]string); ok {
			genConfig.StopSequences = stops
		} else if stopStr, ok := req.Stop.(string); ok && stopStr != "" {
			genConfig.StopSequences = []string{stopStr}
		}
	}

	// Tools & Function Calling
	var geminiTools []GeminiTool
	if len(req.Tools) > 0 {
		var decls []GeminiFunctionDeclaration
		for _, t := range req.Tools {
			if t.Type == "function" || t.Type == "" {
				decls = append(decls, GeminiFunctionDeclaration{
					Name:        t.Function.Name,
					Description: t.Function.Description,
					Parameters:  sanitizeSchema(t.Function.Parameters),
				})
			}
		}
		if len(decls) > 0 {
			sort.Slice(decls, func(i, j int) bool {
				return decls[i].Name < decls[j].Name
			})
			geminiTools = append(geminiTools, GeminiTool{
				FunctionDeclarations: decls,
			})
		}
	}

	// Google Search Grounding support (via search_grounding boolean or model name containing 'search')
	isSearchRequested := (req.SearchGrounding != nil && *req.SearchGrounding) || strings.Contains(strings.ToLower(req.Model), "search")
	if isSearchRequested {
		geminiTools = append(geminiTools, GeminiTool{
			GoogleSearch: map[string]interface{}{},
		})
	}

	toolConfig := convertToolChoice(req.ToolChoice)

	return &AntigravityEnvelope{
		Project:   projectID,
		Model:     modelID,
		UserAgent: "antigravity/ide/2.11.0 darwin/arm64",
		Request: GeminiInnerRequest{
			Contents:          contents,
			SystemInstruction: sysInstruction,
			GenerationConfig:  genConfig,
			Tools:             geminiTools,
			ToolConfig:        toolConfig,
		},
	}
}

// ConvertAnthropicToAntigravity translates Anthropic Messages request to CloudCode PA envelope
func ConvertAnthropicToAntigravity(projectID string, req *model.AnthropicRequest, skillPrompt string) *AntigravityEnvelope {
	var systemText string
	if req.System != nil {
		systemText = extractMessageContent(req.System)
	}
	if skillPrompt != "" {
		systemText = skills.InjectSkill(skillPrompt, systemText)
	}

	var contents []GeminiContent
	for _, m := range req.Messages {
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}

		var parts []GeminiPart
		switch c := m.Content.(type) {
		case string:
			if c != "" {
				parts = append(parts, GeminiPart{Text: c})
			}
		case []interface{}:
			for _, item := range c {
				if block, ok := item.(map[string]interface{}); ok {
					bType, _ := block["type"].(string)
					switch bType {
					case "text":
						if txt, ok := block["text"].(string); ok && txt != "" {
							parts = append(parts, GeminiPart{Text: txt})
						}
					case "tool_use":
						name, _ := block["name"].(string)
						toolID, _ := block["id"].(string)
						var input map[string]interface{}
						if in, ok := block["input"].(map[string]interface{}); ok {
							input = in
						} else {
							input = make(map[string]interface{})
						}
						sig := GetThoughtSignature(toolID, name)
						if sig == "" {
							sig = "skip_thought_signature_validator"
						}
						parts = append(parts, GeminiPart{
							ThoughtSignature: sig,
							FunctionCall: &GeminiFunctionCall{
								Name: name,
								Args: input,
								ID:   toolID,
							},
						})
					case "tool_result":
						var respObj map[string]interface{}
						resContent := extractMessageContent(block["content"])
						// Smart Context Compaction: Prune oversized historical tool responses (>2500 chars)
						resContent = PruneToolOutput(resContent, 2500)

						if err := json.Unmarshal([]byte(resContent), &respObj); err != nil {
							respObj = map[string]interface{}{"output": resContent}
						}
						name, _ := block["tool_use_id"].(string)
						if name == "" {
							name = "tool_result"
						}
						parts = append(parts, GeminiPart{
							FunctionResponse: &GeminiFunctionResponse{
								Name:     name,
								Response: respObj,
							},
						})
					case "image":
						if src, ok := block["source"].(map[string]interface{}); ok {
							mediaType, _ := src["media_type"].(string)
							data, _ := src["data"].(string)
							if data != "" {
								parts = append(parts, GeminiPart{
									InlineData: &GeminiInlineData{
										MimeType: mediaType,
										Data:     data,
									},
								})
							}
						}
					}
				}
			}
		default:
			txt := extractMessageContent(m.Content)
			if txt != "" && txt != "<nil>" {
				parts = append(parts, GeminiPart{Text: txt})
			}
		}

		if len(parts) > 0 {
			contents = appendOrMergeContent(contents, role, parts)
		}
	}

	var sysInstruction *GeminiContent
	if systemText != "" {
		sysInstruction = &GeminiContent{
			Role: "system",
			Parts: []GeminiPart{
				{Text: systemText},
			},
		}
	}

	modelID := req.Model
	if modelID == "" {
		modelID = "claude-sonnet-4-6"
	} else {
		complexity := ClassifyPromptComplexity(contents, systemText)
		modelID = ResolveModelWithComplexity(modelID, complexity)
	}

	var maxTokens *int
	if req.MaxTokens > 0 {
		mt := req.MaxTokens
		maxTokens = &mt
	}

	// Tools
	var geminiTools []GeminiTool
	if len(req.Tools) > 0 {
		var decls []GeminiFunctionDeclaration
		for _, t := range req.Tools {
			decls = append(decls, GeminiFunctionDeclaration{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  sanitizeSchema(t.InputSchema),
			})
		}
		if len(decls) > 0 {
			sort.Slice(decls, func(i, j int) bool {
				return decls[i].Name < decls[j].Name
			})
			geminiTools = append(geminiTools, GeminiTool{
				FunctionDeclarations: decls,
			})
		}
	}

	// Google Search Grounding support (via search_grounding boolean or model name containing 'search')
	isSearchRequested := (req.SearchGrounding != nil && *req.SearchGrounding) || strings.Contains(strings.ToLower(req.Model), "search")
	if isSearchRequested {
		geminiTools = append(geminiTools, GeminiTool{
			GoogleSearch: map[string]interface{}{},
		})
	}

	toolConfig := convertToolChoice(req.ToolChoice)

	var thinkingCfg *GeminiThinkingConfig
	if req.ThinkingBudget != nil {
		thinkingCfg = &GeminiThinkingConfig{
			ThinkingBudget: req.ThinkingBudget,
		}
	}

	return &AntigravityEnvelope{
		Project:   projectID,
		Model:     modelID,
		UserAgent: "antigravity/ide/2.11.0 darwin/arm64",
		Request: GeminiInnerRequest{
			Contents:          contents,
			SystemInstruction: sysInstruction,
			GenerationConfig: &GeminiGenerationConfig{
				MaxOutputTokens: maxTokens,
				Temperature:     req.Temperature,
				TopP:            req.TopP,
				TopK:            req.TopK,
				StopSequences:   req.StopSequences,
				ThinkingConfig:  thinkingCfg,
			},
			Tools:      geminiTools,
			ToolConfig: toolConfig,
		},
	}
}

// PromptComplexity represents the evaluated prompt complexity tier
type PromptComplexity string

const (
	ComplexityLow    PromptComplexity = "low"
	ComplexityMedium PromptComplexity = "medium"
	ComplexityHigh   PromptComplexity = "high"
)

// ClassifyPromptComplexity evaluates contents and system instructions to determine prompt tier
func ClassifyPromptComplexity(contents []GeminiContent, systemText string) PromptComplexity {
	var totalLen int
	var codeBlockCount int
	var keywordScore int

	highKeywords := []string{
		"architect", "refactor", "optimize", "benchmark", "algorithm",
		"concurrency", "deadlock", "race condition", "memory leak",
		"distributed", "microservice", "security audit", "vulnerability",
		"complex", "implement full", "design pattern", "kubernetes",
		"explain in detail", "step by step reasoning", "mathematical proof",
		"docker-compose", "infrastructure", "deep dive", "migration",
	}

	mediumKeywords := []string{
		"debug", "fix bug", "function", "feature", "endpoint",
		"database", "query", "schema", "test", "docker", "component",
		"interface", "api", "explain", "how to", "compare", "summary",
	}

	allText := strings.ToLower(systemText)
	for _, c := range contents {
		for _, p := range c.Parts {
			txt := p.Text
			totalLen += len(txt)
			allText += " " + strings.ToLower(txt)
			codeBlockCount += strings.Count(txt, "```")
		}
	}

	for _, kw := range highKeywords {
		if strings.Contains(allText, kw) {
			keywordScore += 3
		}
	}
	for _, kw := range mediumKeywords {
		if strings.Contains(allText, kw) {
			keywordScore += 1
		}
	}

	// 1. High Complexity: Heavy context (> 3000 chars), multiple code blocks, or high keyword score
	if totalLen > 3000 || codeBlockCount >= 3 || keywordScore >= 5 || len(contents) >= 8 {
		return ComplexityHigh
	}

	// 2. Medium Complexity: Moderate context (600 - 3000 chars), at least one code block, or medium score
	if totalLen > 600 || codeBlockCount >= 1 || keywordScore >= 2 || len(contents) >= 4 {
		return ComplexityMedium
	}

	// 3. Low Complexity: Short Q&A, simple queries, greetings (< 600 chars)
	return ComplexityLow
}

// ResolveModelWithComplexity resolves user-requested model aliases to smart-tiered concrete model IDs
func ResolveModelWithComplexity(requestedModel string, complexity PromptComplexity) string {
	switch requestedModel {
	case "gemini-3.8-flash", "gemini-3.8-flash-tiered", "gemini-flash", "gemini-flash-auto":
		switch complexity {
		case ComplexityHigh:
			return "gemini-3.8-flash-high"
		case ComplexityMedium:
			return "gemini-3.8-flash-medium"
		default:
			return "gemini-3.8-flash-low"
		}
	case "gemini-3.7-flash", "gemini-3.7-flash-tiered":
		switch complexity {
		case ComplexityHigh:
			return "gemini-3.7-flash-high"
		case ComplexityMedium:
			return "gemini-3.7-flash-medium"
		default:
			return "gemini-3.7-flash-low"
		}
	case "gemini-3.6-flash", "gemini-3.6-flash-tiered":
		switch complexity {
		case ComplexityHigh:
			return "gemini-3.6-flash-high"
		case ComplexityMedium:
			return "gemini-3.6-flash-medium"
		default:
			return "gemini-3.6-flash-low"
		}
	case "gemini-3.1-pro", "gemini-3.1-pro-tiered", "gemini-pro-auto", "gemini-pro":
		if complexity == ComplexityHigh {
			return "gemini-3.1-pro-high"
		}
		return "gemini-3.1-pro-low"
	case "", "gpt-4", "gpt-4o", "gpt-3.5-turbo":
		return "gemini-2.5-pro"
	default:
		// Exact explicit model name provided by user (e.g. claude-sonnet-4-6, gemini-3.8-flash-high)
		return requestedModel
	}
}

func extractMessageContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var sb strings.Builder
		for _, part := range v {
			if m, ok := part.(map[string]interface{}); ok {
				if text, ok := m["text"].(string); ok {
					sb.WriteString(text)
				}
			}
		}
		return sb.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func appendOrMergeContent(contents []GeminiContent, role string, parts []GeminiPart) []GeminiContent {
	if len(parts) == 0 {
		return contents
	}
	if len(contents) > 0 && contents[len(contents)-1].Role == role {
		contents[len(contents)-1].Parts = append(contents[len(contents)-1].Parts, parts...)
		return contents
	}
	return append(contents, GeminiContent{
		Role:  role,
		Parts: parts,
	})
}

func parseDataURL(url string) (mimeType, data string, ok bool) {
	if !strings.HasPrefix(url, "data:") {
		return "", "", false
	}
	parts := strings.SplitN(url[5:], ";base64,", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func convertToolChoice(tc interface{}) *GeminiToolConfig {
	if tc == nil {
		return nil
	}
	switch v := tc.(type) {
	case string:
		switch strings.ToLower(v) {
		case "auto":
			return &GeminiToolConfig{FunctionCallingConfig: &GeminiFunctionCallingConfig{Mode: "AUTO"}}
		case "none":
			return &GeminiToolConfig{FunctionCallingConfig: &GeminiFunctionCallingConfig{Mode: "NONE"}}
		case "required", "any":
			return &GeminiToolConfig{FunctionCallingConfig: &GeminiFunctionCallingConfig{Mode: "ANY"}}
		}
	case map[string]interface{}:
		if fn, ok := v["function"].(map[string]interface{}); ok {
			if name, ok := fn["name"].(string); ok && name != "" {
				return &GeminiToolConfig{
					FunctionCallingConfig: &GeminiFunctionCallingConfig{
						Mode:                 "ANY",
						AllowedFunctionNames: []string{name},
					},
				}
			}
		}
		if tType, ok := v["type"].(string); ok {
			switch strings.ToLower(tType) {
			case "auto":
				return &GeminiToolConfig{FunctionCallingConfig: &GeminiFunctionCallingConfig{Mode: "AUTO"}}
			case "any":
				return &GeminiToolConfig{FunctionCallingConfig: &GeminiFunctionCallingConfig{Mode: "ANY"}}
			case "tool":
				if name, ok := v["name"].(string); ok && name != "" {
					return &GeminiToolConfig{
						FunctionCallingConfig: &GeminiFunctionCallingConfig{
							Mode:                 "ANY",
							AllowedFunctionNames: []string{name},
						},
					}
				}
			}
		}
	}
	return nil
}

func sanitizeSchema(schema interface{}) interface{} {
	if schema == nil {
		return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	}
	m, ok := schema.(map[string]interface{})
	if !ok {
		return schema
	}
	clean := make(map[string]interface{})
	for k, v := range m {
		if k == "$schema" || k == "additionalProperties" {
			continue
		}
		if k == "properties" {
			if props, isMap := v.(map[string]interface{}); isMap {
				cleanProps := make(map[string]interface{})
				for pk, pv := range props {
					cleanProps[pk] = sanitizeSchema(pv)
				}
				clean[k] = cleanProps
				continue
			}
		}
		if k == "items" {
			clean[k] = sanitizeSchema(v)
			continue
		}
		clean[k] = v
	}
	if _, hasType := clean["type"]; !hasType {
		clean["type"] = "object"
	}
	return clean
}
