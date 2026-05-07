package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// applySafetyDelay pauses for the given duration before a mutating operation,
// emitting progress notifications each second so clients can show a countdown.
// Returns immediately when delay <= 0. Respects ctx cancellation.
func applySafetyDelay(ctx context.Context, req mcp.CallToolRequest, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}

	total := int(delay.Seconds())
	if total < 1 {
		total = 1
	}

	elapsed := 0
	remaining := delay

	for remaining > 0 {
		tick := time.Second
		if remaining < tick {
			tick = remaining
		}

		msg := fmt.Sprintf("safety delay: %ds remaining", int(remaining.Seconds()))
		sendProgress(ctx, req, elapsed, total, msg)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(tick):
		}

		elapsed++
		remaining -= tick
	}

	return nil
}
