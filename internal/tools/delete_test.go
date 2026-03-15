package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func TestDeleteResource_Namespaced(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("to-delete", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "delete_resource", func(s *server.MCPServer) {
		registerDeleteResource(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Pod",
		"name":      "to-delete",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Deleted Pod/to-delete") {
		t.Errorf("expected delete confirmation, got: %s", text)
	}
	if !strings.Contains(text, `namespace "default"`) {
		t.Errorf("expected namespace in response, got: %s", text)
	}
	if !strings.Contains(text, "test-ctx") {
		t.Errorf("expected context in response, got: %s", text)
	}
}

func TestDeleteResource_ClusterScoped(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testNode("node-1"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "delete_resource", func(s *server.MCPServer) {
		registerDeleteResource(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind": "Node",
		"name": "node-1",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Deleted Node/node-1") {
		t.Errorf("expected delete confirmation, got: %s", text)
	}
	if strings.Contains(text, "namespace") {
		t.Errorf("cluster-scoped should not mention namespace, got: %s", text)
	}
}

func TestDeleteResource_NotFound(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "delete_resource", func(s *server.MCPServer) {
		registerDeleteResource(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Pod",
		"name":      "nonexistent",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "failed to delete") {
		t.Errorf("expected error for nonexistent resource, got: %s", text)
	}
}

func TestDeleteResource_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other-ctx"}, AllowWrite: true, AllowDestructive: true}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "delete_resource", func(s *server.MCPServer) {
		registerDeleteResource(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind": "Pod",
		"name": "test",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not allowed error, got: %s", text)
	}
}
