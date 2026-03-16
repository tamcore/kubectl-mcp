package ratelimit

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Wrap returns a new handler that checks the rate limiter before calling the
// inner handler. If the limiter is nil or unlimited, calls pass through.
// When rate limited, returns an error result with an LLM-friendly message.
func Wrap(handler server.ToolHandlerFunc, limiter *Limiter) server.ToolHandlerFunc {
	if limiter == nil {
		return handler
	}
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if !limiter.Allow() {
			return mcp.NewToolResultError(limiter.DenyMessage()), nil
		}
		return handler(ctx, req)
	}
}
