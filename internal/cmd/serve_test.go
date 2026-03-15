package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestSummarizeArgs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "nil map",
			args: nil,
			want: "",
		},
		{
			name: "empty map",
			args: map[string]any{},
			want: "",
		},
		{
			name: "single key-value",
			args: map[string]any{"namespace": "default"},
			want: `"namespace":"default"`,
		},
		{
			name: "unmarshalable value",
			args: map[string]any{"bad": make(chan int)},
			want: "...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarizeArgs(tt.args)
			if got != tt.want {
				t.Fatalf("summarizeArgs() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummarizeArgs_MultipleKeys(t *testing.T) {
	args := map[string]any{"a": 1, "b": "two"}
	got := summarizeArgs(args)
	if !strings.Contains(got, `"a":1`) {
		t.Fatalf("expected key a, got %q", got)
	}
	if !strings.Contains(got, `"b":"two"`) {
		t.Fatalf("expected key b, got %q", got)
	}
	if strings.HasPrefix(got, "{") || strings.HasSuffix(got, "}") {
		t.Fatalf("expected no outer braces, got %q", got)
	}
}

func TestSummarizeArgs_Truncation(t *testing.T) {
	args := map[string]any{"key": strings.Repeat("x", 200)}
	got := summarizeArgs(args)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected truncated string ending with …, got %q", got)
	}
	// 120 bytes kept + 3 bytes for UTF-8 "…" = 123 max.
	if len(got) > 123 {
		t.Fatalf("truncated string too long: %d bytes", len(got))
	}
}

func TestNewLoggingHooks(t *testing.T) {
	hooks := newLoggingHooks()
	if hooks == nil {
		t.Fatal("newLoggingHooks() returned nil")
	}

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	req.Params.Name = "test-tool"

	// Exercise BeforeCallTool hook.
	for _, fn := range hooks.OnBeforeCallTool {
		fn(ctx, "id-1", req)
	}

	// Exercise AfterCallTool hook — success path.
	successResult := &mcp.CallToolResult{IsError: false}
	for _, fn := range hooks.OnAfterCallTool {
		fn(ctx, "id-1", req, successResult)
	}

	// Exercise AfterCallTool hook — error path.
	errorResult := &mcp.CallToolResult{IsError: true}
	for _, fn := range hooks.OnAfterCallTool {
		fn(ctx, "id-2", req, errorResult)
	}

	// Exercise OnError hook.
	for _, fn := range hooks.OnError {
		fn(ctx, "id-3", mcp.MethodToolsCall, nil, errors.New("boom"))
	}
}
