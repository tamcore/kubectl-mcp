package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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

func TestStreamableHTTPServerStartsAndAcceptsConnections(t *testing.T) {
	s := server.NewMCPServer("test-server", "0.0.1",
		server.WithToolCapabilities(false),
	)

	httpServer := server.NewStreamableHTTPServer(s)

	// Find a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	go func() {
		if err := httpServer.Start(addr); err != nil && !strings.Contains(err.Error(), "Server closed") {
			t.Logf("server error: %v", err)
		}
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	})

	// Wait for the server to accept TCP connections.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Send an MCP initialize request and verify we get a JSON-RPC response.
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.0.1"}}}`
	resp, err := http.Post(
		fmt.Sprintf("http://%s/mcp", addr),
		"application/json",
		strings.NewReader(initBody),
	)
	if err != nil {
		t.Fatalf("POST /mcp failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") && !strings.Contains(ct, "application/json") {
		t.Fatalf("unexpected Content-Type: %s", ct)
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
