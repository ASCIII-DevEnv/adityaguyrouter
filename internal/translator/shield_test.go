package translator

import (
	"strings"
	"testing"
)

func TestSecretShieldRedactAndUnredact(t *testing.T) {
	shield := NewSecretShield()

	rawPrompt := `Hubungkan ke DB postgresql://user:mypassword123456@db.internal:5432/app dan gunakan API KEY sk-1234567890abcdef1234567890abcdef untuk kirim data.`

	redacted := shield.Redact(rawPrompt)
	if strings.Contains(redacted, "mypassword123456") {
		t.Fatalf("expected db password to be redacted, got: %s", redacted)
	}
	if strings.Contains(redacted, "sk-1234567890abcdef1234567890abcdef") {
		t.Fatalf("expected api key to be redacted, got: %s", redacted)
	}
	if !strings.Contains(redacted, "[REDACTED_SECRET_") {
		t.Fatalf("expected token placeholder [REDACTED_SECRET_...]")
	}

	// Restore unredact
	unredacted := shield.Unredact(redacted)
	if unredacted != rawPrompt {
		t.Fatalf("expected unredact to match original prompt, got: %s", unredacted)
	}
}

func TestPruneToolOutput(t *testing.T) {
	hugeOutput := strings.Repeat("A", 3000) + strings.Repeat("B", 3000)
	pruned := PruneToolOutput(hugeOutput, 1000)

	if len(pruned) >= len(hugeOutput) {
		t.Fatalf("expected pruned output to be shorter than original")
	}
	if !strings.Contains(pruned, "[... Truncated by Gateway:") {
		t.Fatalf("expected truncation banner in output")
	}
	if !strings.HasPrefix(pruned, "AAAAA") || !strings.HasSuffix(pruned, "BBBBB") {
		t.Fatalf("expected head and tail to be preserved")
	}
}
