package provider

import (
	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/model"
	"antigravity-gateway/internal/storage"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type AccountPool struct {
	mu           sync.Mutex
	cfg          *config.Config
	storage      *storage.Storage
	client       *AntigravityClient
	sessions     map[string]string // sessionID -> email (Session affinity)
	sessionsMu   sync.RWMutex
	refreshLocks sync.Map // email -> *sync.Mutex (Double-checked locking for token refresh)
}

func NewAccountPool(cfg *config.Config, st *storage.Storage, client *AntigravityClient) *AccountPool {
	return &AccountPool{
		cfg:      cfg,
		storage:  st,
		client:   client,
		sessions: make(map[string]string),
	}
}

// RemoveAccountSessions cleans up any session affinities tied to a removed account
func (p *AccountPool) RemoveAccountSessions(email string) {
	p.sessionsMu.Lock()
	defer p.sessionsMu.Unlock()
	for sessID, targetEmail := range p.sessions {
		if targetEmail == email {
			delete(p.sessions, sessID)
		}
	}
}

// SelectAccount chooses the best healthy account with anti-ban heuristics, prioritizing targetAccount if specified
func (p *AccountPool) SelectAccount(targetAccount string, sessionID string, excludedEmails []string, requestedModel ...string) (*model.Account, error) {
	chosen, err := p.selectAccountLocked(targetAccount, sessionID, excludedEmails, requestedModel...)
	if err != nil {
		return nil, err
	}

	// Apply human jitter outside the lock so other goroutines are not blocked
	p.applyHumanJitter()
	return chosen, nil
}

func (p *AccountPool) selectAccountLocked(targetAccount string, sessionID string, excludedEmails []string, requestedModel ...string) (*model.Account, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	excluded := make(map[string]bool)
	for _, e := range excludedEmails {
		excluded[e] = true
	}

	accounts := p.storage.GetAccounts()
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no Antigravity accounts configured. Please login first")
	}

	var reqModel string
	if len(requestedModel) > 0 {
		reqModel = strings.ToLower(requestedModel[0])
	}

	isModelExhausted := func(acc *model.Account) bool {
		if reqModel == "" || acc.Families == nil {
			return false
		}
		if strings.HasPrefix(reqModel, "gemini-") {
			if gemFam, ok := acc.Families["gemini"]; ok {
				if gemFam.LimitWeekly.RemainingPercent == 0.0 {
					return true
				}
			}
		} else if strings.HasPrefix(reqModel, "claude-") || strings.HasPrefix(reqModel, "gpt-") {
			if claudeFam, ok := acc.Families["claude"]; ok {
				if claudeFam.LimitWeekly.RemainingPercent == 0.0 {
					return true
				}
			}
		}
		return false
	}

	// 0. Explicit Target Account Override (for testing specific account in Playground)
	if targetAccount != "" && !excluded[targetAccount] {
		for _, acc := range accounts {
			if acc.Email == targetAccount {
				if !acc.IsActive {
					return nil, fmt.Errorf("target account %s is currently disabled", targetAccount)
				}
				return acc, nil
			}
		}
		return nil, fmt.Errorf("target account %s not found in pool", targetAccount)
	}

	now := time.Now()

	// 1. Check Session Affinity (if conversation session key matches and account is healthy)
	if sessionID != "" {
		p.sessionsMu.RLock()
		targetEmail, hasAffinity := p.sessions[sessionID]
		p.sessionsMu.RUnlock()

		if hasAffinity && !excluded[targetEmail] {
			for _, acc := range accounts {
				if acc.Email == targetEmail && acc.IsActive && now.After(acc.CooldownUntil) && !isModelExhausted(acc) {
					// Check if account has safe remaining quota (> 15%) to maintain prompt cache affinity
					hasQuota5h := acc.Quota5hPercent == 0 || acc.Quota5hPercent > 15.0
					hasQuotaWeekly := acc.QuotaWeeklyPercent == 0 || acc.QuotaWeeklyPercent > 15.0
					if hasQuota5h && hasQuotaWeekly {
						return acc, nil
					}
				}
			}
		}
	}

	// 2. Filter available healthy accounts with Pre-emptive Parking (> 15% safe margin)
	var candidates []*model.Account
	for _, acc := range accounts {
		if !acc.IsActive || excluded[acc.Email] || isModelExhausted(acc) {
			continue
		}
		// In cooldown?
		if now.Before(acc.CooldownUntil) {
			continue
		}
		// Preemptive Parking: Let account sleep before reaching 0% so rolling 5h window recovers
		if acc.Quota5hPercent > 0 && acc.Quota5hPercent < 15.0 {
			continue
		}
		if acc.QuotaWeeklyPercent > 0 && acc.QuotaWeeklyPercent < 15.0 {
			continue
		}
		candidates = append(candidates, acc)
	}

	if len(candidates) == 0 {
		// Fallback 1: Any active account whose cooldown has expired, remaining quota > 5%, and model not 0%
		for _, acc := range accounts {
			if acc.IsActive && now.After(acc.CooldownUntil) && !excluded[acc.Email] && !isModelExhausted(acc) {
				if (acc.Quota5hPercent == 0 || acc.Quota5hPercent > 5.0) && (acc.QuotaWeeklyPercent == 0 || acc.QuotaWeeklyPercent > 5.0) {
					candidates = append(candidates, acc)
				}
			}
		}
	}

	if len(candidates) == 0 {
		// Fallback 2: Any active account whose cooldown has expired and model not 0%
		for _, acc := range accounts {
			if acc.IsActive && now.After(acc.CooldownUntil) && !excluded[acc.Email] && !isModelExhausted(acc) {
				candidates = append(candidates, acc)
			}
		}
	}

	if len(candidates) == 0 {
		// Fallback 3: If ALL accounts are in cooldown, take any active account not excluded and not 0%
		for _, acc := range accounts {
			if acc.IsActive && !excluded[acc.Email] && !isModelExhausted(acc) {
				candidates = append(candidates, acc)
			}
		}
	}

	if len(candidates) == 0 {
		if reqModel != "" {
			return nil, fmt.Errorf("no available Antigravity accounts configured for model '%s' (quota exhausted on all accounts)", reqModel)
		}
		return nil, fmt.Errorf("no available Antigravity accounts configured")
	}

	// 3. Routing Strategy Sorting:
	strategy := p.storage.GetProviderStrategy()
	switch strategy {
	case "fill_first":
		// Prioritize manual order / stable order first. If candidate[i] has safe quota, keep using it completely until 429
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Order != candidates[j].Order {
				return candidates[i].Order < candidates[j].Order
			}
			return candidates[i].Email < candidates[j].Email
		})
	case "round_robin":
		// Pick least recently used
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].LastUsedAt.Before(candidates[j].LastUsedAt)
		})
	case "least_used":
		// Pick lowest total requests
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].TotalRequests != candidates[j].TotalRequests {
				return candidates[i].TotalRequests < candidates[j].TotalRequests
			}
			return candidates[i].LastUsedAt.Before(candidates[j].LastUsedAt)
		})
	default: // "smart_quota"
		// Prioritize highest remaining quota bottleneck, penalize errors, tie-breaker idle longest
		sort.Slice(candidates, func(i, j int) bool {
			minQuotaI := candidates[i].Quota5hPercent
			if candidates[i].QuotaWeeklyPercent > 0 && candidates[i].QuotaWeeklyPercent < minQuotaI {
				minQuotaI = candidates[i].QuotaWeeklyPercent
			}
			minQuotaJ := candidates[j].Quota5hPercent
			if candidates[j].QuotaWeeklyPercent > 0 && candidates[j].QuotaWeeklyPercent < minQuotaJ {
				minQuotaJ = candidates[j].QuotaWeeklyPercent
			}

			scoreI := minQuotaI - float64(candidates[i].ConsecutiveErrors*30)
			scoreJ := minQuotaJ - float64(candidates[j].ConsecutiveErrors*30)

			if scoreI != scoreJ {
				return scoreI > scoreJ
			}
			return candidates[i].LastUsedAt.Before(candidates[j].LastUsedAt)
		})
	}

	chosen := candidates[0]

	// Register session affinity
	if sessionID != "" {
		p.sessionsMu.Lock()
		p.sessions[sessionID] = chosen.Email
		p.sessionsMu.Unlock()
	}

	return chosen, nil
}

