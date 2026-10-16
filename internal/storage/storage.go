package storage

import (
	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/model"
	"antigravity-gateway/internal/skills"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Storage struct {
	mu              sync.RWMutex
	accountsPath    string
	apiKeysPath     string
	skillsPath      string
	tokenUsagePath  string
	fallbacksPath   string
	strategyPath    string

	accounts        map[string]*model.Account
	apiKeys         map[string]*model.ApiKey
	skills          map[string]*model.Skill
	fallbacks       *model.FallbackConfig
	providerStrategy string
	tokenUsage      []model.TokenUsageRecord
	tokenUsageDirty bool
	inspector       *RequestInspector
}

func NewStorage(cfg *config.Config) (*Storage, error) {
	strategyPath := cfg.StrategyPath
	if strategyPath == "" && cfg.DataDir != "" {
		strategyPath = filepath.Join(cfg.DataDir, "provider_strategy.json")
	}

	s := &Storage{
		accountsPath:   cfg.AccountsPath,
		apiKeysPath:    cfg.ApiKeysPath,
		skillsPath:     cfg.SkillsPath,
		tokenUsagePath: cfg.TokenUsagePath,
		fallbacksPath:  cfg.FallbacksPath,
		strategyPath:   strategyPath,
		accounts:       make(map[string]*model.Account),
		apiKeys:        make(map[string]*model.ApiKey),
		skills:         make(map[string]*model.Skill),
		tokenUsage:     make([]model.TokenUsageRecord, 0),
		inspector:      NewRequestInspector(60),
	}

	if err := s.loadAll(); err != nil {
		return nil, err
	}
	s.startFlushWorker()
	return s, nil
}

func (s *Storage) startFlushWorker() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s.FlushTokenUsage()
		}
	}()
}

// FlushTokenUsage persists dirty token usage to disk without holding locks during file I/O
func (s *Storage) FlushTokenUsage() {
	s.mu.Lock()
	if !s.tokenUsageDirty {
		s.mu.Unlock()
		return
	}
	s.tokenUsageDirty = false
	records := make([]model.TokenUsageRecord, len(s.tokenUsage))
	copy(records, s.tokenUsage)
	s.mu.Unlock()

	bytes, err := json.MarshalIndent(records, "", "  ")
	if err == nil {
		_ = atomicWriteFile(s.tokenUsagePath, bytes, 0644)
	}
}

// Request Inspector operations
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, "temp-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// Request Inspector operations
func (s *Storage) GetInspector() *RequestInspector {
	return s.inspector
}

