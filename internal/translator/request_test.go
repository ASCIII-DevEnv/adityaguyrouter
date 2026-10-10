package translator

import (
	"antigravity-gateway/internal/model"
	"strings"
	"testing"
)

func TestConvertOpenAIToAntigravity(t *testing.T) {
	req := &model.OpenAIChatRequest{
		Model: "gemini-2.5-pro",
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
