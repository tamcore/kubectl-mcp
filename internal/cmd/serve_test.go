package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/mcplog"
)

// startTestMCPServer starts a streamable-HTTP MCP server on a random port and
// returns the address. The server is shut down when the test completes.
func startTestMCPServer(t *testing.T, s *server.MCPServer) string {
	t.Helper()

	httpServer := server.NewStreamableHTTPServer(s)

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

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			return addr
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("server did not start within 3 seconds")
	return ""
}

// sendInitialize sends an MCP initialize request and returns the raw response body.
func sendInitialize(t *testing.T, addr string) []byte {
	t.Helper()

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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return body
}

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

func TestSummarizeArgs_NoTruncation(t *testing.T) {
	long := strings.Repeat("x", 500)
	args := map[string]any{"key": long}
	got := summarizeArgs(args)
	if strings.HasSuffix(got, "…") {
		t.Fatalf("expected full string, got truncated output: %q", got)
	}
	if !strings.Contains(got, long) {
		t.Fatalf("expected full value in output, got: %q", got)
	}
}

func TestStreamableHTTPServerStartsAndAcceptsConnections(t *testing.T) {
	s := server.NewMCPServer("test-server", "0.0.1",
		server.WithToolCapabilities(false),
	)

	addr := startTestMCPServer(t, s)
	body := sendInitialize(t, addr)

	if len(body) == 0 {
		t.Fatal("empty response body")
	}
}

func TestServerInstructions_NotEmpty(t *testing.T) {
	instructions := serverInstructions()
	if instructions == "" {
		t.Fatal("serverInstructions() returned empty string")
	}
}

func TestServerInstructions_ContainsKeyGuidance(t *testing.T) {
	instructions := serverInstructions()

	requiredPhrases := []string{
		"read-only",
		"list_contexts",
		"explain_resource",
		"secrets",
		"destructive",
		"dry",
	}

	for _, phrase := range requiredPhrases {
		if !strings.Contains(strings.ToLower(instructions), phrase) {
			t.Errorf("instructions missing key phrase %q", phrase)
		}
	}
}

