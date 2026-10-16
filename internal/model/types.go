package model

import "time"

// Account represents a Google Antigravity account credentials
type Account struct {
	Email          string    `json:"email"`
	Name           string    `json:"name,omitempty"`
	Picture        string    `json:"picture,omitempty"`
	Tier           string    `json:"tier,omitempty"`
	Provider       string    `json:"provider,omitempty"`
	ProjectId      string    `json:"projectId,omitempty"`
	AccessToken    string    `json:"accessToken"`
	RefreshToken   string    `json:"refreshToken"`
	TokenExpiresAt time.Time `json:"tokenExpiresAt"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`

	// Safety & Rate Limiting state (anti-ban)
	IsActive          bool      `json:"isActive"`
	Order             int       `json:"order"`
	CooldownUntil     time.Time `json:"cooldownUntil,omitempty"`
	CooldownReason    string    `json:"cooldownReason,omitempty"`
	ConsecutiveErrors int       `json:"consecutiveErrors,omitempty"`
	LastUsedAt        time.Time `json:"lastUsedAt,omitempty"`
	TotalRequests     int64     `json:"totalRequests,omitempty"`
	TotalTokens       int64     `json:"totalTokens,omitempty"`

	// Cached quotas
	Quota5hPercent     float64            `json:"quota5hPercent"`
	Quota5hResetAt     string             `json:"quota5hResetAt,omitempty"`
	QuotaWeeklyPercent float64            `json:"quotaWeeklyPercent"`
	QuotaWeeklyResetAt string             `json:"quotaWeeklyResetAt,omitempty"`
	QuotaFetchedAt     time.Time          `json:"quotaFetchedAt,omitempty"`
	Families           map[string]Family  `json:"families,omitempty"`
	ModelsList         []ModelQuotaDetail `json:"modelsList,omitempty"`
}

type ModelQuotaDetail struct {
	ID                 string  `json:"id"`
	DisplayName        string  `json:"displayName"`
	RemainingPercent   float64 `json:"remainingPercent"`
	ResetAt            string  `json:"resetAt,omitempty"`
}

type FamilyLimit struct {
	RemainingPercent float64 `json:"remainingPercent"`
	ResetAt          string  `json:"resetAt,omitempty"`
	DisplayName      string  `json:"displayName"`
}

type Family struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Limit5h     FamilyLimit        `json:"limit5h"`
	LimitWeekly FamilyLimit        `json:"limitWeekly"`
	Models      []ModelQuotaDetail `json:"models"`
}

// ApiKey represents client credentials for calling the gateway
type ApiKey struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	LastUsed  time.Time `json:"lastUsed,omitempty"`
	Requests  int64     `json:"requests"`
	DefaultSkill string `json:"defaultSkill,omitempty"`
}

// Skill represents a prompt enhancement preset (Caveman, Ponytail, etc.)
type Skill struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	SystemPrompt string    `json:"systemPrompt"`
	Enabled      bool      `json:"enabled"`
	IsBuiltin    bool      `json:"isBuiltin"`
	CreatedAt    time.Time `json:"createdAt"`
}

// ModelInfo represents available models in the gateway
type ModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	DisplayName string `json:"displayName"`
	Family      string `json:"family"` // "gemini" or "claude"
	Description string `json:"description"`
}

// FallbackConfig defines configurable model fallback chains
type FallbackConfig struct {
	Enabled bool                `json:"enabled"`
	Rules   map[string][]string `json:"rules"` // originalModel -> []fallbackModels
}

// ProjectInfo represents a local project folder in PROJECTS_DIR
type ProjectInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	TechStack   string `json:"techStack,omitempty"`
	Description string `json:"description,omitempty"`
}

// ProjectContextSummary represents structured context extracted from project files
type ProjectContextSummary struct {
	ProjectName string   `json:"projectName"`
	TechStack   string   `json:"techStack"`
	KeyFiles    []string `json:"keyFiles"`
	SummaryText string   `json:"summaryText"`
}

// OpenAI Protocol Structs
type OpenAIToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type OpenAITool struct {
	Type     string             `json:"type"` // "function"
	Function OpenAIToolFunction `json:"function"`
}

type OpenAIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIToolCall struct {
	Index    *int                   `json:"index,omitempty"`
	ID       string                 `json:"id"`
	Type     string                 `json:"type"` // "function"
	Function OpenAIToolCallFunction `json:"function"`
}

type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content"` // string or []OpenAIContentPart
	Name       string           `json:"name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type OpenAIChatRequest struct {
	Model          string          `json:"model"`
	Messages       []OpenAIMessage `json:"messages"`
	Stream         bool            `json:"stream,omitempty"`
	Temperature    *float64        `json:"temperature,omitempty"`
	TopP           *float64        `json:"top_p,omitempty"`
	TopK           *int            `json:"top_k,omitempty"`
	MaxTokens      *int            `json:"max_tokens,omitempty"`
	ThinkingBudget *int            `json:"thinking_budget,omitempty"`
	SearchGrounding *bool          `json:"search_grounding,omitempty"`
	Stop           interface{}     `json:"stop,omitempty"`
	Tools          []OpenAITool    `json:"tools,omitempty"`
	ToolChoice     interface{}     `json:"tool_choice,omitempty"`
	Skill          string          `json:"skill,omitempty"`   // Custom gateway param
	Skills         []string        `json:"skills,omitempty"`  // Custom combo skills
	Account         string          `json:"account,omitempty"` // Target account
	Project         string          `json:"project,omitempty"` // Context project folder (optional)
}

type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason,omitempty"`
}

type OpenAIChatResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   map[string]int `json:"usage,omitempty"`
}

type OpenAIStreamDelta struct {
	Role             string           `json:"role,omitempty"`
	Content          string           `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type OpenAIStreamChoice struct {
	Index        int               `json:"index"`
	Delta        OpenAIStreamDelta `json:"delta"`
	FinishReason *string           `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens            int                  `json:"prompt_tokens"`
	CompletionTokens        int                  `json:"completion_tokens"`
	TotalTokens             int                  `json:"total_tokens"`
	PromptTokensDetails     *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *CompTokensDetails   `json:"completion_tokens_details,omitempty"`
}

type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type CompTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type OpenAIStreamChunk struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []OpenAIStreamChoice `json:"choices"`
	Usage   *OpenAIUsage         `json:"usage,omitempty"`
}

// Anthropic Protocol Structs
type AnthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema interface{} `json:"input_schema,omitempty"`
}

type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type AnthropicRequest struct {
	Model           string             `json:"model"`
	Messages        []AnthropicMessage `json:"messages"`
	System          interface{}        `json:"system,omitempty"` // string or parts
	MaxTokens       int                `json:"max_tokens,omitempty"`
	ThinkingBudget  *int               `json:"thinking_budget,omitempty"`
	SearchGrounding *bool              `json:"search_grounding,omitempty"`
	Stream          bool               `json:"stream,omitempty"`
	Temperature     *float64           `json:"temperature,omitempty"`
	TopP            *float64           `json:"top_p,omitempty"`
	TopK            *int               `json:"top_k,omitempty"`
	StopSequences   []string           `json:"stop_sequences,omitempty"`
	Tools           []AnthropicTool    `json:"tools,omitempty"`
	ToolChoice      interface{}        `json:"tool_choice,omitempty"`
	Skill           string             `json:"skill,omitempty"`
	Skills          []string           `json:"skills,omitempty"`
	Account         string             `json:"account,omitempty"`
	Project         string             `json:"project,omitempty"`
}

type AnthropicResponse struct {
	ID           string                   `json:"id"`
	Type         string                   `json:"type"`
	Role         string                   `json:"role"`
	Model        string                   `json:"model"`
	Content      []map[string]interface{} `json:"content"`
	StopReason   string                   `json:"stop_reason,omitempty"`
	StopSequence *string                  `json:"stop_sequence,omitempty"`
	Usage        map[string]int           `json:"usage,omitempty"`
}

// Token Analytics Structures
type TokenUsageRecord struct {
	ID                 string    `json:"id"`
	Timestamp          time.Time `json:"timestamp"`
	Date               string    `json:"date"` // YYYY-MM-DD
	Hour               int       `json:"hour"` // 0-23
	Model              string    `json:"model"`
	Account            string    `json:"account"`
	PromptTokens       int       `json:"promptTokens"`
	CachedTokens       int       `json:"cachedTokens"`
	ActualOutputTokens int       `json:"actualOutputTokens"`
	EstimatedRawTokens int       `json:"estimatedRawTokens"`
	SavedOutputTokens  int       `json:"savedOutputTokens"`
	SavedPercentage    float64   `json:"savedPercentage"`
	SkillsUsed         []string  `json:"skillsUsed"`
	HasTokenSaver      bool      `json:"hasTokenSaver"`
}

type TokenAnalyticsSummary struct {
	TotalRequests          int64   `json:"totalRequests"`
	TotalInputTokens       int64   `json:"totalInputTokens"`
	TotalCachedTokens      int64   `json:"totalCachedTokens"`
	TotalActualOutput      int64   `json:"totalActualOutput"`
	TotalEstimatedRaw      int64   `json:"totalEstimatedRaw"`
	TotalSavedOutput       int64   `json:"totalSavedOutput"`
	OverallSavedPercentage float64 `json:"overallSavedPercentage"`
}

type ModelUsageBreakdown struct {
	Model           string  `json:"model"`
	Requests        int64   `json:"requests"`
	InputTokens     int64   `json:"inputTokens"`
	CachedTokens    int64   `json:"cachedTokens"`
	ActualOutput    int64   `json:"actualOutput"`
	EstimatedRaw    int64   `json:"estimatedRaw"`
	SavedOutput     int64   `json:"savedOutput"`
	SavedPercentage float64 `json:"savedPercentage"`
}

type TimeSeriesDataPoint struct {
	Label        string  `json:"label"` // e.g. "14:00" or "Senin (06/10)" or "W1 Okt"
	DateKey      string  `json:"dateKey"`
	InputTokens  int64   `json:"inputTokens"`
	CachedTokens int64   `json:"cachedTokens"`
	ActualOutput int64   `json:"actualOutput"`
	EstimatedRaw int64   `json:"estimatedRaw"`
	SavedOutput  int64   `json:"savedOutput"`
	SavedPct     float64 `json:"savedPct"`
}

type TokenAnalyticsResponse struct {
	Period     string                `json:"period"` // "day", "week", "month"
	Model      string                `json:"model"`
	Summary    TokenAnalyticsSummary `json:"summary"`
	TimeSeries []TimeSeriesDataPoint `json:"timeSeries"`
	ByModel    []ModelUsageBreakdown `json:"byModel"`
	RecentLogs []TokenUsageRecord    `json:"recentLogs"`
}

type ProviderStrategyConfig struct {
	Strategy string `json:"strategy"` // "smart_quota", "fill_first"
}