func (s *Storage) loadAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Load accounts
	if raw, err := os.ReadFile(s.accountsPath); err == nil {
		var wrapper struct {
			Accounts map[string]*model.Account `json:"accounts"`
		}
		if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Accounts != nil {
			s.accounts = wrapper.Accounts
		} else {
			// Direct map fallback
			var direct map[string]*model.Account
			if err := json.Unmarshal(raw, &direct); err == nil {
				s.accounts = direct
			}
		}
	}

	// 2. Load API keys
	if raw, err := os.ReadFile(s.apiKeysPath); err == nil {
		var keys map[string]*model.ApiKey
		if err := json.Unmarshal(raw, &keys); err == nil {
			s.apiKeys = keys
		}
	}
	// Create default API key if empty or ensure combo skill is set
	const allComboSkills = "caveman,ponytail,token-efficiency"
	if len(s.apiKeys) == 0 {
		defKey := &model.ApiKey{
			ID:           "default-key",
			Key:          "«redacted:sk-…»",
			Name:         "Default Local Key",
			DefaultSkill: allComboSkills,
			CreatedAt:    time.Now(),
		}
		s.apiKeys[defKey.Key] = defKey
		_ = s.saveApiKeysLocked()
	} else if defKey, exists := s.apiKeys["«redacted:sk-…»"]; exists {
		defKey.DefaultSkill = allComboSkills
		_ = s.saveApiKeysLocked()
	}

	// 3. Load Skills
	if raw, err := os.ReadFile(s.skillsPath); err == nil {
		var sk map[string]*model.Skill
		if err := json.Unmarshal(raw, &sk); err == nil {
			s.skills = sk
		}
	}
	// Seed or update built-in skills (preserve user's Enabled flag, and do NOT resurrect if user explicitly deleted)
	builtinMap := make(map[string]bool)
	for _, b := range skills.GetBuiltinSkills() {
		builtinMap[b.ID] = true
		existing, exists := s.skills[b.ID]
		if !exists {
			if len(s.skills) == 0 {
				item := b
				s.skills[b.ID] = &item
			}
		} else {
			existing.IsBuiltin = true
			existing.SystemPrompt = b.SystemPrompt
			existing.Description = b.Description
			existing.Name = b.Name
		}
	}
	// Purge obsolete built-in skills that no longer exist in GetBuiltinSkills()
	for id, sk := range s.skills {
		if sk.IsBuiltin && !builtinMap[id] {
			delete(s.skills, id)
		}
	}
	_ = s.saveSkillsLocked()

	// 4. Load Token Usage
	s.loadTokenUsageLocked()

	// Aggregate historical tokens for accounts that don't have TotalTokens recorded yet
	accountsNeedingTokens := make(map[string]bool)
	for _, acc := range s.accounts {
		if acc.TotalTokens == 0 {
			accountsNeedingTokens[acc.Email] = true
		}
	}
	if len(accountsNeedingTokens) > 0 {
		for _, rec := range s.tokenUsage {
			if accountsNeedingTokens[rec.Account] {
				if acc, ok := s.accounts[rec.Account]; ok {
					acc.TotalTokens += int64(rec.PromptTokens + rec.ActualOutputTokens)
				}
			}
		}
	}

	// 5. Load Fallback Rules
	if raw, err := os.ReadFile(s.fallbacksPath); err == nil {
		var fb model.FallbackConfig
		if err := json.Unmarshal(raw, &fb); err == nil {
			s.fallbacks = &fb
		}
	}
	if s.fallbacks == nil {
		s.fallbacks = &model.FallbackConfig{
			Enabled: true,
			Rules: map[string][]string{
		"claude-opus-4-6-thinking": {"claude-sonnet-4-6", "gemini-3.8-flash-tiered"},
		"claude-sonnet-4-6":       {"claude-opus-4-6-thinking", "gemini-3.8-flash-tiered"},
		"gpt-oss-120b-medium":     {"gemini-3.8-flash-tiered", "claude-sonnet-4-6"},
		"gemini-3.8-flash-tiered": {"claude-sonnet-4-6", "gpt-oss-120b-medium"},
		"default":                 {"gemini-3.8-flash-tiered", "claude-sonnet-4-6"},
			},
		}
		_ = s.saveFallbacksLocked()
	}

	// 6. Load Provider Strategy
	if s.strategyPath != "" {
		if raw, err := os.ReadFile(s.strategyPath); err == nil {
			var sc model.ProviderStrategyConfig
			if err := json.Unmarshal(raw, &sc); err == nil && sc.Strategy != "" {
				s.providerStrategy = sc.Strategy
			}
		}
	}
	if s.providerStrategy == "" {
		s.providerStrategy = "smart_quota"
	}

	return nil
}

func (s *Storage) saveAccountsLocked() error {
	wrapper := map[string]interface{}{
		"accounts":  s.accounts,
		"updatedAt": time.Now().Format(time.RFC3339),
	}
	bytes, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.accountsPath, bytes, 0644)
}

func (s *Storage) saveApiKeysLocked() error {
	bytes, err := json.MarshalIndent(s.apiKeys, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.apiKeysPath, bytes, 0644)
}

func (s *Storage) saveSkillsLocked() error {
	bytes, err := json.MarshalIndent(s.skills, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.skillsPath, bytes, 0644)
}

// Account operations
func (s *Storage) GetAccounts() []*model.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]*model.Account, 0, len(s.accounts))
	for _, acc := range s.accounts {
		res = append(res, acc)
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].Order != res[j].Order {
			return res[i].Order < res[j].Order
		}
		return res[i].Email < res[j].Email
	})
	return res
}

func (s *Storage) GetAccount(email string) (*model.Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acc, exists := s.accounts[email]
	return acc, exists
}

func (s *Storage) UpsertAccount(acc *model.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if acc.CreatedAt.IsZero() {
		acc.CreatedAt = time.Now()
	}
	acc.UpdatedAt = time.Now()
	s.accounts[acc.Email] = acc
	return s.saveAccountsLocked()
}

func (s *Storage) RecordAccountUsage(email string, tokens int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if acc, ok := s.accounts[email]; ok {
		acc.ConsecutiveErrors = 0
		acc.TotalRequests++
		acc.TotalTokens += tokens
		acc.LastUsedAt = time.Now()
		acc.UpdatedAt = time.Now()
		_ = s.saveAccountsLocked()
	}
}

