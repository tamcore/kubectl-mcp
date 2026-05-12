package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
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
	instructions := serverInstructions(&config.Config{})
	if instructions == "" {
		t.Fatal("serverInstructions() returned empty string")
	}
}

func TestServerInstructions_ContainsKeyGuidance(t *testing.T) {
	instructions := serverInstructions(&config.Config{})

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
		server.WithInstructions(serverInstructions(&config.Config{})),
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
	dir := t.TempDir()
	clw, err := mcplog.NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	pool := kube.NewClientPoolForTest(
		&config.Config{AllowedContexts: []string{"*"}},
		clientcmdapi.Config{
			CurrentContext: "test-ctx",
			Contexts:       map[string]*clientcmdapi.Context{"test-ctx": {}},
		},
		nil,
	)

	var s *server.MCPServer
	hooks := newLoggingHooks(&s, mcplog.LogLevelInfo, clw, pool)
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
	dir := t.TempDir()
	clw, err := mcplog.NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	pool := kube.NewClientPoolForTest(
		&config.Config{AllowedContexts: []string{"*"}},
		clientcmdapi.Config{
			CurrentContext: "test-ctx",
			Contexts:       map[string]*clientcmdapi.Context{"test-ctx": {}},
		},
		nil,
	)

	var s *server.MCPServer
	hooks := newLoggingHooks(&s, mcplog.LogLevelOff, clw, pool)

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

	// At off level, context log files should not be created.
	contextLogFile := filepath.Join(clw.Dir(), "test-ctx.log")
	if _, statErr := os.Stat(contextLogFile); statErr == nil {
		content, _ := os.ReadFile(contextLogFile)
		if len(content) > 0 {
			t.Fatalf("expected no log output at off level, got: %s", string(content))
		}
	}
}

func TestLoggingHooks_Info(t *testing.T) {
	dir := t.TempDir()
	clw, err := mcplog.NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	pool := kube.NewClientPoolForTest(
		&config.Config{AllowedContexts: []string{"*"}},
		clientcmdapi.Config{
			CurrentContext: "test-ctx",
			Contexts:       map[string]*clientcmdapi.Context{"test-ctx": {}},
		},
		nil,
	)

	var s *server.MCPServer
	hooks := newLoggingHooks(&s, mcplog.LogLevelInfo, clw, pool)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	req.Params.Name = "get_resource"
	req.Params.Arguments = map[string]any{"kind": "Pod", "name": "nginx"}

	// Before hook should log tool name + summarized args.
	for _, fn := range hooks.OnBeforeCallTool {
		fn(ctx, "id-1", req)
	}
	contextLogFile := filepath.Join(clw.Dir(), "test-ctx.log")
	content, readErr := os.ReadFile(contextLogFile)
	if readErr != nil {
		t.Fatalf("failed to read context log: %v", readErr)
	}
	if !strings.Contains(string(content), "get_resource") {
		t.Fatalf("expected tool name in log, got: %s", string(content))
	}

	// After hook should log success.
	result := &mcp.CallToolResult{IsError: false}
	for _, fn := range hooks.OnAfterCallTool {
		fn(ctx, "id-1", req, result)
	}
	content, _ = os.ReadFile(contextLogFile)
	if !strings.Contains(string(content), "✓") || !strings.Contains(string(content), "get_resource") {
		t.Fatalf("expected success marker in log, got: %s", string(content))
	}

	// After hook should log failure.
	errorResult := &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{mcp.NewTextContent("not found")},
	}
	for _, fn := range hooks.OnAfterCallTool {
		fn(ctx, "id-2", req, errorResult)
	}
	content, _ = os.ReadFile(contextLogFile)
	if !strings.Contains(string(content), "✗") {
		t.Fatalf("expected failure marker in log, got: %s", string(content))
	}
}

