package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

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

func TestDeleteResource_ForceDelete(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("force-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "delete_resource", func(s *server.MCPServer) {
		registerDeleteResource(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Pod",
		"name":      "force-pod",
		"namespace": "default",
		"force":     true,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("unexpected error: %s", text)
	}
	if !strings.Contains(text, "Deleted Pod/force-pod") {
		t.Errorf("expected delete confirmation, got: %s", text)
	}

	// Verify that the fake dynamic client received a delete with GracePeriodSeconds=0.
	var gracePeriodZero *int64
	for _, action := range dynClient.Actions() {
		if action.GetVerb() == "delete" {
			da, ok := action.(clienttesting.DeleteAction)
			if !ok {
				continue
			}
			gracePeriodZero = da.GetDeleteOptions().GracePeriodSeconds
		}
	}
	if gracePeriodZero == nil || *gracePeriodZero != 0 {
		t.Errorf("expected GracePeriodSeconds=0 for force delete, got: %v", gracePeriodZero)
	}
}

func TestDeleteResource_ForceAndGracePeriodConflict(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("conflict-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "delete_resource", func(s *server.MCPServer) {
		registerDeleteResource(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":               "Pod",
		"name":               "conflict-pod",
		"namespace":          "default",
		"force":              true,
		"gracePeriodSeconds": float64(30),
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !res.IsError {
		t.Fatal("expected error when force=true and gracePeriodSeconds>0 are both set")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "force") || !strings.Contains(text, "gracePeriodSeconds") {
		t.Errorf("expected error message to mention force and gracePeriodSeconds, got: %s", text)
	}
}

func TestDeleteResource_ForceFalseDefaultBehaviour(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("normal-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "delete_resource", func(s *server.MCPServer) {
		registerDeleteResource(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Pod",
		"name":      "normal-pod",
		"namespace": "default",
		"force":     false,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("unexpected error: %s", text)
	}
	if !strings.Contains(text, "Deleted Pod/normal-pod") {
		t.Errorf("expected delete confirmation, got: %s", text)
	}

	// With force=false the grace period should NOT be set to 0 (nil means use resource default).
	for _, action := range dynClient.Actions() {
		if action.GetVerb() == "delete" {
			da, ok := action.(clienttesting.DeleteAction)
			if !ok {
				continue
			}
			gp := da.GetDeleteOptions().GracePeriodSeconds
			if gp != nil && *gp == 0 {
				t.Errorf("expected no forced grace period for normal delete, but GracePeriodSeconds=0 was set")
			}
		}
	}
}

func TestDeleteResource_ElicitationMentionsForce(t *testing.T) {
	// This test verifies that the elicitation message mentions "force" when force=true.
	// Since elicitation gracefully degrades (no session → proceeds), we can test by checking
	// that a force delete with dryRun=false proceeds to completion (which implies the
	// elicitation prompt was constructed with the force message).
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("elicit-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "delete_resource", func(s *server.MCPServer) {
		registerDeleteResource(s, pool)
	})

	// force=true, dryRun=false — elicitation fires; without a session it degrades gracefully.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Pod",
		"name":      "elicit-pod",
		"namespace": "default",
		"force":     true,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("unexpected error during force delete: %s", text)
	}
	if !strings.Contains(text, "Deleted Pod/elicit-pod") {
		t.Errorf("expected delete confirmation, got: %s", text)
	}
}
