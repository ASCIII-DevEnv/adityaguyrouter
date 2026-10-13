package provider

import (
	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/model"
	"antigravity-gateway/internal/storage"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAccountPoolSessionAffinityAndCleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pool_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		DataDir:        tmpDir,
		AccountsPath:   filepath.Join(tmpDir, "accounts.json"),
		ApiKeysPath:    filepath.Join(tmpDir, "api_keys.json"),
		SkillsPath:     filepath.Join(tmpDir, "skills.json"),
		TokenUsagePath: filepath.Join(tmpDir, "token_usage.json"),
	}

	st, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	acc := &model.Account{
		Email:              "user1@gmail.com",
		IsActive:           true,
		Quota5hPercent:     100,
		QuotaWeeklyPercent: 100,
		TokenExpiresAt:     time.Now().Add(1 * time.Hour),
	}
	if err := st.UpsertAccount(acc); err != nil {
		t.Fatalf("failed to upsert account: %v", err)
	}

	client := NewAntigravityClient(cfg)
	pool := NewAccountPool(cfg, st, client)

	sessionID := "test-session-123"
	selected, err := pool.SelectAccount("", sessionID, nil)
	if err != nil {
		t.Fatalf("expected successful selection, got error: %v", err)
	}
	if selected.Email != "user1@gmail.com" {
		t.Fatalf("expected user1@gmail.com, got %s", selected.Email)
	}

	// Verify session registered
	pool.sessionsMu.RLock()
	mappedEmail := pool.sessions[sessionID]
	pool.sessionsMu.RUnlock()
	if mappedEmail != "user1@gmail.com" {
		t.Fatalf("expected session mapped to user1@gmail.com, got %s", mappedEmail)
	}

	// Remove session affinity on account deletion
	pool.RemoveAccountSessions("user1@gmail.com")

	pool.sessionsMu.RLock()
	_, stillExists := pool.sessions[sessionID]
	pool.sessionsMu.RUnlock()
	if stillExists {
		t.Fatalf("expected session affinity to be expunged")
	}
}

func TestModelExhaustionUninitializedFamily(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pool_test_exhaust_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		DataDir:        tmpDir,
		AccountsPath:   filepath.Join(tmpDir, "accounts.json"),
		ApiKeysPath:    filepath.Join(tmpDir, "api_keys.json"),
		SkillsPath:     filepath.Join(tmpDir, "skills.json"),
		TokenUsagePath: filepath.Join(tmpDir, "token_usage.json"),
	}

	st, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	// Account with uninitialized family limits (RemainingPercent 0 but empty DisplayName)
	// and positive general account quotas
	acc := &model.Account{
		Email:              "user2@gmail.com",
		IsActive:           true,
		Quota5hPercent:     100,
		QuotaWeeklyPercent: 85,
		TokenExpiresAt:     time.Now().Add(1 * time.Hour),
		Families: map[string]model.Family{
			"gemini": {
				ID:   "gemini",
				Name: "Gemini Models",
				Limit5h: model.FamilyLimit{
					RemainingPercent: 0,
					DisplayName:      "",
				},
				LimitWeekly: model.FamilyLimit{
					RemainingPercent: 0,
					DisplayName:      "",
				},
			},
		},
	}
	if err := st.UpsertAccount(acc); err != nil {
		t.Fatalf("failed to upsert account: %v", err)
	}

	client := NewAntigravityClient(cfg)
	pool := NewAccountPool(cfg, st, client)

	// Should not be treated as exhausted since DisplayName is empty (uninitialized)
	selected, err := pool.SelectAccount("", "", nil, "gemini-3.8-flash-high")
	if err != nil {
		t.Fatalf("expected account to be selectable, got: %v", err)
	}
	if selected.Email != "user2@gmail.com" {
		t.Fatalf("expected user2@gmail.com, got %s", selected.Email)
	}

	// Now explicitly mark as exhausted with real DisplayName and ResetAt
	acc.Families["gemini"] = model.Family{
		ID:   "gemini",
		Name: "Gemini Models",
		LimitWeekly: model.FamilyLimit{
			RemainingPercent: 0,
			DisplayName:      "Gemini Weekly",
			ResetAt:          "2026-10-15T00:00:00Z",
		},
	}
	_ = st.UpsertAccount(acc)

	_, err = pool.SelectAccount("", "", nil, "gemini-3.8-flash-high")
	if err == nil {
		t.Fatalf("expected error when model is genuinely exhausted, got nil")
	}
}

