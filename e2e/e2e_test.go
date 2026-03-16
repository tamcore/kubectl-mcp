//go:build e2e

package e2e

import (
	"context"
	"fmt"
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

	s := server.NewMCPServer("kubectl-mcp-e2e", "test",
		server.WithToolCapabilities(false),
	)
	tools.RegisterAll(s, pool, cfg)

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

	s := server.NewMCPServer("kubectl-mcp-e2e", "test",
		server.WithToolCapabilities(false),
	)
	tools.RegisterAll(s, pool, cfg)

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
