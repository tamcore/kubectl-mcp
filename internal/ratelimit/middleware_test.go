package ratelimit

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var dummyHandler server.ToolHandlerFunc = func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("ok"), nil
}

func TestWrap_PassesThroughUnderLimit(t *testing.T) {
	l := NewLimiter(120)
	wrapped := Wrap(dummyHandler, l)

	res, err := wrapped(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Error("expected non-error result")
	}
}

func TestWrap_ReturnsErrorWhenRateLimited(t *testing.T) {
	l := NewLimiter(1) // 1/min, burst=1
	wrapped := Wrap(dummyHandler, l)

	// First call uses the burst token.
	_, _ = wrapped(context.Background(), mcp.CallToolRequest{})

	// Subsequent calls should be rate limited.
	var rateLimited bool
	for i := 0; i < 10; i++ {
		res, err := wrapped(context.Background(), mcp.CallToolRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			text := res.Content[0].(mcp.TextContent).Text
			if strings.Contains(text, "Rate limited") {
				rateLimited = true
				break
			}
		}
	}
	if !rateLimited {
		t.Error("expected rate limiting error after exceeding limit")
	}
}

func TestWrap_NilLimiterPassesThrough(t *testing.T) {
	wrapped := Wrap(dummyHandler, nil)

	for i := 0; i < 100; i++ {
		res, err := wrapped(context.Background(), mcp.CallToolRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("nil limiter should never rate limit, iteration %d", i)
		}
	}
}

func TestWrap_UnlimitedLimiterPassesThrough(t *testing.T) {
	l := NewLimiter(0) // unlimited
	wrapped := Wrap(dummyHandler, l)

	for i := 0; i < 100; i++ {
		res, err := wrapped(context.Background(), mcp.CallToolRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unlimited limiter should never rate limit, iteration %d", i)
		}
	}
}