func (s *Storage) DeleteAccount(email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.accounts, email)
	return s.saveAccountsLocked()
}

func (s *Storage) ReorderAccounts(orderedEmails []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for idx, email := range orderedEmails {
		if acc, ok := s.accounts[email]; ok {
			acc.Order = idx
		}
	}
	return s.saveAccountsLocked()
}

// ApiKey operations
func (s *Storage) GetApiKeys() []*model.ApiKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]*model.ApiKey, 0, len(s.apiKeys))
	for _, k := range s.apiKeys {
		res = append(res, k)
	}
	return res
}

func (s *Storage) GetApiKey(key string) (*model.ApiKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	apiKey, exists := s.apiKeys[key]
	return apiKey, exists
}

func (s *Storage) ValidateApiKey(key string) (*model.ApiKey, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	apiKey, exists := s.apiKeys[key]
	if exists {
		apiKey.LastUsed = time.Now()
		apiKey.Requests++
		_ = s.saveApiKeysLocked()
	}
	return apiKey, exists
}

func (s *Storage) CreateApiKey(name, defaultSkill string) (*model.ApiKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("sk-ag-%s", hex.EncodeToString(b))

	if defaultSkill == "" {
		defaultSkill = "caveman,ponytail,token-efficiency"
	}

	apiKey := &model.ApiKey{
		ID:           fmt.Sprintf("key_%d", time.Now().UnixNano()),
		Key:          key,
		Name:         name,
		CreatedAt:    time.Now(),
		DefaultSkill: defaultSkill,
	}
	s.apiKeys[key] = apiKey
	if err := s.saveApiKeysLocked(); err != nil {
		return nil, err
	}
	return apiKey, nil
}

func (s *Storage) DeleteApiKey(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.apiKeys, key)
	return s.saveApiKeysLocked()
}

// Skill operations
func (s *Storage) GetSkills() []*model.Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]*model.Skill, 0, len(s.skills))
	for _, sk := range s.skills {
		res = append(res, sk)
	}
	// Sort predictably: Builtin first, then alphabetical by name
	sort.Slice(res, func(i, j int) bool {
		if res[i].IsBuiltin != res[j].IsBuiltin {
			return res[i].IsBuiltin
		}
		return strings.ToLower(res[i].Name) < strings.ToLower(res[j].Name)
	})
	return res
}

func (s *Storage) GetSkill(id string) (*model.Skill, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sk, exists := s.skills[id]
	if !exists {
		return nil, false
	}
	return sk, true
}

func (s *Storage) ToggleSkill(id string) (*model.Skill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sk, exists := s.skills[id]
	if !exists {
		return nil, fmt.Errorf("skill '%s' tidak ditemukan", id)
	}
	sk.Enabled = !sk.Enabled
	if err := s.saveSkillsLocked(); err != nil {
		return nil, err
	}
	return sk, nil
}

func (s *Storage) UpsertSkill(sk *model.Skill) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, exists := s.skills[sk.ID]; exists {
		if sk.Name == "" {
			sk.Name = existing.Name
		}
		if sk.Description == "" {
			sk.Description = existing.Description
		}
		if sk.SystemPrompt == "" {
			sk.SystemPrompt = existing.SystemPrompt
		}
		sk.IsBuiltin = existing.IsBuiltin
		if sk.CreatedAt.IsZero() {
			sk.CreatedAt = existing.CreatedAt
		}
	} else {
		if sk.CreatedAt.IsZero() {
			sk.CreatedAt = time.Now()
		}
	}
	s.skills[sk.ID] = sk
	return s.saveSkillsLocked()
}

func (s *Storage) DeleteSkill(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sk, exists := s.skills[id]
	if !exists {
		return fmt.Errorf("skill '%s' tidak ditemukan", id)
	}
	_ = sk
	delete(s.skills, id)
	return s.saveSkillsLocked()
}

func (s *Storage) ResetBuiltinSkills() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	builtinMap := make(map[string]bool)
	for _, b := range skills.GetBuiltinSkills() {
		builtinMap[b.ID] = true
		item := b
		item.Enabled = true
		s.skills[b.ID] = &item
	}
	for id, sk := range s.skills {
		if sk.IsBuiltin && !builtinMap[id] {
			delete(s.skills, id)
		}
	}
	return s.saveSkillsLocked()
}

