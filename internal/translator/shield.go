package translator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var (
	// Regex patterns for sensitive credentials & secrets
	secretRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:password|passwd|pwd|secret|api_key|apikey|token|auth_token|access_token|private_key)\s*[:=]\s*["']?([a-zA-Z0-9_\-\.]{12,})["']?`),
		regexp.MustCompile(`(?i)(postgres(?:ql)?|mysql|mongodb(?:\+srv)?|redis)://[a-zA-Z0-9_\.\-]+:([^@\s]{4,})@`),
		regexp.MustCompile(`-----BEGIN (?:RSA |EC )?PRIVATE KEY-----[\s\S]+?-----END (?:RSA |EC )?PRIVATE KEY-----`),
		regexp.MustCompile(`(?i)\b(?:sk-[a-zA-Z0-9]{20,}|ya29\.[a-zA-Z0-9_\-]{40,}|ghp_[a-zA-Z0-9]{36}|xox[baprs]-[a-zA-Z0-9]{10,})\b`),
	}
)

type SecretShield struct {
	mu            sync.RWMutex
	secretToToken map[string]string // raw secret -> [REDACTED_SECRET_...]
	tokenToSecret map[string]string // [REDACTED_SECRET_...] -> raw secret
}

func NewSecretShield() *SecretShield {
	return &SecretShield{
		secretToToken: make(map[string]string),
		tokenToSecret: make(map[string]string),
	}
}

// Redact replaces detected secret patterns with reversible tokens
func (s *SecretShield) Redact(text string) string {
	if text == "" {
		return text
	}

	result := text
	for _, re := range secretRegexes {
		matches := re.FindAllStringSubmatch(result, -1)
		for _, m := range matches {
			secretVal := ""
			if len(m) > 2 && m[2] != "" {
				secretVal = m[2] // DB password
			} else if len(m) > 1 && m[1] != "" {
				secretVal = m[1] // Key/Token
			} else if len(m) > 0 {
				secretVal = m[0] // Full matched secret
			}

			if len(secretVal) < 8 {
				continue
			}

			s.mu.Lock()
			token, exists := s.secretToToken[secretVal]
			if !exists {
				hash := sha256.Sum256([]byte(secretVal))
				token = fmt.Sprintf("[REDACTED_SECRET_%s]", hex.EncodeToString(hash[:4]))
				s.secretToToken[secretVal] = token
				s.tokenToSecret[token] = secretVal
			}
			s.mu.Unlock()

			result = strings.ReplaceAll(result, secretVal, token)
		}
	}

	return result
}

// Unredact restores redacted tokens back to original secrets on inbound responses
func (s *SecretShield) Unredact(text string) string {
	if text == "" {
		return text
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := text
	for token, secret := range s.tokenToSecret {
		if strings.Contains(result, token) {
			result = strings.ReplaceAll(result, token, secret)
		}
	}
	return result
}

// PruneToolOutput truncates oversized historical tool outputs keeping head and tail
func PruneToolOutput(content string, maxChars int) string {
	if maxChars <= 0 {
		maxChars = 2000
	}
	if len(content) <= maxChars {
		return content
	}

	half := maxChars / 2
	head := content[:half]
	tail := content[len(content)-half:]
	omitted := len(content) - maxChars

	return fmt.Sprintf("%s\n\n[... Truncated by Gateway: %d characters omitted for context compaction ...]\n\n%s", head, omitted, tail)
}
