package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func TestCordonNode(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testNode("node-1"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "cordon_node", func(s *server.MCPServer) {
		registerCordonNode(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node": "node-1",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Cordoned") {
		t.Errorf("expected cordon confirmation, got: %s", text)
	}
	if !strings.Contains(text, "node-1") {
		t.Errorf("expected node name in response, got: %s", text)
	}
	if !strings.Contains(text, "test-ctx") {
		t.Errorf("expected context in response, got: %s", text)
	}
}

func TestUncordonNode(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testNode("node-1"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "uncordon_node", func(s *server.MCPServer) {
		registerUncordonNode(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node": "node-1",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Uncordoned") {
		t.Errorf("expected uncordon confirmation, got: %s", text)
	}
	if !strings.Contains(text, "node-1") {
		t.Errorf("expected node name in response, got: %s", text)
	}
}

func TestCordonNode_NotFound(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "cordon_node", func(s *server.MCPServer) {
		registerCordonNode(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node": "nonexistent",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "failed to cordon") {
		t.Errorf("expected error, got: %s", text)
	}
}

func TestUncordonNode_NotFound(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "uncordon_node", func(s *server.MCPServer) {
		registerUncordonNode(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node": "nonexistent",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "failed to uncordon") {
		t.Errorf("expected error, got: %s", text)
	}
}

func TestCordonNode_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}, AllowWrite: true}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "cordon_node", func(s *server.MCPServer) {
		registerCordonNode(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node": "test",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not allowed error, got: %s", text)
	}
}
