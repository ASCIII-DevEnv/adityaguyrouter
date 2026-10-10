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
