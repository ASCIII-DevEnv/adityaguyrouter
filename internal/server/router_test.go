package server

import (
	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/provider"
	"antigravity-gateway/internal/storage"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRouterClientAuthValidation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "router_test_*")
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

	client := provider.NewAntigravityClient(cfg)
	pool := provider.NewAccountPool(cfg, st, client)
	rt := NewRouter(cfg, st, pool, client)

	// 1. Spoofed playground request without browser same-origin header should fail
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req1.Header.Set("x-playground-request", "true")
	if rt.validateClientAuth(req1) {
		t.Fatalf("expected spoofed playground request without browser same-origin headers to be rejected")
	}

	// 2. Legitimate same-origin playground request should succeed
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req2.Host = "localhost:3456"
	req2.Header.Set("x-playground-request", "true")
	req2.Header.Set("Sec-Fetch-Site", "same-origin")
	if !rt.validateClientAuth(req2) {
		t.Fatalf("expected same-origin playground request to pass")
	}

	// 3. Unauthorized request triggers writeUnauthorized
	rec := httptest.NewRecorder()
	rt.writeUnauthorized(rec)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected application/json content-type")
	}
}

func TestSkillsAPIEndpoints(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "skills_api_test_*")
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

	client := provider.NewAntigravityClient(cfg)
	pool := provider.NewAccountPool(cfg, st, client)
	rt := NewRouter(cfg, st, pool, client)

	// 1. GET /api/skills
	req := httptest.NewRequest("GET", "/api/skills", nil)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET /api/skills, got %d", rec.Code)
	}

	// 2. POST /api/skills?action=toggle&id=caveman
	reqToggle := httptest.NewRequest("POST", "/api/skills?action=toggle&id=caveman", nil)
	recToggle := httptest.NewRecorder()
	rt.ServeHTTP(recToggle, reqToggle)
	if recToggle.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for toggle, got %d: %s", recToggle.Code, recToggle.Body.String())
	}

	// 3. DELETE /api/skills?id=caveman
	reqDel := httptest.NewRequest("DELETE", "/api/skills?id=caveman", nil)
	recDel := httptest.NewRecorder()
	rt.ServeHTTP(recDel, reqDel)
	if recDel.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for delete, got %d: %s", recDel.Code, recDel.Body.String())
	}

	// 4. POST /api/skills?action=reset_builtins
	reqReset := httptest.NewRequest("POST", "/api/skills?action=reset_builtins", nil)
	recReset := httptest.NewRecorder()
	rt.ServeHTTP(recReset, reqReset)
	if recReset.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for reset, got %d: %s", recReset.Code, recReset.Body.String())
	}
}

func TestAccountsSyncEndpoint(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "accounts_sync_test_*")
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

	client := provider.NewAntigravityClient(cfg)
	pool := provider.NewAccountPool(cfg, st, client)
	rt := NewRouter(cfg, st, pool, client)

	// POST /api/accounts/sync
	req := httptest.NewRequest("POST", "/api/accounts/sync", nil)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/accounts/sync, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPlaygroundGuardrailInjection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "guardrail_test_*")
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

	client := provider.NewAntigravityClient(cfg)
	pool := provider.NewAccountPool(cfg, st, client)
	gh := NewGatewayHandler(cfg, st, pool, client)

	// 1. Without x-playground-request header
	reqNormal := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	promptNormal := gh.applyPlaygroundGuardrail(reqNormal, "initial prompt")
	if promptNormal != "initial prompt" {
		t.Fatalf("expected untouched prompt, got: %s", promptNormal)
	}

	// 2. With x-playground-request header
	reqPg := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	reqPg.Header.Set("x-playground-request", "true")
	promptPg := gh.applyPlaygroundGuardrail(reqPg, "initial prompt")
	if !strings.Contains(promptPg, "IDENTITAS & CORE PRINCIPLES: GREAT SAGE (CIEL)") {
		t.Fatalf("expected guardrail injected, got: %s", promptPg)
	}
	if !strings.Contains(promptPg, "ZERO EXECUTABLE CODE") {
		t.Fatalf("expected no-code rule injected")
	}
	if !strings.Contains(promptPg, "ELITE PROMPT ENGINEER UNTUK AI AGENT") {
		t.Fatalf("expected elite prompt engineer capability injected")
	}
}