// Token Usage and Analytics
func (s *Storage) loadTokenUsageLocked() {
	if raw, err := os.ReadFile(s.tokenUsagePath); err == nil {
		var records []model.TokenUsageRecord
		if err := json.Unmarshal(raw, &records); err == nil && len(records) > 0 {
		// Normalize historical models
		for i := range records {
			m := records[i].Model
			if m == "gemini-2.5-pro" {
				records[i].Model = "gemini-3.8-flash-tiered"
			} else if m == "claude-sonnet-5-5" {
				records[i].Model = "claude-sonnet-4-6"
			} else if m == "claude-opus-5-5-thinking" {
				records[i].Model = "claude-opus-4-6-thinking"
			}
		}
			s.tokenUsage = records
			s.pruneOldTokenUsageLocked(time.Now())
			_ = s.saveTokenUsageLocked()
			return
		}
	}
	// Seed realistic initial analytics dataset if empty
	s.seedSampleTokenUsageLocked()
}

// pruneOldTokenUsageLocked retains records from the past 1 month (30 days) and deletes older ones
func (s *Storage) pruneOldTokenUsageLocked(now time.Time) {
	cutoff := now.AddDate(0, 0, -30)
	var kept []model.TokenUsageRecord
	for _, rec := range s.tokenUsage {
		if !rec.Timestamp.Before(cutoff) {
			kept = append(kept, rec)
		}
	}
	// Secondary safety ceiling (100,000 records) to protect 256MB container memory
	const maxRetentionCap = 100000
	if len(kept) > maxRetentionCap {
		kept = kept[len(kept)-maxRetentionCap:]
	}
	s.tokenUsage = kept
}

func (s *Storage) saveTokenUsageLocked() error {
	s.pruneOldTokenUsageLocked(time.Now())
	bytes, err := json.MarshalIndent(s.tokenUsage, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.tokenUsagePath, bytes, 0644)
}

func (s *Storage) seedSampleTokenUsageLocked() {
	now := time.Now()
	modelsList := []string{
		"gemini-3.8-flash-tiered",
		"claude-sonnet-4-6",
		"claude-opus-4-6-thinking",
		"gpt-oss-120b-medium",
	}
	accountsList := []string{
		"developer_1@example.com",
		"developer_2@example.com",
		"team_ai@example.com",
	}

	// Skill variations to produce realistic variety across seed records
	skillCombos := [][]string{
		{"caveman"},             // ~62%
		{"ponytail"},            // ~48%
		{"caveman", "ponytail"}, // ~80%
	}

	// Seed for past 30 days
	var records []model.TokenUsageRecord
	reqCounter := 1

	for daysAgo := 29; daysAgo >= 0; daysAgo-- {
		t := now.AddDate(0, 0, -daysAgo)
		dateStr := t.Format("2006-01-02")
		// Between 4 and 10 requests per day
		dailyCount := 5 + (daysAgo*7)%6
		for i := 0; i < dailyCount; i++ {
			hour := (i * 3 + 8) % 24
			recTime := time.Date(t.Year(), t.Month(), t.Day(), hour, (i*17)%60, 0, 0, t.Location())
			m := modelsList[(daysAgo+i)%len(modelsList)]
			acc := accountsList[(i)%len(accountsList)]
			chosenSkills := skillCombos[(daysAgo*3+i)%len(skillCombos)]

			// Simulated realistic token counts
			actualOutput := 80 + (i*43+daysAgo*17)%260
			promptTokens := 280 + (i*97+daysAgo*41)%650
			cachedTokens := (promptTokens * 40) / 100 // ~40% cached

			estimatedRaw, savedOutput, savedPct, hasSaver := skills.CalculateTokenSavings(actualOutput, chosenSkills)

			rec := model.TokenUsageRecord{
				ID:                 fmt.Sprintf("seed_%d_%d", reqCounter, recTime.Unix()),
				Timestamp:          recTime,
				Date:               dateStr,
				Hour:               hour,
				Model:              m,
				Account:            acc,
				PromptTokens:       promptTokens,
				CachedTokens:       cachedTokens,
				ActualOutputTokens: actualOutput,
				EstimatedRawTokens: estimatedRaw,
				SavedOutputTokens:  savedOutput,
				SavedPercentage:    savedPct,
				SkillsUsed:         chosenSkills,
				HasTokenSaver:      hasSaver,
			}
			records = append(records, rec)
			reqCounter++
		}
	}

	s.tokenUsage = records
	_ = s.saveTokenUsageLocked()
}

