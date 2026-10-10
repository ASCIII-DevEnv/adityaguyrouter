package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Notifier struct {
	client *http.Client
}

func NewNotifier() *Notifier {
	return &Notifier{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// SendWebhook dispatches formatted notification alerts to Telegram or Discord webhooks
func (n *Notifier) SendWebhook(ctx context.Context, webhookURL string, title string, message string) error {
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return nil
	}

	var payload []byte
	var err error

	if strings.Contains(webhookURL, "discord.com/api/webhooks") {
		// Discord Webhook Payload
		discordData := map[string]interface{}{
			"username": "Antigravity Gateway",
			"embeds": []map[string]interface{}{
				{
					"title":       title,
					"description": message,
					"color":       3066993, // Emerald green
					"timestamp":   time.Now().Format(time.RFC3339),
				},
			},
		}
		payload, err = json.Marshal(discordData)
	} else if strings.Contains(webhookURL, "api.telegram.org") {
		// Telegram Bot sendMessage API Payload
		u, parseErr := url.Parse(webhookURL)
		chatID := ""
		if parseErr == nil {
			chatID = u.Query().Get("chat_id")
		}

		// Clean query parameters from URL for clean POST
		if u != nil && chatID != "" {
			u.RawQuery = ""
			webhookURL = u.String()
		}

		telegramData := map[string]interface{}{
			"chat_id":    chatID,
			"text":       fmt.Sprintf("🔔 *%s*\n\n%s", title, message),
			"parse_mode": "Markdown",
		}
		payload, err = json.Marshal(telegramData)
	} else {
		// Standard Generic webhook
		genericData := map[string]interface{}{
			"title":     title,
			"message":   message,
			"timestamp": time.Now().Format(time.RFC3339),
		}
		payload, err = json.Marshal(genericData)
	}

	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		var errResp struct {
			Description string `json:"description"`
		}
		_ = json.NewDecoder(res.Body).Decode(&errResp)
		if errResp.Description != "" {
			return fmt.Errorf("HTTP %d: %s", res.StatusCode, errResp.Description)
		}
		return fmt.Errorf("webhook responded with HTTP %d", res.StatusCode)
	}

	return nil
}
