package translator

import (
	"antigravity-gateway/internal/model"
	"strings"
	"testing"
)

func TestConvertOpenAIToAntigravity(t *testing.T) {
	req := &model.OpenAIChatRequest{
		Model: "gemini-3.8-flash-tiered",
		Messages: []model.OpenAIMessage{
			{Role: "system", Content: "You are an assistant."},
			{Role: "user", Content: "Halo world"},
		},
	}

	env := ConvertOpenAIToAntigravity("proj-123", req, "Caveman speak.")
	if env.Project != "proj-123" {
		t.Fatalf("expected proj-123, got %s", env.Project)
	}

	if env.Request.SystemInstruction == nil {
		t.Fatalf("expected system instruction to be populated")
	}

	sysText := env.Request.SystemInstruction.Parts[0].Text
	if !strings.Contains(sysText, "Caveman speak") {
		t.Fatalf("expected caveman skill in system prompt, got: %s", sysText)
	}

	if len(env.Request.Contents) != 1 {
		t.Fatalf("expected 1 user content turn, got %d", len(env.Request.Contents))
	}
	if env.Request.Contents[0].Parts[0].Text != "Halo world" {
		t.Fatalf("content mismatch")
	}
}

func TestConvertAnthropicToAntigravity(t *testing.T) {
	req := &model.AnthropicRequest{
		Model: "claude-sonnet-4-6",
		Messages: []model.AnthropicMessage{
			{Role: "user", Content: "Test Anthropic"},
		},
		MaxTokens: 1024,
	}

	env := ConvertAnthropicToAntigravity("proj-456", req, "Ponytail lazy.")
	if env.Project != "proj-456" {
		t.Fatalf("expected proj-456, got %s", env.Project)
	}
	if env.Model != "claude-sonnet-4-6" {
		t.Fatalf("expected claude-sonnet-4-6, got %s", env.Model)
	}
	if env.Request.GenerationConfig == nil || *env.Request.GenerationConfig.MaxOutputTokens != 1024 {
		t.Fatalf("generation config max output tokens mismatch")
	}
}

func TestSmartTieredResolution(t *testing.T) {
	// Simple query -> low tier
	lowContents := []GeminiContent{
		{Role: "user", Parts: []GeminiPart{{Text: "Halo, apa kabar?"}}},
	}
	lowComp := ClassifyPromptComplexity(lowContents, "")
	if lowComp != ComplexityLow {
		t.Fatalf("expected ComplexityLow, got %s", lowComp)
	}
	if resolved := ResolveModelWithComplexity("gemini-3.8-flash-tiered", lowComp); resolved != "gemini-3.8-flash-low" {
		t.Fatalf("expected gemini-3.8-flash-low, got %s", resolved)
	}

	// Medium query with code and debugging
	medContents := []GeminiContent{
		{Role: "user", Parts: []GeminiPart{{Text: "Tolong debug function ini:\n```js\nfunction add(a, b) { return a + b; }\n```\nKenapa terjadi error saat dipanggil?"}}},
	}
	medComp := ClassifyPromptComplexity(medContents, "")
	if medComp != ComplexityMedium {
		t.Fatalf("expected ComplexityMedium, got %s", medComp)
	}
	if resolved := ResolveModelWithComplexity("gemini-3.8-flash-tiered", medComp); resolved != "gemini-3.8-flash-medium" {
		t.Fatalf("expected gemini-3.8-flash-medium, got %s", resolved)
	}

	// High query with architecture and complex keywords
	highContents := []GeminiContent{
		{Role: "user", Parts: []GeminiPart{{Text: "Tolong rancang software architect untuk microservice distributed system dengan zero deadlock, handle concurrency tinggi, dan kubernetes deployment. Berikan analisis mendalam dan benchmark perbandingan."}}},
	}
	highComp := ClassifyPromptComplexity(highContents, "")
	if highComp != ComplexityHigh {
		t.Fatalf("expected ComplexityHigh, got %s", highComp)
	}
	if resolved := ResolveModelWithComplexity("gemini-3.8-flash-tiered", highComp); resolved != "gemini-3.8-flash-high" {
		t.Fatalf("expected gemini-3.8-flash-high, got %s", resolved)
	}
}

