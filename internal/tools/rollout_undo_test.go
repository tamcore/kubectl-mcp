package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func TestRolloutUndo_DefaultRevision(t *testing.T) {
	dep := testDeployment("my-deploy", "default")
	rs1 := testReplicaSet("my-deploy-rs1", "default", "1", "my-deploy")
	rs2 := testReplicaSet("my-deploy-rs2", "default", "2", "my-deploy")

	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(dep, rs1, rs2)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_undo", func(s *server.MCPServer) {
		registerRolloutUndo(s, pool, cfg)
	})

	// Without toRevision, should roll back to the previous revision (1).
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "my-deploy",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Rolled back") {
		t.Errorf("expected rollback confirmation, got: %s", text)
	}
	if !strings.Contains(text, "my-deploy") {
		t.Errorf("expected deployment name, got: %s", text)
	}
}

func TestRolloutUndo_SpecificRevision(t *testing.T) {
	dep := testDeployment("my-deploy", "default")
	rs1 := testReplicaSet("my-deploy-rs1", "default", "1", "my-deploy")
	rs2 := testReplicaSet("my-deploy-rs2", "default", "2", "my-deploy")
	rs3 := testReplicaSet("my-deploy-rs3", "default", "3", "my-deploy")

	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(dep, rs1, rs2, rs3)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_undo", func(s *server.MCPServer) {
		registerRolloutUndo(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":       "Deployment",
		"name":       "my-deploy",
		"namespace":  "default",
		"toRevision": float64(1),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Rolled back") {
		t.Errorf("expected rollback confirmation, got: %s", text)
	}
	if !strings.Contains(text, "revision 1") {
		t.Errorf("expected revision 1 in output, got: %s", text)
	}
}

func TestRolloutUndo_RevisionNotFound(t *testing.T) {
	dep := testDeployment("my-deploy", "default")
	rs1 := testReplicaSet("my-deploy-rs1", "default", "1", "my-deploy")

	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(dep, rs1)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_undo", func(s *server.MCPServer) {
		registerRolloutUndo(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":       "Deployment",
		"name":       "my-deploy",
		"namespace":  "default",
		"toRevision": float64(99),
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !res.IsError {
		t.Error("expected error for nonexistent revision")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "revision 99 not found") {
		t.Errorf("expected revision not found error, got: %s", text)
	}
}

func TestRolloutUndo_UnsupportedKind(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_undo", func(s *server.MCPServer) {
		registerRolloutUndo(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "StatefulSet",
		"name":      "test",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !res.IsError {
		t.Error("expected error for unsupported kind")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "does not support rollout undo") {
		t.Errorf("expected unsupported error, got: %s", text)
	}
}

func TestRolloutUndo_NoReplicaSets(t *testing.T) {
	dep := testDeployment("my-deploy", "default")
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(dep)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_undo", func(s *server.MCPServer) {
		registerRolloutUndo(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "my-deploy",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !res.IsError {
		t.Error("expected error when no revisions found")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "no revisions found") {
		t.Errorf("expected no revisions error, got: %s", text)
	}
}

func TestRolloutUndo_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}, AllowWrite: true}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_undo", func(s *server.MCPServer) {
		registerRolloutUndo(s, pool, cfg)
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
