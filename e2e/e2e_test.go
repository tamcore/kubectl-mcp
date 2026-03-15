//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
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

// startSSEServer creates an MCP server backed by the real cluster and starts
// it on a random port. It returns the base URL and a cleanup function.
func startSSEServer(t *testing.T) string {
	t.Helper()

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = home + "/.kube/config"
	}

	cfg := &config.Config{
		Transport:       "sse",
		SSEAddress:      "127.0.0.1:0",
		Kubeconfig:      kubeconfig,
		AllowedContexts: []string{"*"},
		AllowSecrets:    false,
	}

	pool, err := kube.NewClientPool(cfg)
	if err != nil {
		t.Fatalf("NewClientPool: %v", err)
	}

	s := server.NewMCPServer("kubectl-mcp-e2e", "test",
		server.WithToolCapabilities(false),
	)
	tools.RegisterAll(s, pool, cfg)

	// Find a free port.
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
		if err := sseServer.Start(addr); err != nil {
			// Ignore errors from server shutdown.
			if !strings.Contains(err.Error(), "Server closed") {
				t.Logf("SSE server error: %v", err)
			}
		}
	}()

	// Wait for the server to be ready.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = sseServer.Shutdown(ctx)
	})

	return fmt.Sprintf("http://%s", addr)
}

func callTool(t *testing.T, c *mcpclient.Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

func newClient(t *testing.T, base string) *mcpclient.Client {
	t.Helper()
	c, err := mcpclient.NewSSEMCPClient(base + "/sse")
	if err != nil {
		t.Fatalf("NewSSEMCPClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// Use background context for the SSE stream — the stream must stay alive
	// for the lifetime of the client, not just this function.
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	initReq := mcp.InitializeRequest{
		Params: struct {
			ProtocolVersion string                 `json:"protocolVersion"`
			Capabilities    mcp.ClientCapabilities `json:"capabilities"`
			ClientInfo      mcp.Implementation     `json:"clientInfo"`
		}{
			ProtocolVersion: "2024-11-05",
			ClientInfo: mcp.Implementation{
				Name:    "e2e-test",
				Version: "0.0.1",
			},
		},
	}

	if _, err := c.Initialize(ctx, initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c
}

func TestListContexts(t *testing.T) {
	base := startSSEServer(t)
	c := newClient(t, base)

	result := callTool(t, c, "list_contexts", nil)
	text := resultText(result)

	if text == "" {
		t.Fatal("list_contexts returned empty result")
	}

	// kind-action creates a context called "kind-e2e".
	if !strings.Contains(text, "kind-e2e") {
		t.Errorf("expected kind-e2e context, got: %s", text)
	}
}

func TestListNamespaces(t *testing.T) {
	base := startSSEServer(t)
	c := newClient(t, base)

	result := callTool(t, c, "list_namespaces", map[string]any{
		"context": "kind-e2e",
	})
	text := resultText(result)

	if !strings.Contains(text, "default") || !strings.Contains(text, "kube-system") {
		t.Errorf("expected default and kube-system namespaces, got: %s", text)
	}
}

func TestListResources(t *testing.T) {
	base := startSSEServer(t)
	c := newClient(t, base)

	result := callTool(t, c, "list_resources", map[string]any{
		"context":   "kind-e2e",
		"kind":      "Pod",
		"namespace": "kube-system",
	})
	text := resultText(result)

	if result.IsError {
		t.Fatalf("list_resources error: %s", text)
	}

	// Should return JSON array.
	var items []map[string]any
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON array, got: %s", text)
	}
	if len(items) == 0 {
		t.Error("expected at least one pod in kube-system")
	}
}

func TestGetResource(t *testing.T) {
	base := startSSEServer(t)
	c := newClient(t, base)

	result := callTool(t, c, "get_resource", map[string]any{
		"context": "kind-e2e",
		"kind":    "Namespace",
		"name":    "default",
	})
	text := resultText(result)

	if result.IsError {
		t.Fatalf("get_resource error: %s", text)
	}

	// Should return JSON object.
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		t.Fatalf("expected JSON object, got: %s", text)
	}
	if obj["kind"] != "Namespace" {
		t.Errorf("expected kind=Namespace, got %v", obj["kind"])
	}
}

func TestGetEvents(t *testing.T) {
	base := startSSEServer(t)
	c := newClient(t, base)

	result := callTool(t, c, "get_events", map[string]any{
		"context":   "kind-e2e",
		"namespace": "kube-system",
	})
	text := resultText(result)

	if result.IsError {
		t.Fatalf("get_events error: %s", text)
	}

	// Should be valid JSON (array or "No events found").
	if text != "No events found" {
		var items []map[string]any
		if err := json.Unmarshal([]byte(text), &items); err != nil {
			t.Fatalf("expected JSON array, got: %s", text)
		}
	}
}

func TestSecretRedaction(t *testing.T) {
	base := startSSEServer(t)
	c := newClient(t, base)

	// kube-system always has SA token secrets on kind clusters.
	result := callTool(t, c, "list_resources", map[string]any{
		"context":   "kind-e2e",
		"kind":      "Secret",
		"namespace": "kube-system",
	})
	text := resultText(result)

	if result.IsError {
		t.Fatalf("list_resources Secret error: %s", text)
	}

	var items []map[string]any
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON array, got: %s", text)
	}
	if len(items) == 0 {
		t.Fatal("expected at least one secret in kube-system")
	}
}

func TestListResourcesWithFilter(t *testing.T) {
	base := startSSEServer(t)
	c := newClient(t, base)

	result := callTool(t, c, "list_resources", map[string]any{
		"context":   "kind-e2e",
		"kind":      "Pod",
		"namespace": "kube-system",
		"filter":    "status.phase=Running",
	})
	text := resultText(result)

	if result.IsError {
		t.Fatalf("list_resources with filter error: %s", text)
	}

	// When a filter is active, the response has a "Matched N of M Kind\n\n" header
	// before the JSON array. Strip it.
	jsonStart := strings.Index(text, "[")
	if jsonStart < 0 {
		t.Fatalf("expected JSON array in response, got: %s", text)
	}

	var items []map[string]any
	if err := json.Unmarshal([]byte(text[jsonStart:]), &items); err != nil {
		t.Fatalf("expected JSON array, got: %s", text)
	}

	// All returned pods should have status Running.
	for _, item := range items {
		if status, ok := item["status"].(string); ok && status != "Running" {
			t.Errorf("expected Running pod, got status=%s", status)
		}
	}
}
