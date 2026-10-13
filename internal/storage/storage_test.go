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

func TestStorageSkillOperations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage_skill_test_*")
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

	// 1. Initial skills should include built-in
	skillsList := st.GetSkills()
	if len(skillsList) == 0 {
		t.Fatalf("expected built-in skills to be seeded")
	}

	// 2. Toggle built-in skill (e.g. caveman)
	caveman, ok := st.GetSkill("caveman")
	if !ok || !caveman.Enabled {
		t.Fatalf("expected caveman to exist and be enabled initially")
	}

	toggled, err := st.ToggleSkill("caveman")
	if err != nil {
		t.Fatalf("failed to toggle skill: %v", err)
	}
	if toggled.Enabled {
		t.Fatalf("expected caveman to be disabled after toggle")
	}

	// 3. Re-init storage: disabled state should be preserved across restart
	st2, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("failed to re-init storage: %v", err)
	}
	caveman2, ok := st2.GetSkill("caveman")
	if !ok {
		t.Fatalf("caveman missing after reload")
	}
	if caveman2.Enabled {
		t.Fatalf("expected caveman to remain disabled after reload")
	}

	// 4. Custom skill CRUD
	customSkill := &model.Skill{
		ID:           "test-custom",
		Name:         "Test Custom",
		Description:  "Description for test",
		SystemPrompt: "Test Prompt",
		Enabled:      true,
	}
	if err := st2.UpsertSkill(customSkill); err != nil {
		t.Fatalf("failed to upsert custom skill: %v", err)
	}

	if _, ok := st2.GetSkill("test-custom"); !ok {
		t.Fatalf("expected test-custom to exist")
	}

	// Delete custom skill
	if err := st2.DeleteSkill("test-custom"); err != nil {
		t.Fatalf("failed to delete custom skill: %v", err)
	}
	if _, ok := st2.GetSkill("test-custom"); ok {
		t.Fatalf("expected test-custom to be deleted")
	}

	// 5. Delete built-in skill (permanently deleted from map)
	if err := st2.DeleteSkill("caveman"); err != nil {
		t.Fatalf("failed to delete built-in skill: %v", err)
	}
	if _, ok := st2.GetSkill("caveman"); ok {
		t.Fatalf("expected caveman to be deleted")
	}

	// 6. Reset builtins
	if err := st2.ResetBuiltinSkills(); err != nil {
		t.Fatalf("failed to reset builtin skills: %v", err)
	}
	cavemanRestored, ok := st2.GetSkill("caveman")
	if !ok || !cavemanRestored.Enabled {
		t.Fatalf("expected caveman to be restored and enabled after reset")
	}
}

func TestStorageProviderStrategyPersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage_strategy_test_*")
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
		StrategyPath:   filepath.Join(tmpDir, "provider_strategy.json"),
	}

	st, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	// 1. Default should be smart_quota
	if strat := st.GetProviderStrategy(); strat != "smart_quota" {
		t.Fatalf("expected default smart_quota, got %s", strat)
	}

	// 2. Set to fill_first and verify
	st.SetProviderStrategy("fill_first")
	if strat := st.GetProviderStrategy(); strat != "fill_first" {
		t.Fatalf("expected fill_first, got %s", strat)
	}

	// 3. Re-initialize storage (simulate server restart)
	st2, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("failed to re-init storage: %v", err)
	}

	// Strategy must remain fill_first across restart
	if strat := st2.GetProviderStrategy(); strat != "fill_first" {
		t.Fatalf("expected fill_first to persist across restart, got %s", strat)
	}

	// 4. Invalid strategy falls back to smart_quota
	st2.SetProviderStrategy("non_existent_method")
	if strat := st2.GetProviderStrategy(); strat != "smart_quota" {
		t.Fatalf("expected non_existent_method to fallback to smart_quota, got %s", strat)
	}
}
