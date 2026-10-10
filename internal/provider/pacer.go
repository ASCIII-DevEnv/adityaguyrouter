package provider

import (
	"context"
	"sync"
	"time"
)

// TrafficPacer controls request bursts per session/key to prevent bot-detection flags
type TrafficPacer struct {
	mu       sync.Mutex
	sessions map[string]*sessionBucket
	rate     float64       // leak rate: tokens per second
	capacity float64       // bucket burst capacity
	minGap   time.Duration // minimum physical gap between requests
}

type sessionBucket struct {
	tokens    float64
	lastCheck time.Time
	lastReq   time.Time
}

func NewTrafficPacer(rate float64, capacity float64, minGap time.Duration) *TrafficPacer {
	if rate <= 0 {
		rate = 0.5 // default 1 token per 2 seconds (30 req/min)
	}
	if capacity <= 0 {
		capacity = 5.0 // allow burst of 5 requests
	}
	if minGap <= 0 {
		minGap = 150 * time.Millisecond
	}
	return &TrafficPacer{
		sessions: make(map[string]*sessionBucket),
		rate:     rate,
		capacity: capacity,
		minGap:   minGap,
	}
}

// Pace delays or queues execution if requests are arriving too quickly
func (p *TrafficPacer) Pace(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		sessionID = "default"
	}

	p.mu.Lock()
	now := time.Now()
	b, exists := p.sessions[sessionID]
	if !exists {
		b = &sessionBucket{
			tokens:    p.capacity,
			lastCheck: now,
			lastReq:   time.Time{},
		}
		p.sessions[sessionID] = b
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.tokens += elapsed * p.rate
	if b.tokens > p.capacity {
		b.tokens = p.capacity
	}
	b.lastCheck = now

	// Check minimum physical gap between consecutive requests
	var waitDelay time.Duration
	if !b.lastReq.IsZero() {
		gap := now.Sub(b.lastReq)
		if gap < p.minGap {
			waitDelay = p.minGap - gap
		}
	}

	// If bucket is depleted, calculate pacer queue delay
	if b.tokens < 1.0 {
		needed := 1.0 - b.tokens
		burstDelay := time.Duration(needed/p.rate*float64(time.Second))
		if burstDelay > waitDelay {
			waitDelay = burstDelay
		}
		b.tokens = 0.0
	} else {
		b.tokens -= 1.0
	}

	b.lastReq = now.Add(waitDelay)
	p.mu.Unlock()

	// Wait asynchronously without holding lock
	if waitDelay > 0 {
		select {
		case <-time.After(waitDelay):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// GetModelFallbackChain returns progressive fallback models when a model fails or quota is exhausted
func GetModelFallbackChain(originalModel string) []string {
	chain := []string{originalModel}

	switch originalModel {
	case "claude-sonnet-4-6", "claude-opus-4-6-thinking", "claude-sonnet", "claude-opus", "claude-3-7-sonnet", "claude-3-5-sonnet":
		chain = append(chain, "gemini-3.8-flash-high", "gemini-3.8-flash-tiered")
	case "gemini-3.1-pro-high", "gemini-3.1-pro", "gemini-2.5-pro":
		chain = append(chain, "gemini-3.8-flash-high", "gemini-3.8-flash-tiered")
	case "gemini-3.8-flash-high", "gemini-3.8-flash-medium", "gemini-3.8-flash-low", "gemini-3.8-flash-tiered":
		chain = append(chain, "gemini-3.7-flash-high", "gemini-3.6-flash-high")
	default:
		chain = append(chain, "gemini-3.8-flash-high", "gemini-3.8-flash-tiered")
	}

	// Remove duplicates while preserving order
	seen := make(map[string]bool)
	var unique []string
	for _, m := range chain {
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}
	return unique
}