func (p *AccountPool) applyHumanJitter() {
	jitterMs := 150 + rand.Intn(200)
	time.Sleep(time.Duration(jitterMs) * time.Millisecond)
}

// EnsureValidToken refreshes access token if expired with double-checked locking
func (p *AccountPool) EnsureValidToken(ctx context.Context, acc *model.Account) (*model.Account, error) {
	if time.Now().Before(acc.TokenExpiresAt.Add(-2 * time.Minute)) {
		return acc, nil
	}

	// Serialize token refresh per account to prevent duplicate refresh calls
	lockVal, _ := p.refreshLocks.LoadOrStore(acc.Email, &sync.Mutex{})
	accMu := lockVal.(*sync.Mutex)
	accMu.Lock()
	defer accMu.Unlock()

	// Double check latest account status from storage after acquiring lock
	if latest, ok := p.storage.GetAccount(acc.Email); ok {
		if time.Now().Before(latest.TokenExpiresAt.Add(-2 * time.Minute)) {
			return latest, nil
		}
		acc = latest
	}

	// Refresh token
	newToken, expiresAt, err := p.client.RefreshToken(ctx, acc.RefreshToken)
	if err != nil {
		// Circuit breaker for failed auth
		acc.ConsecutiveErrors++
		acc.CooldownUntil = time.Now().Add(10 * time.Minute)
		acc.CooldownReason = fmt.Sprintf("Token refresh failed: %v", err)
		_ = p.storage.UpsertAccount(acc)
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}

	acc.AccessToken = newToken
	acc.TokenExpiresAt = expiresAt
	acc.ConsecutiveErrors = 0
	_ = p.storage.UpsertAccount(acc)
	return acc, nil
}