func (s *Storage) RecordTokenUsage(rec *model.TokenUsageRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if rec.ID == "" {
		rec.ID = fmt.Sprintf("req_%d", now.UnixNano())
	}
	if rec.Timestamp.IsZero() {
		rec.Timestamp = now
	}
	if rec.Date == "" {
		rec.Date = rec.Timestamp.Format("2006-01-02")
	}
	rec.Hour = rec.Timestamp.Hour()

	s.tokenUsage = append(s.tokenUsage, *rec)
	// Prune records older than 1 month (30 days)
	s.pruneOldTokenUsageLocked(now)
	s.tokenUsageDirty = true
}

func (s *Storage) GetTokenAnalytics(period string, modelFilter string) *model.TokenAnalyticsResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	todayStr := now.Format("2006-01-02")

	// Determine time boundaries
	var startDate time.Time
	switch period {
	case "day":
		// Today from 00:00:00
		startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "month":
		// Past 30 days
		startDate = now.AddDate(0, 0, -29)
		startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	default:
		period = "week"
		// Past 7 days
		startDate = now.AddDate(0, 0, -6)
		startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	}

	// Filter matching records
	var filtered []model.TokenUsageRecord
	for _, rec := range s.tokenUsage {
		recModel := rec.Model
		if recModel == "gemini-2.5-pro" {
			recModel = "gemini-3.8-flash-tiered"
		} else if recModel == "claude-sonnet-5-5" {
			recModel = "claude-sonnet-4-6"
		} else if recModel == "claude-opus-5-5-thinking" {
			recModel = "claude-opus-4-6-thinking"
		}

		if rec.Timestamp.Before(startDate) {
			continue
		}
		if modelFilter != "" && modelFilter != "all" && recModel != modelFilter {
			continue
		}
		recCopy := rec
		recCopy.Model = recModel
		filtered = append(filtered, recCopy)
	}

	// Aggregate summary
	var summary model.TokenAnalyticsSummary
	summary.TotalRequests = int64(len(filtered))

	byModelMap := make(map[string]*model.ModelUsageBreakdown)

	for _, rec := range filtered {
		summary.TotalInputTokens += int64(rec.PromptTokens)
		summary.TotalCachedTokens += int64(rec.CachedTokens)
		summary.TotalActualOutput += int64(rec.ActualOutputTokens)
		summary.TotalEstimatedRaw += int64(rec.EstimatedRawTokens)
		summary.TotalSavedOutput += int64(rec.SavedOutputTokens)

		mb, ok := byModelMap[rec.Model]
		if !ok {
			mb = &model.ModelUsageBreakdown{Model: rec.Model}
			byModelMap[rec.Model] = mb
		}
		mb.Requests++
		mb.InputTokens += int64(rec.PromptTokens)
		mb.CachedTokens += int64(rec.CachedTokens)
		mb.ActualOutput += int64(rec.ActualOutputTokens)
		mb.EstimatedRaw += int64(rec.EstimatedRawTokens)
		mb.SavedOutput += int64(rec.SavedOutputTokens)
	}

	if summary.TotalEstimatedRaw > 0 {
		summary.OverallSavedPercentage = float64(summary.TotalSavedOutput) / float64(summary.TotalEstimatedRaw) * 100.0
	}

	var byModelList []model.ModelUsageBreakdown
	for _, mb := range byModelMap {
		if mb.EstimatedRaw > 0 {
			mb.SavedPercentage = float64(mb.SavedOutput) / float64(mb.EstimatedRaw) * 100.0
		}
		byModelList = append(byModelList, *mb)
	}

	// Build TimeSeries data
	var timeSeries []model.TimeSeriesDataPoint

	if period == "day" {
		// Group by 24 hours of today
		hourlyMap := make(map[int]*model.TimeSeriesDataPoint)
		for h := 0; h < 24; h++ {
			hourlyMap[h] = &model.TimeSeriesDataPoint{
				Label:   fmt.Sprintf("%02d:00", h),
				DateKey: fmt.Sprintf("%s %02d", todayStr, h),
			}
		}
		for _, rec := range filtered {
			if rec.Date == todayStr {
				pt := hourlyMap[rec.Hour]
				pt.InputTokens += int64(rec.PromptTokens)
				pt.CachedTokens += int64(rec.CachedTokens)
				pt.ActualOutput += int64(rec.ActualOutputTokens)
				pt.EstimatedRaw += int64(rec.EstimatedRawTokens)
				pt.SavedOutput += int64(rec.SavedOutputTokens)
			}
		}
		for h := 0; h < 24; h++ {
			pt := hourlyMap[h]
			if pt.EstimatedRaw > 0 {
				pt.SavedPct = float64(pt.SavedOutput) / float64(pt.EstimatedRaw) * 100.0
			}
			timeSeries = append(timeSeries, *pt)
		}
	} else {
		// Group by day for week / month
		numDays := 7
		if period == "month" {
			numDays = 30
		}
		dayMap := make(map[string]*model.TimeSeriesDataPoint)
		var orderedKeys []string

		dayNames := []string{"Min", "Sen", "Sel", "Rab", "Kam", "Jum", "Sab"}

		for d := numDays - 1; d >= 0; d-- {
			dt := now.AddDate(0, 0, -d)
			k := dt.Format("2006-01-02")
			label := fmt.Sprintf("%s (%s)", dayNames[dt.Weekday()], dt.Format("02/01"))
			if period == "month" {
				label = dt.Format("02 Jan")
			}
			dayMap[k] = &model.TimeSeriesDataPoint{
				Label:   label,
				DateKey: k,
			}
			orderedKeys = append(orderedKeys, k)
		}

		for _, rec := range filtered {
			if pt, exists := dayMap[rec.Date]; exists {
				pt.InputTokens += int64(rec.PromptTokens)
				pt.CachedTokens += int64(rec.CachedTokens)
				pt.ActualOutput += int64(rec.ActualOutputTokens)
				pt.EstimatedRaw += int64(rec.EstimatedRawTokens)
				pt.SavedOutput += int64(rec.SavedOutputTokens)
			}
		}

		for _, k := range orderedKeys {
			pt := dayMap[k]
			if pt.EstimatedRaw > 0 {
				pt.SavedPct = float64(pt.SavedOutput) / float64(pt.EstimatedRaw) * 100.0
			}
			timeSeries = append(timeSeries, *pt)
		}
	}

	// Recent logs (last 10 records in reverse chronological order)
	var recent []model.TokenUsageRecord
	startIdx := len(filtered) - 1
	for i := startIdx; i >= 0 && len(recent) < 10; i-- {
		recent = append(recent, filtered[i])
	}

	return &model.TokenAnalyticsResponse{
		Period:     period,
		Model:      modelFilter,
		Summary:    summary,
		TimeSeries: timeSeries,
		ByModel:    byModelList,
		RecentLogs: recent,
	}
}