func TestServerInstructions_InInitializeResponse(t *testing.T) {
	s := server.NewMCPServer("test-server", "0.0.1",
		server.WithToolCapabilities(false),
		server.WithInstructions(serverInstructions()),
	)

	addr := startTestMCPServer(t, s)
	body := sendInitialize(t, addr)

	// The streamable-HTTP response is SSE-formatted. Extract the JSON-RPC
	// result by scanning for the data line.
	var resultJSON []byte
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "data: ") {
			resultJSON = []byte(strings.TrimPrefix(line, "data: "))
			break
		}
	}
	if resultJSON == nil {
		// Fallback: response may be plain JSON (depends on mcp-go version).
		resultJSON = body
	}

	var rpcResp struct {
		Result struct {
			Instructions string `json:"instructions"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resultJSON, &rpcResp); err != nil {
		t.Fatalf("unmarshal initialize response: %v\nbody: %s", err, string(body))
	}

	if rpcResp.Result.Instructions == "" {
		t.Fatalf("instructions field is empty in initialize response:\n%s", string(body))
	}
	if !strings.Contains(rpcResp.Result.Instructions, "read-only") {
		t.Fatalf("instructions missing expected content, got: %s", rpcResp.Result.Instructions)
	}
}

func TestNewLoggingHooks(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	var s *server.MCPServer
	hooks := newLoggingHooks(&s, mcplog.LogLevelInfo, logger)
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

func TestLoggingHooks_Off(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	var s *server.MCPServer
	hooks := newLoggingHooks(&s, mcplog.LogLevelOff, logger)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	req.Params.Name = "get_resource"
	req.Params.Arguments = map[string]any{"kind": "Pod", "name": "nginx"}

	for _, fn := range hooks.OnBeforeCallTool {
		fn(ctx, "id-1", req)
	}
	result := &mcp.CallToolResult{IsError: false}
	for _, fn := range hooks.OnAfterCallTool {
		fn(ctx, "id-1", req, result)
	}
	for _, fn := range hooks.OnError {
		fn(ctx, "id-2", mcp.MethodToolsCall, nil, errors.New("boom"))
	}

	if buf.Len() > 0 {
		t.Fatalf("expected no log output at off level, got: %s", buf.String())
	}
}

func TestLoggingHooks_Info(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	var s *server.MCPServer
	hooks := newLoggingHooks(&s, mcplog.LogLevelInfo, logger)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	req.Params.Name = "get_resource"
	req.Params.Arguments = map[string]any{"kind": "Pod", "name": "nginx"}

	// Before hook should log tool name + summarized args.
	for _, fn := range hooks.OnBeforeCallTool {
		fn(ctx, "id-1", req)
	}
	output := buf.String()
	if !strings.Contains(output, "get_resource") {
		t.Fatalf("expected tool name in log, got: %s", output)
	}

	// After hook should log success.
	buf.Reset()
	result := &mcp.CallToolResult{IsError: false}
	for _, fn := range hooks.OnAfterCallTool {
		fn(ctx, "id-1", req, result)
	}
	output = buf.String()
	if !strings.Contains(output, "✓") || !strings.Contains(output, "get_resource") {
		t.Fatalf("expected success marker in log, got: %s", output)
	}

	// After hook should log failure.
	buf.Reset()
	errorResult := &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{mcp.NewTextContent("not found")},
	}
	for _, fn := range hooks.OnAfterCallTool {
		fn(ctx, "id-2", req, errorResult)
	}
	output = buf.String()
	if !strings.Contains(output, "✗") {
		t.Fatalf("expected failure marker in log, got: %s", output)
	}
}

func TestLoggingHooks_Debug(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	var s *server.MCPServer
	hooks := newLoggingHooks(&s, mcplog.LogLevelDebug, logger)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	req.Params.Name = "get_resource"
	req.Params.Arguments = map[string]any{"kind": "Pod", "name": "nginx", "namespace": "default"}

	// Before hook should log full (unsummarized) args.
	for _, fn := range hooks.OnBeforeCallTool {
		fn(ctx, "id-1", req)
	}
	output := buf.String()
	if !strings.Contains(output, "get_resource") {
		t.Fatalf("expected tool name in debug log, got: %s", output)
	}

	// After hook should log full result text.
	buf.Reset()
	result := &mcp.CallToolResult{
		IsError: false,
		Content: []mcp.Content{mcp.NewTextContent(`{"apiVersion":"v1","kind":"Pod"}`)},
	}
	for _, fn := range hooks.OnAfterCallTool {
		fn(ctx, "id-1", req, result)
	}
	output = buf.String()
	if !strings.Contains(output, "get_resource") {
		t.Fatalf("expected tool name in debug after log, got: %s", output)
	}
	if !strings.Contains(output, "apiVersion") {
		t.Fatalf("expected result content in debug after log, got: %s", output)
	}
}

func TestOpenLogFile_CreatesDirectoryAndFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "subdir", "test.log")

	f, err := openLogFile(logPath)
	if err != nil {
		t.Fatalf("openLogFile(%q) error: %v", logPath, err)
	}
	defer func() { _ = f.Close() }()

	// Write something to verify it's writable.
	if _, err := f.WriteString("test\n"); err != nil {
		t.Fatalf("failed to write to log file: %v", err)
	}

	// Verify the file exists.
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("log file does not exist: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("log file is empty after write")
	}
}

func TestOpenLogFile_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	// Create a file with existing content.
	if err := os.WriteFile(logPath, []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := openLogFile(logPath)
	if err != nil {
		t.Fatalf("openLogFile(%q) error: %v", logPath, err)
	}
	if _, err := f.WriteString("new\n"); err != nil {
		t.Fatalf("write error: %v", err)
	}
	_ = f.Close()

	content, _ := os.ReadFile(logPath)
	if !strings.Contains(string(content), "existing") {
		t.Fatal("existing content was overwritten")
	}
	if !strings.Contains(string(content), "new") {
		t.Fatal("new content not appended")
	}
}
