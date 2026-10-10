package provider

import (
	"context"
	"testing"
	"time"
)

func TestTrafficPacerMinGapAndLeakyBucket(t *testing.T) {
	// Rate: 1 token/sec, Capacity: 2 tokens, minGap: 50ms
	pacer := NewTrafficPacer(1.0, 2.0, 50*time.Millisecond)

	ctx := context.Background()
	start := time.Now()

	// 1st request should be immediate
	if err := pacer.Pace(ctx, "sess-1"); err != nil {
		t.Fatalf("first request failed: %v", err)
	}

	// 2nd request should enforce minimum gap (~50ms)
	if err := pacer.Pace(ctx, "sess-1"); err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Fatalf("expected at least 40ms physical gap, got %v", elapsed)
	}
}

func TestModelFallbackChain(t *testing.T) {
	claudeChain := GetModelFallbackChain("claude-sonnet-4-6")
	if len(claudeChain) < 2 {
		t.Fatalf("expected fallback chain for claude, got %v", claudeChain)
	}
	if claudeChain[0] != "claude-sonnet-4-6" {
		t.Fatalf("expected first model to be claude-sonnet-4-6, got %s", claudeChain[0])
	}
	if claudeChain[1] != "claude-opus-4-6-thinking" {
		t.Fatalf("expected second model to be claude-opus-4-6-thinking, got %s", claudeChain[1])
	}

	flashChain := GetModelFallbackChain("gemini-3.8-flash-tiered")
	if len(flashChain) < 2 {
		t.Fatalf("expected fallback chain for flash, got %v", flashChain)
	}
}
