//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
	"github.com/tamcore/kubectl-mcp/internal/mcplog"
	"github.com/tamcore/kubectl-mcp/internal/resources"
	"github.com/tamcore/kubectl-mcp/internal/tools"
)

// ---------------------------------------------------------------------------
// Server helpers
// ---------------------------------------------------------------------------

func defaultConfig() *config.Config {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = home + "/.kube/config"
	}
	return &config.Config{
		Kubeconfig:      kubeconfig,
		AllowedContexts: []string{"*"},
		AllowWrite:      true,
		AllowDestructive: true,
		AllowSecrets:    false,
		RateLimitRead:   0, // unlimited for tests
		RateLimitWrite:  0,
	}
}

func startSSEServerWithConfig(t *testing.T, cfg *config.Config) string {
	t.Helper()

	pool, err := kube.NewClientPool(cfg)
	if err != nil {
		t.Fatalf("NewClientPool: %v", err)
	}

	var s *server.MCPServer
	opts := []server.ServerOption{
		server.WithToolCapabilities(false),
		server.WithResourceCapabilities(false, false),
	}
	hooks := buildE2ELoggingHooks(t, &s, cfg, pool)
	if hooks != nil {
		opts = append(opts, server.WithLogging(), server.WithHooks(hooks))
	}

	s = server.NewMCPServer("kubectl-mcp-e2e", "test", opts...)
	tools.RegisterAll(s, pool, cfg)
	resources.RegisterAll(s, pool, cfg)


	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	sseServer := server.NewSSEServer(s,
		server.WithBaseURL(fmt.Sprintf("http://%s", addr)),
	)

	go func() {
		if err := sseServer.Start(addr); err != nil && !strings.Contains(err.Error(), "Server closed") {
			t.Logf("SSE server error: %v", err)
		}
	}()

	waitForServer(t, addr)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// mcp-go v0.45.0 SSEServer.Shutdown panics with "close of closed
		// channel" when a client has already disconnected. Recover so the
		// test result is not masked by the library bug.
		defer func() {
			if r := recover(); r != nil {
				t.Logf("recovered panic during SSE shutdown: %v", r)
			}
		}()
		_ = sseServer.Shutdown(ctx)
	})

	return fmt.Sprintf("http://%s", addr)
}

func startStreamableHTTPServerWithConfig(t *testing.T, cfg *config.Config) string {
	t.Helper()

	pool, err := kube.NewClientPool(cfg)
	if err != nil {
		t.Fatalf("NewClientPool: %v", err)
	}

	var s *server.MCPServer
	opts := []server.ServerOption{
		server.WithToolCapabilities(false),
		server.WithResourceCapabilities(false, false),
	}
	hooks := buildE2ELoggingHooks(t, &s, cfg, pool)
	if hooks != nil {
		opts = append(opts, server.WithLogging(), server.WithHooks(hooks))
	}

	s = server.NewMCPServer("kubectl-mcp-e2e", "test", opts...)
	tools.RegisterAll(s, pool, cfg)
	resources.RegisterAll(s, pool, cfg)


	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	httpServer := server.NewStreamableHTTPServer(s)

	go func() {
		if err := httpServer.Start(addr); err != nil && !strings.Contains(err.Error(), "Server closed") {
			t.Logf("streamable-HTTP server error: %v", err)
		}
	}()

	waitForServer(t, addr)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	})

	return fmt.Sprintf("http://%s", addr)
}

func waitForServer(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server at %s did not start in time", addr)
}

// ---------------------------------------------------------------------------
// Client helpers
// ---------------------------------------------------------------------------