// MarkAccountSuccess updates usage metrics
func (p *AccountPool) MarkAccountSuccess(email string) {
	if acc, ok := p.storage.GetAccount(email); ok {
		acc.ConsecutiveErrors = 0
		acc.TotalRequests++
		acc.LastUsedAt = time.Now()
		_ = p.storage.UpsertAccount(acc)
	}
}

// MarkAccountRateLimited applies cooldown for 429 errors
func (p *AccountPool) MarkAccountRateLimited(email string, reason string) {
	if acc, ok := p.storage.GetAccount(email); ok {
		acc.ConsecutiveErrors++
		// Shorter backoff: 30s, 60s, 120s...
		duration := 30 * time.Duration(1<<uint(min(acc.ConsecutiveErrors-1, 4))) * time.Second
		acc.CooldownUntil = time.Now().Add(duration)
		acc.CooldownReason = reason
		_ = p.storage.UpsertAccount(acc)
	}
}

// MarkAccountAuthError triggers immediate quarantine until re-auth
func (p *AccountPool) MarkAccountAuthError(email string, reason string) {
	if acc, ok := p.storage.GetAccount(email); ok {
		acc.ConsecutiveErrors++
		acc.CooldownUntil = time.Now().Add(30 * time.Minute)
		acc.CooldownReason = reason
		_ = p.storage.UpsertAccount(acc)
	}
}

// RefreshAllQuotas updates quota metrics for all stored accounts
func (p *AccountPool) RefreshAllQuotas(ctx context.Context) {
	accounts := p.storage.GetAccounts()
	for _, acc := range accounts {
		validAcc, err := p.EnsureValidToken(ctx, acc)
		if err != nil {
			continue
		}
		p.FetchAccountQuota(ctx, validAcc)
	}
}

