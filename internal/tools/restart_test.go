package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func TestRestartRollout_Deployment(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testDeployment("my-deploy", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "restart_rollout", func(s *server.MCPServer) {
		registerRestartRollout(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "my-deploy",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Restarted Deployment/my-deploy") {
		t.Errorf("expected restart confirmation, got: %s", text)
	}
	if !strings.Contains(text, "restartedAt") {
		t.Errorf("expected restartedAt annotation, got: %s", text)
	}
	if !strings.Contains(text, "test-ctx") {
		t.Errorf("expected context in response, got: %s", text)
	}
}

func TestRestartRollout_DaemonSet(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	ds := testDaemonSet("my-ds", "default")
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(ds)

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "restart_rollout", func(s *server.MCPServer) {
		registerRestartRollout(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "DaemonSet",
		"name":      "my-ds",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Restarted DaemonSet/my-ds") {
		t.Errorf("expected restart confirmation, got: %s", text)
	}
}

func TestRestartRollout_UnsupportedKind(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "restart_rollout", func(s *server.MCPServer) {
		registerRestartRollout(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "ReplicaSet",
		"name":      "test",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "does not support rollout restart") {
		t.Errorf("expected unsupported error, got: %s", text)
	}
}

func TestRestartRollout_NotFound(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "restart_rollout", func(s *server.MCPServer) {
		registerRestartRollout(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "nonexistent",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "failed to restart") {
		t.Errorf("expected not found error, got: %s", text)
	}
}

func TestRestartRollout_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}, AllowWrite: true}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "restart_rollout", func(s *server.MCPServer) {
		registerRestartRollout(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "test",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not allowed error, got: %s", text)
	}
}