func newSSEClient(t *testing.T, base string) *mcpclient.Client {
	t.Helper()
	c, err := mcpclient.NewSSEMCPClient(base + "/sse")
	if err != nil {
		t.Fatalf("NewSSEMCPClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	initClient(t, c)
	return c
}

func newHTTPClient(t *testing.T, base string) *mcpclient.Client {
	t.Helper()
	c, err := mcpclient.NewStreamableHttpClient(base + "/mcp")
	if err != nil {
		t.Fatalf("NewStreamableHttpClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	initClient(t, c)
	return c
}

func initClient(t *testing.T, c *mcpclient.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	initReq := mcp.InitializeRequest{
		Params: struct {
			ProtocolVersion string                 `json:"protocolVersion"`
			Capabilities    mcp.ClientCapabilities `json:"capabilities"`
			ClientInfo      mcp.Implementation     `json:"clientInfo"`
		}{
			ProtocolVersion: "2024-11-05",
			ClientInfo:      mcp.Implementation{Name: "e2e-test", Version: "0.0.1"},
		},
	}

	if _, err := c.Initialize(ctx, initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tool call helpers
// ---------------------------------------------------------------------------

// callToolMayFail calls a tool and returns the result and error without failing the test.
func callToolMayFail(t *testing.T, c *mcpclient.Client, name string, args map[string]any) (*mcp.CallToolResult, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	return c.CallTool(ctx, req)
}

func callTool(t *testing.T, c *mcpclient.Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := c.CallTool(ctx, req)
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return result
}

func resultText(r *mcp.CallToolResult) string {
	if len(r.Content) == 0 {
		return ""
	}
	if tc, ok := r.Content[0].(mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

// ---------------------------------------------------------------------------
// Transport parameterization
// ---------------------------------------------------------------------------

type transportCase struct {
	name       string
	startFunc  func(t *testing.T, cfg *config.Config) string
	clientFunc func(t *testing.T, base string) *mcpclient.Client
}

var allTransports = []transportCase{
	{"SSE", startSSEServerWithConfig, newSSEClient},
	{"HTTP", startStreamableHTTPServerWithConfig, newHTTPClient},
}

// ---------------------------------------------------------------------------
// Logging hooks for E2E (mirrors internal/cmd/serve.go logic)
// ---------------------------------------------------------------------------

// buildE2ELoggingHooks creates logging hooks for E2E tests matching the
// given config's LogLevel. When LogDir is set, uses per-context log files
// via ContextLogWriter. Returns nil for empty/unset levels.
func buildE2ELoggingHooks(t *testing.T, sPtr **server.MCPServer, cfg *config.Config, pool *kube.ClientPool) *server.Hooks {
	t.Helper()

	if cfg.LogLevel == "" {
		return nil
	}

	level, err := mcplog.ParseLogLevel(cfg.LogLevel)
	if err != nil {
		t.Fatalf("invalid log level %q: %v", cfg.LogLevel, err)
	}

	hooks := &server.Hooks{}
	if level == mcplog.LogLevelOff {
		return hooks
	}

	// When LogDir is set, use per-context log writer.
	if cfg.LogDir != "" {
		clw, clwErr := mcplog.NewContextLogWriter(cfg.LogDir)
		if clwErr != nil {
			t.Fatalf("NewContextLogWriter: %v", clwErr)
		}
		t.Cleanup(func() { _ = clw.Close() })
		clw.MainLogger().Printf("e2e server started (log-level=%s)", level)

		return buildContextLoggingHooks(sPtr, level, clw, pool)
	}

	// Fallback: simple stderr logging for tests without LogDir.
	logger := log.New(os.Stderr, "e2e: ", 0)

	hooks.AddBeforeCallTool(func(ctx context.Context, id any, req *mcp.CallToolRequest) {
		args := summarizeToolArgs(req.GetArguments())
		logger.Printf("→ %s(%s)", req.Params.Name, args)

		if level == mcplog.LogLevelDebug && *sPtr != nil {
			_ = (*sPtr).SendLogMessageToClient(ctx, mcplog.NewNotification(
				mcp.LoggingLevelDebug,
				fmt.Sprintf("→ %s(%s)", req.Params.Name, args),
			))
		}
	})

	hooks.AddAfterCallTool(func(ctx context.Context, id any, req *mcp.CallToolRequest, result any) {
		r, ok := result.(*mcp.CallToolResult)
		if !ok {
			return
		}

		if r.IsError {
			logger.Printf("✗ %s failed", req.Params.Name)
			if *sPtr != nil {
				_ = (*sPtr).SendLogMessageToClient(ctx, mcplog.NewNotification(
					mcp.LoggingLevelWarning,
					fmt.Sprintf("%s failed", req.Params.Name),
				))
			}
			return
		}

		if level == mcplog.LogLevelDebug && *sPtr != nil {
			_ = (*sPtr).SendLogMessageToClient(ctx, mcplog.NewNotification(
				mcp.LoggingLevelDebug,
				fmt.Sprintf("✓ %s done", req.Params.Name),
			))
		}
		logger.Printf("✓ %s done", req.Params.Name)
	})

	return hooks
}

// buildContextLoggingHooks creates hooks that route logs to per-context files.
func buildContextLoggingHooks(sPtr **server.MCPServer, level mcplog.LogLevel, clw *mcplog.ContextLogWriter, pool *kube.ClientPool) *server.Hooks {
	hooks := &server.Hooks{}

	hooks.AddBeforeCallTool(func(ctx context.Context, id any, req *mcp.CallToolRequest) {
		logger := resolveE2ELogger(clw, pool, req)
		args := summarizeToolArgs(req.GetArguments())
		logger.Printf("→ %s(%s)", req.Params.Name, args)

		if level == mcplog.LogLevelDebug && *sPtr != nil {
			_ = (*sPtr).SendLogMessageToClient(ctx, mcplog.NewNotification(
				mcp.LoggingLevelDebug,
				fmt.Sprintf("→ %s(%s)", req.Params.Name, args),
			))
		}
	})

	hooks.AddAfterCallTool(func(ctx context.Context, id any, req *mcp.CallToolRequest, result any) {
		r, ok := result.(*mcp.CallToolResult)
		if !ok {
			return
		}

		logger := resolveE2ELogger(clw, pool, req)

		if r.IsError {
			logger.Printf("✗ %s failed", req.Params.Name)
			if *sPtr != nil {
				_ = (*sPtr).SendLogMessageToClient(ctx, mcplog.NewNotification(
					mcp.LoggingLevelWarning,
					fmt.Sprintf("%s failed", req.Params.Name),
				))
			}
			return
		}

		if level == mcplog.LogLevelDebug && *sPtr != nil {
			_ = (*sPtr).SendLogMessageToClient(ctx, mcplog.NewNotification(
				mcp.LoggingLevelDebug,
				fmt.Sprintf("✓ %s done", req.Params.Name),
			))
		}
		logger.Printf("✓ %s done", req.Params.Name)
	})

	hooks.AddOnError(func(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
		clw.MainLogger().Printf("✗ error in %s: %v", method, err)
	})

	return hooks
}

// resolveE2ELogger extracts the "context" parameter from the request and
// returns the per-context logger.
func resolveE2ELogger(clw *mcplog.ContextLogWriter, pool *kube.ClientPool, req *mcp.CallToolRequest) *log.Logger {
	ctxName := ""
	if args := req.GetArguments(); args != nil {
		if v, ok := args["context"]; ok {
			if s, ok := v.(string); ok {
				ctxName = s
			}
		}
	}
	if ctxName == "" {
		ctxName = pool.DefaultContext()
	}
	return clw.LoggerFor(ctxName)
}

func summarizeToolArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "..."
	}
	s := string(b)
	if len(s) > 2 {
		s = s[1 : len(s)-1]
	}
	const maxLen = 120
	if len(s) > maxLen {
		s = s[:maxLen] + "…"
	}
	return s
}
