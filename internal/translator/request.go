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
	ID       string                 `json:"id,omitempty"`
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
			// Smart Context Compaction: Prune oversized historical tool responses (>50,000 chars) while preserving JSON structure
			respObj := parseAndPruneToolResponse(contentStr, 50000)

			part := GeminiPart{
				FunctionResponse: &GeminiFunctionResponse{
					Name:     fnName,
					Response: respObj,
					ID:       m.ToolCallID,
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

	toolCallMap := make(map[string]string)
	for _, m := range req.Messages {
		if blocks, ok := m.Content.([]interface{}); ok {
			for _, item := range blocks {
				if block, ok := item.(map[string]interface{}); ok {
					if block["type"] == "tool_use" {
						id, _ := block["id"].(string)
						name, _ := block["name"].(string)
						if id != "" && name != "" {
							toolCallMap[id] = name
						}
					}
				}
			}
		}
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
						if toolID != "" && name != "" {
							toolCallMap[toolID] = name
						}
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
						resContent := extractMessageContent(block["content"])
						// Smart Context Compaction: Prune oversized historical tool responses (>50,000 chars) while preserving JSON structure
						respObj := parseAndPruneToolResponse(resContent, 50000)

						toolUseID, _ := block["tool_use_id"].(string)
						fnName := toolCallMap[toolUseID]
						if fnName == "" {
							if n, ok := block["name"].(string); ok && n != "" {
								fnName = n
							} else if toolUseID != "" {
								fnName = toolUseID
							} else {
								fnName = "tool_result"
							}
						}
						parts = append(parts, GeminiPart{
							FunctionResponse: &GeminiFunctionResponse{
								Name:     fnName,
								Response: respObj,
								ID:       toolUseID,
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
		return "gemini-3.8-flash-tiered"
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

// parseAndPruneToolResponse safely parses JSON responses and avoids breaking JSON structure
func parseAndPruneToolResponse(contentStr string, maxChars int) map[string]interface{} {
	var respObj map[string]interface{}
	if err := json.Unmarshal([]byte(contentStr), &respObj); err == nil {
		// Valid JSON structure. Check and prune oversized inner string fields
		for k, v := range respObj {
			if s, ok := v.(string); ok && len(s) > maxChars {
				respObj[k] = PruneToolOutput(s, maxChars)
			}
		}
		return respObj
	}
	// Plain text: prune if exceeds maxChars
	pruned := PruneToolOutput(contentStr, maxChars)
	return map[string]interface{}{"output": pruned}
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
	return sanitizeSchemaInternal(schema, true)
}

func sanitizeSchemaInternal(schema interface{}, isRoot bool) interface{} {
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
		if k == "type" {
			if typeList, isList := v.([]interface{}); isList {
				var nonNullTypes []string
				isNullable := false
				for _, t := range typeList {
					if str, ok := t.(string); ok {
						if str == "null" {
							isNullable = true
						} else {
							nonNullTypes = append(nonNullTypes, str)
						}
					}
				}
				if isNullable {
					clean["nullable"] = true
				}
				if len(nonNullTypes) > 0 {
					clean["type"] = nonNullTypes[0]
				} else {
					clean["type"] = "string"
				}
				continue
			} else if typeStrList, isStrList := v.([]string); isStrList {
				var nonNullTypes []string
				isNullable := false
				for _, str := range typeStrList {
					if str == "null" {
						isNullable = true
					} else {
						nonNullTypes = append(nonNullTypes, str)
					}
				}
				if isNullable {
					clean["nullable"] = true
				}
				if len(nonNullTypes) > 0 {
					clean["type"] = nonNullTypes[0]
				} else {
					clean["type"] = "string"
				}
				continue
			} else if typeStr, isStr := v.(string); isStr && typeStr == "null" {
				clean["nullable"] = true
				clean["type"] = "string"
				continue
			}
		}
		if k == "properties" {
			if props, isMap := v.(map[string]interface{}); isMap {
				cleanProps := make(map[string]interface{})
				for pk, pv := range props {
					cleanProps[pk] = sanitizeSchemaInternal(pv, false)
				}
				clean[k] = cleanProps
				continue
			}
		}
		if k == "items" {
			if itemMap, isMap := v.(map[string]interface{}); isMap {
				clean[k] = sanitizeSchemaInternal(itemMap, false)
				continue
			} else if itemList, isList := v.([]interface{}); isList {
				var cleanList []interface{}
				for _, it := range itemList {
					cleanList = append(cleanList, sanitizeSchemaInternal(it, false))
				}
				clean[k] = cleanList
				continue
			}
		}
		// Convert anyOf to oneOf everywhere (Anthropic Claude strictly requires oneOf for unions)
		if k == "anyOf" {
			if list, isList := v.([]interface{}); isList {
				var cleanBranches []interface{}
				for _, branch := range list {
					cleanBranches = append(cleanBranches, sanitizeSchemaInternal(branch, false))
				}
				clean["oneOf"] = cleanBranches
				continue
			}
		}
		if k == "oneOf" || k == "allOf" {
			if list, isList := v.([]interface{}); isList {
				var cleanBranches []interface{}
				for _, branch := range list {
					cleanBranches = append(cleanBranches, sanitizeSchemaInternal(branch, false))
				}
				clean[k] = cleanBranches
				continue
			}
		}
		clean[k] = v
	}


	// Combinator detection
	_, hasAnyOf := clean["anyOf"]
	_, hasOneOf := clean["oneOf"]
	_, hasAllOf := clean["allOf"]
	_, hasRef := clean["$ref"]
	hasCombinator := hasAnyOf || hasOneOf || hasAllOf || hasRef

	if _, hasType := clean["type"]; !hasType {
		if isRoot {
			clean["type"] = "object"
			if _, hasProps := clean["properties"]; !hasProps {
				clean["properties"] = map[string]interface{}{}
			}
		} else if _, hasProps := clean["properties"]; hasProps {
			clean["type"] = "object"
		} else if !hasCombinator {
			clean["type"] = "object"
		}
	} else if isRoot && clean["type"] == "object" {
		if _, hasProps := clean["properties"]; !hasProps {
			clean["properties"] = map[string]interface{}{}
		}
	}
	return clean
}

