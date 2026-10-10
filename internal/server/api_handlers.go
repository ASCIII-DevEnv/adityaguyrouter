package server

import (
	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/model"
	"antigravity-gateway/internal/provider"
	"antigravity-gateway/internal/storage"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ApiHandler struct {
	cfg     *config.Config
	storage *storage.Storage
	pool    *provider.AccountPool
	client  *provider.AntigravityClient
}

func NewApiHandler(cfg *config.Config, st *storage.Storage, pool *provider.AccountPool, client *provider.AntigravityClient) *ApiHandler {
	return &ApiHandler{
		cfg:     cfg,
		storage: st,
		pool:    pool,
		client:  client,
	}
}

// OAuth Start
func (h *ApiHandler) HandleAuthLogin(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		host = fmt.Sprintf("localhost:%d", h.cfg.Port)
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	redirectURI := fmt.Sprintf("%s://%s/callback", scheme, host)
	authURL := h.client.BuildAuthURL(redirectURI, "")
	http.Redirect(w, r, authURL, http.StatusFound)
}

// OAuth Callback
func (h *ApiHandler) HandleAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	host := r.Host
	if host == "" {
		host = fmt.Sprintf("localhost:%d", h.cfg.Port)
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	redirectURI := fmt.Sprintf("%s://%s/callback", scheme, host)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	accessToken, refreshToken, expiresAt, err := h.client.ExchangeCode(ctx, code, redirectURI)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "<h2>Login Gagal</h2><p>%v</p><a href='/'>Kembali</a>", err)
		return
	}

	email, name, picture, err := h.client.GetUserInfo(ctx, accessToken)
	if err != nil {
		email = fmt.Sprintf("user_%d@gmail.com", time.Now().Unix())
	}

	projectID, tier, _ := h.client.LoadCodeAssist(ctx, accessToken)

	acc := &model.Account{
		Email:              email,
		Name:               name,
		Picture:            picture,
		Tier:               tier,
		ProjectId:          projectID,
		AccessToken:        accessToken,
		RefreshToken:       refreshToken,
		TokenExpiresAt:     expiresAt,
		IsActive:           true,
		Quota5hPercent:     100.0,
		QuotaWeeklyPercent: 100.0,
	}

	_ = h.storage.UpsertAccount(acc)

	// Fetch initial quotas asynchronously
	go func() {
		bgCtx, bgCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer bgCancel()
		h.pool.FetchAccountQuota(bgCtx, acc)
	}()

	http.Redirect(w, r, "/?auth=success", http.StatusFound)
}