func TestIndependentFamilyQuotaSelection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pool_test_independent_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		DataDir:        tmpDir,
		AccountsPath:   filepath.Join(tmpDir, "accounts.json"),
		ApiKeysPath:    filepath.Join(tmpDir, "api_keys.json"),
		SkillsPath:     filepath.Join(tmpDir, "skills.json"),
		TokenUsagePath: filepath.Join(tmpDir, "token_usage.json"),
	}

	st, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}
	st.SetProviderStrategy("smart_quota")

	// Account where Claude is low (10% < 15%) but Gemini is high (90% > 15%)
	acc := &model.Account{
		Email:              "multi_user@gmail.com",
		IsActive:           true,
		Quota5hPercent:     10.0, // Historical min from Claude
		QuotaWeeklyPercent: 80.0,
		TokenExpiresAt:     time.Now().Add(1 * time.Hour),
		Families: map[string]model.Family{
			"claude": {
				ID:   "claude",
				Name: "Claude Models",
				Limit5h: model.FamilyLimit{
					DisplayName:      "Claude 5h",
					RemainingPercent: 10.0,
					ResetAt:          "2026-10-12T15:00:00Z",
				},
				LimitWeekly: model.FamilyLimit{
					DisplayName:      "Claude Weekly",
					RemainingPercent: 50.0,
					ResetAt:          "2026-10-15T00:00:00Z",
				},
			},
			"gemini": {
				ID:   "gemini",
				Name: "Gemini Models",
				Limit5h: model.FamilyLimit{
					DisplayName:      "Gemini 5h",
					RemainingPercent: 90.0,
					ResetAt:          "2026-10-12T16:00:00Z",
				},
				LimitWeekly: model.FamilyLimit{
					DisplayName:      "Gemini Weekly",
					RemainingPercent: 80.0,
					ResetAt:          "2026-10-18T00:00:00Z",
				},
			},
		},
	}
	if err := st.UpsertAccount(acc); err != nil {
		t.Fatalf("failed to upsert: %v", err)
	}

	client := NewAntigravityClient(cfg)
	pool := NewAccountPool(cfg, st, client)

	// Gemini request should succeed using the healthy 90% Gemini quota
	selected, err := pool.SelectAccount("", "", nil, "gemini-3.8-flash-high")
	if err != nil {
		t.Fatalf("expected Gemini to select account, got error: %v", err)
	}
	if selected.Email != "multi_user@gmail.com" {
		t.Fatalf("expected multi_user@gmail.com, got %s", selected.Email)
	}
}

func TestFillFirstWeeklyExhaustionAccountTransition(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pool_test_fill_first_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		DataDir:        tmpDir,
		AccountsPath:   filepath.Join(tmpDir, "accounts.json"),
		ApiKeysPath:    filepath.Join(tmpDir, "api_keys.json"),
		SkillsPath:     filepath.Join(tmpDir, "skills.json"),
		TokenUsagePath: filepath.Join(tmpDir, "token_usage.json"),
	}

	st, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}
	st.SetProviderStrategy("fill_first")

	// Account 1 (Order 0): Gemini 0% (habis mingguan), Claude 70%
	acc1 := &model.Account{
		Email:          "acc1_priority@gmail.com",
		Order:          0,
		IsActive:       true,
		TokenExpiresAt: time.Now().Add(1 * time.Hour),
		Families: map[string]model.Family{
			"gemini": {
				ID:   "gemini",
				Name: "Gemini",
				LimitWeekly: model.FamilyLimit{
					DisplayName:      "Gemini Weekly",
					RemainingPercent: 0.0,
					ResetAt:          "2026-10-18T00:00:00Z",
				},
			},
			"claude": {
				ID:   "claude",
				Name: "Claude",
				LimitWeekly: model.FamilyLimit{
					DisplayName:      "Claude Weekly",
					RemainingPercent: 70.0,
					ResetAt:          "2026-10-18T00:00:00Z",
				},
			},
		},
	}

	// Account 2 (Order 1): Gemini 50% (masih ada), Claude 10%
	acc2 := &model.Account{
		Email:          "acc2_secondary@gmail.com",
		Order:          1,
		IsActive:       true,
		TokenExpiresAt: time.Now().Add(1 * time.Hour),
		Families: map[string]model.Family{
			"gemini": {
				ID:   "gemini",
				Name: "Gemini",
				LimitWeekly: model.FamilyLimit{
					DisplayName:      "Gemini Weekly",
					RemainingPercent: 50.0,
					ResetAt:          "2026-10-18T00:00:00Z",
				},
			},
			"claude": {
				ID:   "claude",
				Name: "Claude",
				LimitWeekly: model.FamilyLimit{
					DisplayName:      "Claude Weekly",
					RemainingPercent: 10.0,
					ResetAt:          "2026-10-18T00:00:00Z",
				},
			},
		},
	}

	_ = st.UpsertAccount(acc1)
	_ = st.UpsertAccount(acc2)

	client := NewAntigravityClient(cfg)
	pool := NewAccountPool(cfg, st, client)

	// 1. Request Gemini -> Harus melewati Acc 1 dan memilih Acc 2
	selGemini, err := pool.SelectAccount("", "", nil, "gemini-3.8-flash-tiered")
	if err != nil {
		t.Fatalf("expected Gemini selection to succeed, got: %v", err)
	}
	if selGemini.Email != "acc2_secondary@gmail.com" {
		t.Fatalf("expected acc2 for gemini because acc1 weekly is 0%%, got: %s", selGemini.Email)
	}

	// 2. Request Claude -> Harus memilih Acc 1 karena urutan prioritas Order 0 & Claude masih 70%
	selClaude, err := pool.SelectAccount("", "", nil, "claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("expected Claude selection to succeed, got: %v", err)
	}
	if selClaude.Email != "acc1_priority@gmail.com" {
		t.Fatalf("expected acc1 for claude because acc1 has priority Order 0, got: %s", selClaude.Email)
	}
}