func TestLoggingHooks_Debug(t *testing.T) {
	dir := t.TempDir()
	clw, err := mcplog.NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	pool := kube.NewClientPoolForTest(
		&config.Config{AllowedContexts: []string{"*"}},
		clientcmdapi.Config{
			CurrentContext: "test-ctx",
			Contexts:       map[string]*clientcmdapi.Context{"test-ctx": {}},
		},
		nil,
	)

	var s *server.MCPServer
	hooks := newLoggingHooks(&s, mcplog.LogLevelDebug, clw, pool)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	req.Params.Name = "get_resource"
	req.Params.Arguments = map[string]any{"kind": "Pod", "name": "nginx", "namespace": "default"}

	// Before hook should log full (unsummarized) args.
	for _, fn := range hooks.OnBeforeCallTool {
		fn(ctx, "id-1", req)
	}
	contextLogFile := filepath.Join(clw.Dir(), "test-ctx.log")
	content, readErr := os.ReadFile(contextLogFile)
	if readErr != nil {
		t.Fatalf("failed to read context log: %v", readErr)
	}
	if !strings.Contains(string(content), "get_resource") {
		t.Fatalf("expected tool name in debug log, got: %s", string(content))
	}

	// After hook should log full result text.
	result := &mcp.CallToolResult{
		IsError: false,
		Content: []mcp.Content{mcp.NewTextContent(`{"apiVersion":"v1","kind":"Pod"}`)},
	}
	for _, fn := range hooks.OnAfterCallTool {
		fn(ctx, "id-1", req, result)
	}
	content, _ = os.ReadFile(contextLogFile)
	if !strings.Contains(string(content), "get_resource") {
		t.Fatalf("expected tool name in debug after log, got: %s", string(content))
	}
	if !strings.Contains(string(content), "apiVersion") {
		t.Fatalf("expected result content in debug after log, got: %s", string(content))
	}
}

func TestLoggingHooks_RoutesToExplicitContext(t *testing.T) {
	dir := t.TempDir()
	clw, err := mcplog.NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	pool := kube.NewClientPoolForTest(
		&config.Config{AllowedContexts: []string{"*"}},
		clientcmdapi.Config{
			CurrentContext: "default-ctx",
			Contexts: map[string]*clientcmdapi.Context{
				"default-ctx": {},
				"prod-ctx":    {},
			},
		},
		nil,
	)

	var s *server.MCPServer
	hooks := newLoggingHooks(&s, mcplog.LogLevelInfo, clw, pool)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	req.Params.Name = "list_resources"
	req.Params.Arguments = map[string]any{"kind": "Pod", "context": "prod-ctx"}

	for _, fn := range hooks.OnBeforeCallTool {
		fn(ctx, "id-1", req)
	}

	// Should write to prod-ctx.log, not default-ctx.log.
	prodLog := filepath.Join(clw.Dir(), "prod-ctx.log")
	content, readErr := os.ReadFile(prodLog)
	if readErr != nil {
		t.Fatalf("expected prod-ctx.log to exist: %v", readErr)
	}
	if !strings.Contains(string(content), "list_resources") {
		t.Fatalf("expected tool name in prod-ctx.log, got: %s", string(content))
	}

	defaultLog := filepath.Join(clw.Dir(), "default-ctx.log")
	if _, statErr := os.Stat(defaultLog); statErr == nil {
		t.Fatal("default-ctx.log should not exist when explicit context is used")
	}
}

func TestLoggingHooks_OnErrorUsesMainLogger(t *testing.T) {
	dir := t.TempDir()
	clw, err := mcplog.NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	pool := kube.NewClientPoolForTest(
		&config.Config{AllowedContexts: []string{"*"}},
		clientcmdapi.Config{
			CurrentContext: "test-ctx",
			Contexts:       map[string]*clientcmdapi.Context{"test-ctx": {}},
		},
		nil,
	)

	var s *server.MCPServer
	hooks := newLoggingHooks(&s, mcplog.LogLevelInfo, clw, pool)

	ctx := context.Background()
	for _, fn := range hooks.OnError {
		fn(ctx, "id-1", mcp.MethodToolsCall, nil, errors.New("boom"))
	}

	serverLog := filepath.Join(clw.Dir(), "server.log")
	content, readErr := os.ReadFile(serverLog)
	if readErr != nil {
		t.Fatalf("failed to read server.log: %v", readErr)
	}
	if !strings.Contains(string(content), "boom") {
		t.Fatalf("expected error message in server.log, got: %s", string(content))
	}
}
