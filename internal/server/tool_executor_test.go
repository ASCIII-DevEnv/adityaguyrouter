package server

import (
	"testing"
)

func TestExecuteToolValidation(t *testing.T) {
	// 1. Empty query for web_search
	_, err := ExecuteWebSearch("")
	if err == nil {
		t.Fatalf("expected error for empty search query")
	}

	// 2. Empty URL for fetch_web_page
	_, err = ExecuteFetchWebPage("")
	if err == nil {
		t.Fatalf("expected error for empty URL")
	}

	// 3. Empty repo for github_inspect
	_, err = ExecuteGithubInspect("", "")
	if err == nil {
		t.Fatalf("expected error for empty repo")
	}
}

func TestResolveDoHIP(t *testing.T) {
	// Test DoH resolver
	ip, err := resolveDoHIP("html.duckduckgo.com")
	if err != nil {
		t.Logf("DoH resolution warning (network dependent): %v", err)
	} else if ip == "" {
		t.Fatalf("expected non-empty IP from DoH")
	}
}
