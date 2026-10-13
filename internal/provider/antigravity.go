package provider

import (
	"antigravity-gateway/internal/config"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type AntigravityClient struct {
	cfg          *config.Config
	httpClient   *http.Client
	streamClient *http.Client
}

func NewAntigravityClient(cfg *config.Config) *AntigravityClient {
	return &AntigravityClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
		streamClient: &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   15 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
			// Timeout is 0 (unlimited for streaming, governed by ctx)
		},
	}
}

// BuildAuthURL constructs Google OAuth login redirect URL
func (c *AntigravityClient) BuildAuthURL(redirectURI, state string) string {
	v := url.Values{}
	v.Set("client_id", c.cfg.OAuthClientID)
	v.Set("response_type", "code")
	v.Set("redirect_uri", redirectURI)
	v.Set("scope", strings.Join(c.cfg.OAuthScopes, " "))
	v.Set("state", state)
	v.Set("access_type", "offline")
	v.Set("prompt", "consent")
	return fmt.Sprintf("%s?%s", c.cfg.OAuthAuthURL, v.Encode())
}

// ExchangeCode trades OAuth authorization code for access & refresh tokens
func (c *AntigravityClient) ExchangeCode(ctx context.Context, code, redirectURI string) (string, string, time.Time, error) {
	v := url.Values{}
	v.Set("grant_type", "authorization_code")
	v.Set("client_id", c.cfg.OAuthClientID)
	v.Set("client_secret", c.cfg.OAuthClientSecret)
	v.Set("code", code)
	v.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.OAuthTokenURL, strings.NewReader(v.Encode()))
	if err != nil {
		return "", "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", time.Time{}, err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return "", "", time.Time{}, fmt.Errorf("token exchange failed (%d): %s", res.StatusCode, string(body))
	}

	var data struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", "", time.Time{}, err
	}

	expiresAt := time.Now().Add(time.Duration(data.ExpiresIn) * time.Second)
	return data.AccessToken, data.RefreshToken, expiresAt, nil
}

// RefreshToken renews an expired access token
func (c *AntigravityClient) RefreshToken(ctx context.Context, refreshToken string) (string, time.Time, error) {
	if refreshToken == "" {
		return "", time.Time{}, fmt.Errorf("empty refresh token")
	}

	v := url.Values{}
	v.Set("grant_type", "refresh_token")
	v.Set("client_id", c.cfg.OAuthClientID)
	v.Set("client_secret", c.cfg.OAuthClientSecret)
	v.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.OAuthTokenURL, strings.NewReader(v.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("refresh token failed (%d): %s", res.StatusCode, string(body))
	}

	var data struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", time.Time{}, err
	}

	expiresAt := time.Now().Add(time.Duration(data.ExpiresIn) * time.Second)
	return data.AccessToken, expiresAt, nil
}

// GetUserInfo fetches email, name, picture for the authenticated user
func (c *AntigravityClient) GetUserInfo(ctx context.Context, accessToken string) (email, name, picture string, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.cfg.OAuthUserInfoURL+"?alt=json", nil)
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return "", "", "", fmt.Errorf("userinfo failed (%d): %s", res.StatusCode, string(body))
	}

	var u struct {
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.NewDecoder(res.Body).Decode(&u); err != nil {
		return "", "", "", err
	}
	return u.Email, u.Name, u.Picture, nil
}

// LoadCodeAssist resolves Google Cloud Companion project ID & tier
func (c *AntigravityClient) LoadCodeAssist(ctx context.Context, accessToken string) (projectID, tier string, err error) {
	payload := map[string]interface{}{
		"metadata": map[string]interface{}{
			"ideType":    9,
			"platform":   3,
			"pluginType": 2,
		},
	}
	pBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.LoadCodeAssistURL, bytes.NewReader(pBytes))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("x-request-source", "local")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return "", "", fmt.Errorf("loadCodeAssist failed (%d): %s", res.StatusCode, string(body))
	}

	var resp struct {
		CloudaicompanionProject interface{} `json:"cloudaicompanionProject"`
		CurrentTier             struct {
			Name string `json:"name"`
		} `json:"currentTier"`
		PaidTier struct {
			Name string `json:"name"`
		} `json:"paidTier"`
	}
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return "", "", err
	}

	// project can be a string or {"id": "..."}
	switch v := resp.CloudaicompanionProject.(type) {
	case string:
		projectID = v
	case map[string]interface{}:
		if id, ok := v["id"].(string); ok {
			projectID = id
		}
	}

	tier = resp.CurrentTier.Name
	if tier == "" {
		tier = resp.PaidTier.Name
	}
	if tier == "" {
		tier = "Standard"
	}

	return projectID, tier, nil
}

// CallCloudCode makes authenticated requests to Google Cloud Code PA endpoint with failover
func (c *AntigravityClient) CallCloudCode(ctx context.Context, path string, accessToken, projectID string, body []byte) (*http.Response, error) {
	// Production CloudCode is the primary endpoint; daily staging is optional fallback
	baseURLs := []string{c.cfg.CloudCodeBaseURL, c.cfg.DailyCloudCodeBaseURL}

	var lastErr error
	for i, base := range baseURLs {
		if base == "" {
			continue
		}
		targetURL := fmt.Sprintf("%s%s", base, path)
		req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", c.cfg.UserAgent)
		req.Header.Set("X-Client-Name", c.cfg.ClientName)
		req.Header.Set("X-Client-Version", c.cfg.ClientVersion)

		client := c.httpClient
		if strings.Contains(path, "stream") {
			client = c.streamClient
		}

		res, err := client.Do(req)
		if err == nil {
			// If upstream returns 200 OK or 400 Bad Request / 422 (client format error), return immediately
			if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusUnprocessableEntity {
				return res, nil
			}
			// If this host returns 403 Forbidden, 404 Not Found, 429, or 5xx, and there is another host:
			if i < len(baseURLs)-1 {
				res.Body.Close()
				lastErr = fmt.Errorf("upstream error (%d) from %s, trying failover", res.StatusCode, base)
				continue
			}
			return res, nil
		}
		lastErr = err
	}

	return nil, lastErr
}