func TestConvertOpenAIToolCalling(t *testing.T) {
	req := &model.OpenAIChatRequest{
		Model: "gemini-3.8-flash-tiered",
		Messages: []model.OpenAIMessage{
			{Role: "user", Content: "List directory contents"},
			{
				Role: "assistant",
				ToolCalls: []model.OpenAIToolCall{
					{
						ID:   "call_123",
						Type: "function",
						Function: model.OpenAIToolCallFunction{
							Name:      "list_directory",
							Arguments: `{"path":"."}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				ToolCallID: "call_123",
				Name:       "list_directory",
				Content:    `file1.txt\nfile2.txt`,
			},
		},
		Tools: []model.OpenAITool{
			{
				Type: "function",
				Function: model.OpenAIToolFunction{
					Name:        "list_directory",
					Description: "List files in directory",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"path": map[string]interface{}{"type": "string"},
						},
					},
				},
			},
		},
	}

	env := ConvertOpenAIToAntigravity("proj-test", req, "")
	if len(env.Request.Tools) != 1 || len(env.Request.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("expected 1 function declaration in tools")
	}
	if env.Request.Tools[0].FunctionDeclarations[0].Name != "list_directory" {
		t.Fatalf("tool declaration name mismatch")
	}

	if len(env.Request.Contents) != 3 {
		t.Fatalf("expected 3 contents turns (user, model, user), got %d", len(env.Request.Contents))
	}
	if env.Request.Contents[1].Role != "model" || env.Request.Contents[1].Parts[0].FunctionCall == nil {
		t.Fatalf("expected model function call in turn 1")
	}
	if env.Request.Contents[2].Role != "user" || env.Request.Contents[2].Parts[0].FunctionResponse == nil {
		t.Fatalf("expected user function response in turn 2")
	}
}

func TestConvertAnthropicToolCalling(t *testing.T) {
	req := &model.AnthropicRequest{
		Model: "claude-sonnet-4-6",
		Messages: []model.AnthropicMessage{
			{
				Role: "assistant",
				Content: []interface{}{
					map[string]interface{}{
						"type": "tool_use",
						"id":   "toolu_123",
						"name": "read_file",
						"input": map[string]interface{}{
							"path": "main.go",
						},
					},
				},
			},
			{
				Role: "user",
				Content: []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "toolu_123",
						"content":     "package main",
					},
				},
			},
		},
		Tools: []model.AnthropicTool{
			{
				Name:        "read_file",
				Description: "Read file",
				InputSchema: map[string]interface{}{"type": "object"},
			},
		},
	}

	env := ConvertAnthropicToAntigravity("proj-test", req, "")
	if len(env.Request.Tools) != 1 || len(env.Request.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("expected 1 tool declaration")
	}
	if env.Request.Contents[0].Role != "model" || env.Request.Contents[0].Parts[0].FunctionCall == nil {
		t.Fatalf("expected functionCall in assistant turn")
	}
	if env.Request.Contents[1].Role != "user" || env.Request.Contents[1].Parts[0].FunctionResponse == nil {
		t.Fatalf("expected functionResponse in user turn")
	}
	if env.Request.Contents[1].Parts[0].FunctionResponse.Name != "read_file" {
		t.Fatalf("expected functionResponse.Name to be read_file, got %s", env.Request.Contents[1].Parts[0].FunctionResponse.Name)
	}
	if env.Request.Contents[1].Parts[0].FunctionResponse.ID != "toolu_123" {
		t.Fatalf("expected functionResponse.ID to be toolu_123, got %s", env.Request.Contents[1].Parts[0].FunctionResponse.ID)
	}
}

func TestThinkingBudgetAndGoogleSearchGrounding(t *testing.T) {
	budget := 4096
	search := true

	// 1. OpenAI request with budget & search grounding
	reqOpenAI := &model.OpenAIChatRequest{
		Model:           "gemini-3.8-flash-search",
		Messages:        []model.OpenAIMessage{{Role: "user", Content: "Berita hari ini"}},
		ThinkingBudget:  &budget,
		SearchGrounding: &search,
	}

	env1 := ConvertOpenAIToAntigravity("proj-1", reqOpenAI, "")
	if env1.Request.GenerationConfig == nil || env1.Request.GenerationConfig.ThinkingConfig == nil {
		t.Fatalf("expected thinkingConfig in generationConfig")
	}
	if *env1.Request.GenerationConfig.ThinkingConfig.ThinkingBudget != 4096 {
		t.Fatalf("expected budget 4096, got %d", *env1.Request.GenerationConfig.ThinkingConfig.ThinkingBudget)
	}

	// Verify GoogleSearch tool present
	hasGoogleSearch := false
	for _, tl := range env1.Request.Tools {
		if tl.GoogleSearch != nil {
			hasGoogleSearch = true
			break
		}
	}
	if !hasGoogleSearch {
		t.Fatalf("expected GoogleSearch tool enabled")
	}

	// 2. Anthropic request with thinking budget
	reqAnthropic := &model.AnthropicRequest{
		Model:          "claude-opus-4-6-thinking",
		Messages:       []model.AnthropicMessage{{Role: "user", Content: "Analisis arsitektur"}},
		ThinkingBudget: &budget,
	}

	env2 := ConvertAnthropicToAntigravity("proj-2", reqAnthropic, "")
	if env2.Request.GenerationConfig == nil || env2.Request.GenerationConfig.ThinkingConfig == nil {
		t.Fatalf("expected thinkingConfig in Anthropic generationConfig")
	}
}

func TestSanitizeSchemaDraft202012(t *testing.T) {
	rawSchema := map[string]interface{}{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"operations": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"anyOf": []interface{}{
						map[string]interface{}{
							"type":                 "object",
							"additionalProperties": false,
							"properties": map[string]interface{}{
								"action": map[string]interface{}{"type": "string"},
							},
						},
					},
				},
			},
			"notify": map[string]interface{}{
				"description": "Union parameter without type",
				"anyOf": []interface{}{
					map[string]interface{}{"type": "boolean"},
					map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
		},
	}

	sanitized := sanitizeSchema(rawSchema).(map[string]interface{})
	if _, ok := sanitized["$schema"]; ok {
		t.Fatalf("expected $schema to be removed")
	}
	if _, ok := sanitized["additionalProperties"]; ok {
		t.Fatalf("expected top-level additionalProperties to be removed")
	}

	props := sanitized["properties"].(map[string]interface{})
	ops := props["operations"].(map[string]interface{})
	items := ops["items"].(map[string]interface{})

	if _, hasType := items["type"]; hasType {
		t.Fatalf("items should not have type: object forced onto combinators")
	}
	if _, hasAnyOf := items["anyOf"]; hasAnyOf {
		t.Fatalf("items anyOf should be converted to oneOf")
	}
	oneOfList, hasOneOf := items["oneOf"].([]interface{})
	if !hasOneOf || len(oneOfList) != 1 {
		t.Fatalf("expected items to have oneOf with 1 branch")
	}
	branch := oneOfList[0].(map[string]interface{})
	if _, ok := branch["additionalProperties"]; ok {
		t.Fatalf("branch additionalProperties should be removed")
	}

	notify := props["notify"].(map[string]interface{})
	if _, hasType := notify["type"]; hasType {
		t.Fatalf("notify with anyOf should not have type: object forced onto it")
	}
	if _, hasAnyOf := notify["anyOf"]; hasAnyOf {
		t.Fatalf("notify anyOf should be converted to oneOf")
	}
	if _, hasOneOf := notify["oneOf"]; !hasOneOf {
		t.Fatalf("notify should have oneOf")
	}
}

func TestSanitizeSchema_ArrayTypeUnion(t *testing.T) {
	rawSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        []interface{}{"string", "null"},
				"description": "Path to target file",
			},
			"limit": map[string]interface{}{
				"type": []string{"integer", "null"},
			},
		},
	}

	sanitized := sanitizeSchema(rawSchema).(map[string]interface{})
	props := sanitized["properties"].(map[string]interface{})

	pathProp := props["path"].(map[string]interface{})
	if pathProp["type"] != "string" {
		t.Fatalf("expected path type to be string scalar, got %v", pathProp["type"])
	}
	if pathProp["nullable"] != true {
		t.Fatalf("expected path nullable to be true, got %v", pathProp["nullable"])
	}

	limitProp := props["limit"].(map[string]interface{})
	if limitProp["type"] != "integer" {
		t.Fatalf("expected limit type to be integer scalar, got %v", limitProp["type"])
	}
	if limitProp["nullable"] != true {
		t.Fatalf("expected limit nullable to be true, got %v", limitProp["nullable"])
	}
}

func TestSanitizeSchema_OpenCodeConstraints(t *testing.T) {
	rawSchema := map[string]interface{}{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"offset": map[string]interface{}{
				"type":             "integer",
				"exclusiveMinimum": 0,
				"multipleOf":       1,
			},
			"limit": map[string]interface{}{
				"type":             "integer",
				"exclusiveMaximum": 100,
			},
			"format": map[string]interface{}{
				"type":  "string",
				"const": "json",
			},
			"tags": map[string]interface{}{
				"type":        "array",
				"uniqueItems": true,
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"sample": map[string]interface{}{
				"type":     "string",
				"examples": []interface{}{"alpha", "beta"},
			},
		},
	}

	sanitized := sanitizeSchema(rawSchema).(map[string]interface{})
	props := sanitized["properties"].(map[string]interface{})

	offsetProp := props["offset"].(map[string]interface{})
	if _, ok := offsetProp["exclusiveMinimum"]; ok {
		t.Fatalf("exclusiveMinimum should be stripped")
	}
	if offsetProp["minimum"] != 0 {
		t.Fatalf("expected minimum to be 0, got %v", offsetProp["minimum"])
	}
	if _, ok := offsetProp["multipleOf"]; ok {
		t.Fatalf("multipleOf should be stripped")
	}

	limitProp := props["limit"].(map[string]interface{})
	if _, ok := limitProp["exclusiveMaximum"]; ok {
		t.Fatalf("exclusiveMaximum should be stripped")
	}
	if limitProp["maximum"] != 100 {
		t.Fatalf("expected maximum to be 100, got %v", limitProp["maximum"])
	}

	formatProp := props["format"].(map[string]interface{})
	if _, ok := formatProp["const"]; ok {
		t.Fatalf("const should be stripped")
	}
	enumList, ok := formatProp["enum"].([]interface{})
	if !ok || len(enumList) != 1 || enumList[0] != "json" {
		t.Fatalf("expected enum to be ['json'], got %v", formatProp["enum"])
	}

	tagsProp := props["tags"].(map[string]interface{})
	if _, ok := tagsProp["uniqueItems"]; ok {
		t.Fatalf("uniqueItems should be stripped")
	}

	sampleProp := props["sample"].(map[string]interface{})
	if _, ok := sampleProp["examples"]; ok {
		t.Fatalf("examples should be stripped")
	}
	if sampleProp["example"] != "alpha" {
		t.Fatalf("expected example to be alpha, got %v", sampleProp["example"])
	}
}


