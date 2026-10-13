package server

import (
	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/provider"
	"antigravity-gateway/internal/storage"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Router struct {
	cfg        *config.Config
	storage    *storage.Storage
	gateway    *GatewayHandler
	api        *ApiHandler
	staticPath string
}

func NewRouter(cfg *config.Config, st *storage.Storage, pool *provider.AccountPool, client *provider.AntigravityClient) *Router {
	return &Router{
		cfg:        cfg,
		storage:    st,
		gateway:    NewGatewayHandler(cfg, st, pool, client),
		api:        NewApiHandler(cfg, st, pool, client),
		staticPath: "./public",
	}
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS Headers for IDE extensions & browser tools
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key, x-skill, x-session-id, conversation-id")
	w.Header().Set("Access-Control-Expose-Headers", "x-fallback-model, x-original-model, x-account, x-model")
	w.Header().Set("Referrer-Policy", "no-referrer")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	p := r.URL.Path

	// 1. Gateway LLM Endpoints
	if p == "/v1/chat/completions" {
		if !rt.validateClientAuth(r) {
			rt.writeUnauthorized(w)
			return
		}
		rt.gateway.HandleChatCompletions(w, r)
		return
	}

	if p == "/v1/messages" {
		if !rt.validateClientAuth(r) {
			rt.writeUnauthorized(w)
			return
		}
		rt.gateway.HandleMessages(w, r)
		return
	}

	if p == "/v1/models" {
		rt.gateway.HandleModels(w, r)
		return
	}

	// 2. Dashboard REST APIs
	if p == "/api/auth/login" {
		rt.api.HandleAuthLogin(w, r)
		return
	}
	if p == "/callback" {
		rt.api.HandleAuthCallback(w, r)
		return
	}
	if p == "/api/accounts" {
		rt.api.HandleAccounts(w, r)
		return
	}
	if p == "/api/accounts/sync" {
		rt.api.HandleAccountsSync(w, r)
		return
	}
	if p == "/api/projects" {
		rt.api.HandleProjects(w, r)
		return
	}
	if p == "/api/projects/context" {
		rt.api.HandleProjectContext(w, r)
		return
	}
	if p == "/api/accounts/import-local" {
		rt.api.HandleImportLocalSystem(w, r)
		return
	}
	if p == "/api/keys" {
		rt.api.HandleApiKeys(w, r)
		return
	}
	if p == "/api/skills" || p == "/api/skills/toggle" {
		rt.api.HandleSkills(w, r)
		return
	}
	if p == "/api/analytics/tokens" {
		rt.api.HandleTokenAnalytics(w, r)
		return
	}
	if p == "/api/notifications/webhook" {
		rt.api.HandleWebhookNotification(w, r)
		return
	}
	if p == "/api/inspector" {
		rt.api.HandleInspector(w, r)
		return
	}
	if p == "/api/fallbacks" {
		rt.api.HandleFallbacks(w, r)
		return
	}
	if p == "/api/provider/strategy" {
		rt.api.HandleProviderStrategy(w, r)
		return
	}
	if p == "/api/tools/execute" {
		rt.api.HandleToolExecute(w, r)
		return
	}
	if p == "/api/diagnostics" {
		rt.api.HandleDiagnostics(w, r)
		return
	}

	// 3. Static Web Dashboard Files
	if p == "/" || p == "/index.html" {
		indexPath := filepath.Join(rt.staticPath, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			http.ServeFile(w, r, indexPath)
			return
		}
	}

	// Fallback static files
	filePath := filepath.Join(rt.staticPath, p)
	if _, err := os.Stat(filePath); err == nil {
		http.ServeFile(w, r, filePath)
		return
	}

	http.NotFound(w, r)
}

func (rt *Router) validateClientAuth(r *http.Request) bool {
	// Allow internal calls from same-origin web playground
	if r.Header.Get("x-playground-request") == "true" {
		secSite := r.Header.Get("Sec-Fetch-Site")
		referer := r.Header.Get("Referer")
		origin := r.Header.Get("Origin")
		if secSite == "same-origin" || (referer != "" && strings.Contains(referer, r.Host)) || (origin != "" && strings.Contains(origin, r.Host)) {
			return true
		}
	}

	auth := r.Header.Get("Authorization")
	key := ""
	if strings.HasPrefix(auth, "Bearer ") {
		key = strings.TrimPrefix(auth, "Bearer ")
	} else if k := r.Header.Get("x-api-key"); k != "" {
		key = k
	}

	// Check if key is valid
	if key != "" {
		_, ok := rt.storage.ValidateApiKey(key)
		return ok
	}

	// If no keys configured, allow default local access
	keys := rt.storage.GetApiKeys()
	return len(keys) == 0
}

func (rt *Router) writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"type":"invalid_api_key","message":"Missing or invalid API Key. Include 'Authorization: Bearer <key>' or 'x-api-key: <key>'"}}` + "\n"))
}