// Accounts API
func (h *ApiHandler) HandleAccounts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		accounts := h.storage.GetAccounts()
		// Redact tokens
		var safeAccs []map[string]interface{}
		for _, a := range accounts {
			safeAccs = append(safeAccs, map[string]interface{}{
				"email":              a.Email,
				"name":               a.Name,
				"picture":            a.Picture,
				"tier":               a.Tier,
				"projectId":          a.ProjectId,
				"isActive":           a.IsActive,
				"order":              a.Order,
				"cooldownUntil":      a.CooldownUntil,
				"cooldownReason":     a.CooldownReason,
				"totalRequests":      a.TotalRequests,
				"quota5hPercent":     a.Quota5hPercent,
				"quota5hResetAt":     a.Quota5hResetAt,
				"quotaWeeklyPercent": a.QuotaWeeklyPercent,
				"quotaWeeklyResetAt": a.QuotaWeeklyResetAt,
				"families":           a.Families,
				"modelsList":         a.ModelsList,
				"lastUsedAt":         a.LastUsedAt,
				"updatedAt":          a.UpdatedAt,
				"tokenExpiresAt":     a.TokenExpiresAt,
				"createdAt":          a.CreatedAt,
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"accounts": safeAccs,
		})

	case http.MethodPost:
		// Toggle account status, clear cooldown, or manual refresh
		var body struct {
			Email    string `json:"email"`
			IsActive *bool  `json:"isActive"`
			Action   string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		acc, ok := h.storage.GetAccount(body.Email)
		if !ok {
			http.Error(w, "Account not found", http.StatusNotFound)
			return
		}

		if body.IsActive != nil {
			acc.IsActive = *body.IsActive
			_ = h.storage.UpsertAccount(acc)
		}

		if body.Action == "clear_cooldown" {
			acc.CooldownUntil = time.Time{}
			acc.CooldownReason = ""
			acc.ConsecutiveErrors = 0
			_ = h.storage.UpsertAccount(acc)
		}

		if body.Action == "refresh" {
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			validAcc, err := h.pool.EnsureValidToken(ctx, acc)
			if err == nil {
				h.pool.FetchAccountQuota(ctx, validAcc)
				if time.Now().After(validAcc.CooldownUntil) {
					validAcc.CooldownReason = ""
					_ = h.storage.UpsertAccount(validAcc)
				}
			}
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})

	case http.MethodDelete:
		email := r.URL.Query().Get("email")
		if email == "" {
			http.Error(w, "email parameter required", http.StatusBadRequest)
			return
		}
		if _, exists := h.storage.GetAccount(email); !exists {
			http.Error(w, "Account not found", http.StatusNotFound)
			return
		}
		h.pool.RemoveAccountSessions(email)
		_ = h.storage.DeleteAccount(email)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})

	case http.MethodPut:
		// Reorder accounts
		var body struct {
			Order []string `json:"order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.storage.ReorderAccounts(body.Order)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Import accounts from OmniRoute SQLite
func (h *ApiHandler) HandleImportOmniRoute(w http.ResponseWriter, r *http.Request) {
	homeDir, _ := os.UserHomeDir()
	sqlitePath := filepath.Join(homeDir, "omniroute/data/storage.sqlite")

	if _, err := os.Stat(sqlitePath); os.IsNotExist(err) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"count":   0,
			"message": "OmniRoute SQLite not found at " + sqlitePath,
		})
		return
	}

	// Read sqlite via simple cli query to avoid CGO sqlite drivers in alpine
	// (we can read raw JSON from sqlite3 cli if present or fall back)
	db, err := sql.Open("sqlite3", sqlitePath)
	importedCount := 0

	if err == nil {
		defer db.Close()
		rows, qErr := db.Query("SELECT email, access_token, refresh_token, token_expires_at, project_id FROM provider_connections WHERE provider = 'antigravity'")
		if qErr == nil {
			defer rows.Close()
			for rows.Next() {
				var email, accessToken, refreshToken, expiresAtStr, projectID string
				if err := rows.Scan(&email, &accessToken, &refreshToken, &expiresAtStr, &projectID); err == nil && email != "" {
					expTime, _ := time.Parse(time.RFC3339, expiresAtStr)
					if expTime.IsZero() {
						expTime = time.Now().Add(1 * time.Hour)
					}
					acc := &model.Account{
						Email:              email,
						AccessToken:        accessToken,
						RefreshToken:       refreshToken,
						TokenExpiresAt:     expTime,
						ProjectId:          projectID,
						IsActive:           true,
						Tier:               "Google AI Pro",
						Quota5hPercent:     100.0,
						QuotaWeeklyPercent: 100.0,
					}
					_ = h.storage.UpsertAccount(acc)
					importedCount++
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"count":   importedCount,
		"message": fmt.Sprintf("Imported %d accounts from OmniRoute", importedCount),
	})
}

// HandleImportLocalSystem auto-detects and imports Google credentials from gcloud / local system
func (h *ApiHandler) HandleImportLocalSystem(w http.ResponseWriter, r *http.Request) {
	homeDir, _ := os.UserHomeDir()
	importedCount := 0

	// Check gcloud application_default_credentials.json
	gcloudPaths := []string{
		filepath.Join(homeDir, ".config/gcloud/application_default_credentials.json"),
		os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
	}

	for _, p := range gcloudPaths {
		if p == "" {
			continue
		}
		if raw, err := os.ReadFile(p); err == nil {
			var cred struct {
				ClientID     string `json:"client_id"`
				ClientSecret string `json:"client_secret"`
				RefreshToken string `json:"refresh_token"`
				Type         string `json:"type"`
			}
			if err := json.Unmarshal(raw, &cred); err == nil && cred.RefreshToken != "" {
				ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
				token, expiresAt, err := h.client.RefreshToken(ctx, cred.RefreshToken)
				cancel()
				if err == nil {
					infoCtx, infoCancel := context.WithTimeout(context.Background(), 10*time.Second)
					email, name, picture, _ := h.client.GetUserInfo(infoCtx, token)
					projID, tier, _ := h.client.LoadCodeAssist(infoCtx, token)
					infoCancel()

					if email == "" {
						email = "gcloud_user@google.com"
					}
					if projID == "" {
						projID = "aicode-consumers"
					}

					acc := &model.Account{
						Email:              email,
						Name:               name,
						Picture:            picture,
						ProjectId:          projID,
						Tier:               tier,
						AccessToken:        token,
						RefreshToken:       cred.RefreshToken,
						TokenExpiresAt:     expiresAt,
						IsActive:           true,
						Quota5hPercent:     100.0,
						QuotaWeeklyPercent: 100.0,
					}
					_ = h.storage.UpsertAccount(acc)
					importedCount++
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"count":   importedCount,
		"message": fmt.Sprintf("Imported %d credentials from local system", importedCount),
	})
}

// API Keys Management
func (h *ApiHandler) HandleApiKeys(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		keys := h.storage.GetApiKeys()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"keys": keys})

	case http.MethodPost:
		var body struct {
			Name         string `json:"name"`
			DefaultSkill string `json:"defaultSkill"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.Name == "" {
			body.Name = "New API Key"
		}
		key, err := h.storage.CreateApiKey(body.Name, body.DefaultSkill)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(key)

	case http.MethodDelete:
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "key parameter required", http.StatusBadRequest)
			return
		}
		if _, exists := h.storage.GetApiKey(key); !exists {
			http.Error(w, "API Key not found", http.StatusNotFound)
			return
		}
		_ = h.storage.DeleteApiKey(key)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Skills Management