// FetchAccountQuota queries Google for 5h and weekly quota summary, plus per-model quota
func (p *AccountPool) FetchAccountQuota(ctx context.Context, acc *model.Account) {
	body := []byte(fmt.Sprintf(`{"project":"%s"}`, acc.ProjectId))

	// 1. Quota Summary
	res, err := p.client.CallCloudCode(ctx, p.cfg.QuotaSummaryPath, acc.AccessToken, acc.ProjectId, body)
	var summaryGroups []struct {
		DisplayName string `json:"displayName"`
		Buckets     []struct {
			RemainingFraction float64 `json:"remainingFraction"`
			Window            string  `json:"window"`
			ResetTime         string  `json:"resetTime"`
		} `json:"buckets"`
	}

	if err == nil && res.StatusCode == http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		var sData struct {
			Groups []struct {
				DisplayName string `json:"displayName"`
				Buckets     []struct {
					RemainingFraction float64 `json:"remainingFraction"`
					Window            string  `json:"window"`
					ResetTime         string  `json:"resetTime"`
				} `json:"buckets"`
			} `json:"groups"`
			QuotaSummary struct {
				Groups []struct {
					DisplayName string `json:"displayName"`
					Buckets     []struct {
						RemainingFraction float64 `json:"remainingFraction"`
						Window            string  `json:"window"`
						ResetTime         string  `json:"resetTime"`
					} `json:"buckets"`
				} `json:"groups"`
			} `json:"quotaSummary"`
		}
		if err := json.Unmarshal(raw, &sData); err == nil {
			if len(sData.Groups) > 0 {
				summaryGroups = sData.Groups
			} else {
				summaryGroups = sData.QuotaSummary.Groups
			}
		}
	} else if res != nil {
		res.Body.Close()
	}

	var session5hBuckets []model.FamilyLimit
	var weeklyBuckets []model.FamilyLimit

	for _, g := range summaryGroups {
		for _, b := range g.Buckets {
			w := strings.ToLower(b.Window)
			pct := b.RemainingFraction * 100.0
			fl := model.FamilyLimit{
				DisplayName:      g.DisplayName,
				RemainingPercent: pct,
				ResetAt:          b.ResetTime,
			}
			if strings.Contains(w, "5h") || strings.Contains(w, "session") || strings.Contains(w, "daily") {
				session5hBuckets = append(session5hBuckets, fl)
				acc.Quota5hPercent = pct
				acc.Quota5hResetAt = b.ResetTime
			} else if strings.Contains(w, "7d") || strings.Contains(w, "week") {
				weeklyBuckets = append(weeklyBuckets, fl)
				acc.QuotaWeeklyPercent = pct
				acc.QuotaWeeklyResetAt = b.ResetTime
			}
		}
	}

	// 2. Fetch Available Models
	var modelsList []model.ModelQuotaDetail
	mRes, mErr := p.client.CallCloudCode(ctx, p.cfg.ModelsPath, acc.AccessToken, acc.ProjectId, body)
	if mErr == nil && mRes.StatusCode == http.StatusOK {
		raw, _ := io.ReadAll(mRes.Body)
		mRes.Body.Close()

		var mData struct {
			Models map[string]struct {
				DisplayName string `json:"displayName"`
				IsInternal  bool   `json:"isInternal"`
				QuotaInfo   struct {
					RemainingFraction float64 `json:"remainingFraction"`
					ResetTime         string  `json:"resetTime"`
				} `json:"quotaInfo"`
			} `json:"models"`
		}
		if err := json.Unmarshal(raw, &mData); err == nil && mData.Models != nil {
			for id, info := range mData.Models {
				if info.IsInternal || strings.HasPrefix(id, "tab_") || strings.HasPrefix(id, "chat_") || id == "gemini-2.5-pro" || id == "gemini-3.1-pro" {
					continue
				}
				dName := info.DisplayName
				if dName == "" {
					dName = id
				}
				// Claude v4.6 standard naming
				if strings.Contains(strings.ToLower(id), "claude-sonnet") {
					dName = "Claude Sonnet 4.6 (Thinking)"
				} else if strings.Contains(strings.ToLower(id), "claude-opus") {
					dName = "Claude Opus 4.6 (Thinking)"
				}
				pct := info.QuotaInfo.RemainingFraction * 100.0
				modelsList = append(modelsList, model.ModelQuotaDetail{
					ID:               id,
					DisplayName:      dName,
					RemainingPercent: pct,
					ResetAt:          info.QuotaInfo.ResetTime,
				})
			}
		}
	} else if mRes != nil {
		mRes.Body.Close()
	}

	// Separate into Gemini & Claude families
	var geminiModels []model.ModelQuotaDetail
	var claudeModels []model.ModelQuotaDetail

	for _, m := range modelsList {
		low := strings.ToLower(m.ID)
		if strings.Contains(low, "claude") || strings.Contains(low, "gpt") {
			claudeModels = append(claudeModels, m)
		} else {
			geminiModels = append(geminiModels, m)
		}
	}

	// Find family limits
	var gem5h, gemWk, cl5h, clWk model.FamilyLimit
	for _, b := range session5hBuckets {
		if strings.Contains(strings.ToLower(b.DisplayName), "gemini") {
			gem5h = b
		} else if strings.Contains(strings.ToLower(b.DisplayName), "claude") || strings.Contains(strings.ToLower(b.DisplayName), "gpt") {
			cl5h = b
		}
	}
	for _, b := range weeklyBuckets {
		if strings.Contains(strings.ToLower(b.DisplayName), "gemini") {
			gemWk = b
		} else if strings.Contains(strings.ToLower(b.DisplayName), "claude") || strings.Contains(strings.ToLower(b.DisplayName), "gpt") {
			clWk = b
		}
	}

	if gem5h.DisplayName == "" && len(geminiModels) > 0 {
		gem5h = model.FamilyLimit{DisplayName: "Gemini 5 Jam", RemainingPercent: geminiModels[0].RemainingPercent, ResetAt: geminiModels[0].ResetAt}
	}
	if cl5h.DisplayName == "" && len(claudeModels) > 0 {
		cl5h = model.FamilyLimit{DisplayName: "Claude 5 Jam", RemainingPercent: claudeModels[0].RemainingPercent, ResetAt: claudeModels[0].ResetAt}
	}

	acc.Families = map[string]model.Family{
		"gemini": {
			ID:          "gemini",
			Name:        "Gemini Models",
			Limit5h:     gem5h,
			LimitWeekly: gemWk,
			Models:      geminiModels,
		},
		"claude": {
			ID:          "claude",
			Name:        "Claude Models",
			Limit5h:     cl5h,
			LimitWeekly: clWk,
			Models:      claudeModels,
		},
	}
	acc.ModelsList = modelsList
	acc.QuotaFetchedAt = time.Now()
	_ = p.storage.UpsertAccount(acc)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
