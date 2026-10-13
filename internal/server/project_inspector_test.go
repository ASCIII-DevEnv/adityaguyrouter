package server

import (
	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/provider"
	"antigravity-gateway/internal/storage"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectsInspectionEndpoints(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "projects_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create dummy project folders
	proj1 := filepath.Join(tmpDir, "test-app-go")
	_ = os.MkdirAll(proj1, 0755)
	_ = os.WriteFile(filepath.Join(proj1, "go.mod"), []byte("module testapp\n\ngo 1.24\n"), 0644)
	_ = os.WriteFile(filepath.Join(proj1, "README.md"), []byte("# Test App Go\nApp deskripsi singkat.\n"), 0644)

	proj2 := filepath.Join(tmpDir, "test-app-node")
	_ = os.MkdirAll(proj2, 0755)
	_ = os.WriteFile(filepath.Join(proj2, "package.json"), []byte(`{"name":"test-node","version":"1.0.0"}`), 0644)

	cfg := &config.Config{
		DataDir:        tmpDir,
		ProjectsDir:    tmpDir,
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

	// 1. Test GET /api/projects
	req := httptest.NewRequest("GET", "/api/projects", nil)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var pResp struct {
		Projects []struct {
			Name      string `json:"name"`
			TechStack string `json:"techStack"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pResp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(pResp.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(pResp.Projects))
	}

	// 2. Test GET /api/projects/context
	reqCtx := httptest.NewRequest("GET", "/api/projects/context?name=test-app-go", nil)
	recCtx := httptest.NewRecorder()
	rt.ServeHTTP(recCtx, reqCtx)

	if recCtx.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for context, got %d", recCtx.Code)
	}

	var ctxResp struct {
		ProjectName string   `json:"projectName"`
		TechStack   string   `json:"techStack"`
		KeyFiles    []string `json:"keyFiles"`
		SummaryText string   `json:"summaryText"`
	}
	if err := json.Unmarshal(recCtx.Body.Bytes(), &ctxResp); err != nil {
		t.Fatalf("unmarshal context error: %v", err)
	}
	if ctxResp.ProjectName != "test-app-go" {
		t.Fatalf("expected project name test-app-go, got %s", ctxResp.ProjectName)
	}
	if ctxResp.TechStack != "Go" {
		t.Fatalf("expected tech stack Go, got %s", ctxResp.TechStack)
	}
}