func (h *ApiHandler) HandleSkills(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		skills := h.storage.GetSkills()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"skills": skills})

	case http.MethodPost:
		var sk model.Skill
		if err := json.NewDecoder(r.Body).Decode(&sk); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if sk.ID == "" {
			sk.ID = strings.ToLower(strings.ReplaceAll(sk.Name, " ", "-"))
		}
		if err := h.storage.UpsertSkill(&sk); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(sk)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id parameter required", http.StatusBadRequest)
			return
		}
		if err := h.storage.DeleteSkill(id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleTokenAnalytics handles GET /api/analytics/tokens
func (h *ApiHandler) HandleTokenAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "week"
	}
	modelFilter := r.URL.Query().Get("model")

	resp := h.storage.GetTokenAnalytics(period, modelFilter)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleWebhookNotification triggers test notifications to configured webhook URLs
func (h *ApiHandler) HandleWebhookNotification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		WebhookURL string `json:"webhookUrl"`
		Title      string `json:"title"`
		Message    string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if body.WebhookURL == "" {
		http.Error(w, "webhookUrl required", http.StatusBadRequest)
		return
	}
	if body.Title == "" {
		body.Title = "⚡ Antigravity Quota Alert"
	}
	if body.Message == "" {
		body.Message = "Test notifikasi berhasil dikirim dari Antigravity Gateway!"
	}

	notifier := provider.NewNotifier()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	err := notifier.SendWebhook(ctx, body.WebhookURL, body.Title, body.Message)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Notifikasi berhasil terkirim"})
}

// HandleInspector returns recent live request inspection logs
func (h *ApiHandler) HandleInspector(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		records := h.storage.GetInspector().GetRecords()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"records": records})
	case http.MethodDelete:
		h.storage.GetInspector().Clear()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleFallbacks handles GET and POST /api/fallbacks
func (h *ApiHandler) HandleFallbacks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		fb := h.storage.GetFallbackConfig()
		availableModels := []string{
			"gemini-3.8-flash-tiered",
			"gemini-3.7-flash-tiered",
			"claude-sonnet-4-6",
			"claude-opus-4-6-thinking",
			"gpt-oss-120b-medium",
			"gemini-3.8-flash-high",
			"gemini-3.8-flash-medium",
			"gemini-3.8-flash-low",
			"gemini-3.7-flash-high",
			"gemini-3.7-flash-medium",
			"gemini-3.6-flash-high",
			"gemini-3.6-flash-medium",
		}
		resp := map[string]interface{}{
			"enabled":         fb.Enabled,
			"rules":           fb.Rules,
			"availableModels": availableModels,
		}
		_ = json.NewEncoder(w).Encode(resp)

	case http.MethodPost:
		var body struct {
			Enabled bool                `json:"enabled"`
			Rules   map[string][]string `json:"rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg := &model.FallbackConfig{
			Enabled: body.Enabled,
			Rules:   body.Rules,
		}
		if cfg.Rules == nil {
			cfg.Rules = make(map[string][]string)
		}
		if err := h.storage.SaveFallbackConfig(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"enabled": cfg.Enabled,
			"rules":   cfg.Rules,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Provider Strategy API: GET & POST /api/provider/strategy
func (h *ApiHandler) HandleProviderStrategy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		strat := h.storage.GetProviderStrategy()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"strategy": strat,
			"options": []map[string]string{
				{"id": "smart_quota", "name": "Smart Quota (Default)", "desc": "Prioritaskan sisa kuota tertinggi & rotasi cerdas anti rate-limit"},
				{"id": "fill_first", "name": "Fill First (Urutan Akun)", "desc": "Pakai akun urutan pertama sampai habis, lalu berpindah ke akun berikutnya"},
				{"id": "round_robin", "name": "Round Robin (Bergiliran)", "desc": "Rotasi request merata antar seluruh akun aktif"},
				{"id": "least_used", "name": "Least Used (Beban Terendah)", "desc": "Pilih akun dengan jumlah total request paling sedikit"},
			},
		})

	case http.MethodPost:
		var body struct {
			Strategy string `json:"strategy"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.Strategy == "" {
			body.Strategy = "smart_quota"
		}
		h.storage.SetProviderStrategy(body.Strategy)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  true,
			"strategy": body.Strategy,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
