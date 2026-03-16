package ratelimit

import (
	"fmt"
	"math"

	"golang.org/x/time/rate"
)

// Limiter is a token-bucket rate limiter for MCP tool calls.
type Limiter struct {
	inner      *rate.Limiter
	perMinute  int
	unlimited  bool
}

// NewLimiter creates a rate limiter that allows perMinute calls per minute.
// If perMinute <= 0, the limiter is unlimited (Allow always returns true).
func NewLimiter(perMinute int) *Limiter {
	if perMinute <= 0 {
		return &Limiter{unlimited: true, perMinute: 0}
	}

	perSecond := float64(perMinute) / 60.0
	burst := int(math.Max(1, float64(perMinute)/10.0))

	return &Limiter{
		inner:     rate.NewLimiter(rate.Limit(perSecond), burst),
		perMinute: perMinute,
	}
}

// Allow reports whether a single event may happen now.
func (l *Limiter) Allow() bool {
	if l.unlimited {
		return true
	}
	return l.inner.Allow()
}

// DenyMessage returns an LLM-friendly error message explaining the rate limit.
func (l *Limiter) DenyMessage() string {
	if l.unlimited {
		return "Rate limited (unlimited — this should not happen)."
	}
	return fmt.Sprintf(
		"Rate limited: too many tool calls. Please slow down and wait a moment before retrying. Current limit: %d calls/minute.",
		l.perMinute,
	)
}
