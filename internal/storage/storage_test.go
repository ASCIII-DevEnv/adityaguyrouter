package storage

import (
	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestStorageRecordTokenUsageAndFlush(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage_test_*")
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

	st, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	// Record token usage
	st.RecordTokenUsage(&model.TokenUsageRecord{
		ID:                 "rec_test_1",
		Model:              "gemini-3.8-flash-tiered",
		Account:            "test@example.com",
		ActualOutputTokens: 100,
		PromptTokens:       200,
	})

	if !st.tokenUsageDirty {
		t.Fatalf("expected tokenUsageDirty to be true")
	}

	// Trigger manual flush
	st.FlushTokenUsage()

	if st.tokenUsageDirty {
		t.Fatalf("expected tokenUsageDirty to be false after flush")
	}

	// Verify file was written to disk
	if _, err := os.Stat(cfg.TokenUsagePath); os.IsNotExist(err) {
		t.Fatalf("expected token_usage.json to exist on disk")
	}
}