// Fallback Rules Management
func (s *Storage) GetFallbackConfig() *model.FallbackConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.fallbacks == nil {
		return &model.FallbackConfig{
			Enabled: true,
			Rules:   make(map[string][]string),
		}
	}
	res := &model.FallbackConfig{
		Enabled: s.fallbacks.Enabled,
		Rules:   make(map[string][]string),
	}
	for k, v := range s.fallbacks.Rules {
		cp := make([]string, len(v))
		copy(cp, v)
		res.Rules[k] = cp
	}
	return res
}

func (s *Storage) SaveFallbackConfig(cfg *model.FallbackConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fallbacks = cfg
	return s.saveFallbacksLocked()
}

func (s *Storage) saveFallbacksLocked() error {
	if s.fallbacks == nil {
		return nil
	}
	bytes, err := json.MarshalIndent(s.fallbacks, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.fallbacksPath, bytes, 0644)
}

func (s *Storage) GetModelFallbackChain(originalModel string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.fallbacks == nil || !s.fallbacks.Enabled {
		return []string{originalModel}
	}

	chain := []string{originalModel}
	if targets, ok := s.fallbacks.Rules[originalModel]; ok && len(targets) > 0 {
		chain = append(chain, targets...)
	} else if defTargets, ok := s.fallbacks.Rules["default"]; ok && len(defTargets) > 0 {
		chain = append(chain, defTargets...)
	}

	// Remove duplicates while preserving order
	seen := make(map[string]bool)
	var unique []string
	for _, m := range chain {
		m = strings.TrimSpace(m)
		if m != "" && !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}
	return unique
}

func (s *Storage) GetProviderStrategy() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.providerStrategy == "" {
		return "smart_quota"
	}
	return s.providerStrategy
}

func (s *Storage) SetProviderStrategy(strat string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strat != "fill_first" && strat != "least_used" && strat != "round_robin" {
		strat = "smart_quota"
	}
	s.providerStrategy = strat
	if s.strategyPath != "" {
		_ = s.saveProviderStrategyLocked()
	}
}

func (s *Storage) saveProviderStrategyLocked() error {
	sc := model.ProviderStrategyConfig{
		Strategy: s.providerStrategy,
	}
	bytes, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.strategyPath, bytes, 0644)
}

