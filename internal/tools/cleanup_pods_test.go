package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func testPodWithStatus(name, ns, phase, reason string) *unstructured.Unstructured {
	status := map[string]interface{}{"phase": phase}
	if reason != "" {
		status["reason"] = reason
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":              name,
			"namespace":         ns,
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"status": status,
	}}
}

func TestCleanupPods_DryRun(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowDestructive = true

	evicted := testPodWithStatus("evicted-pod", "default", "Failed", "Evicted")
	failed := testPodWithStatus("failed-pod", "default", "Failed", "")
	succeeded := testPodWithStatus("done-pod", "default", "Succeeded", "")
	running := testPodWithStatus("running-pod", "default", "Running", "")

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(evicted, failed, succeeded, running)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "cleanup_pods", func(s *server.MCPServer) {
		registerCleanupPods(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"dryRun":    true,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "DRY RUN") {
		t.Errorf("expected DRY RUN header, got: %s", text)
	}
	if !strings.Contains(text, "evicted-pod") {
		t.Error("expected evicted-pod in output")
	}
	if !strings.Contains(text, "failed-pod") {
		t.Error("expected failed-pod in output")
	}
	if !strings.Contains(text, "done-pod") {
		t.Error("expected done-pod in output")
	}
	if strings.Contains(text, "running-pod") {
		t.Error("running-pod should not be included")
	}
}

func TestCleanupPods_Execute(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowDestructive = true

	failed := testPodWithStatus("failed-pod", "default", "Failed", "")

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(failed)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "cleanup_pods", func(s *server.MCPServer) {
		registerCleanupPods(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Deleted: 1 pods") {
		t.Errorf("expected 1 deleted pod, got: %s", text)
	}
	if !strings.Contains(text, "failed-pod") {
		t.Error("expected failed-pod in output")
	}
}

func TestCleanupPods_CustomStates(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowDestructive = true

	succeeded := testPodWithStatus("done-pod", "default", "Succeeded", "")
	failed := testPodWithStatus("failed-pod", "default", "Failed", "")

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(succeeded, failed)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "cleanup_pods", func(s *server.MCPServer) {
		registerCleanupPods(s, pool, cfg)
	})

	// Only clean up Succeeded pods.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"states":    "Succeeded",
		"dryRun":    true,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "done-pod") {
		t.Error("expected done-pod")
	}
	if strings.Contains(text, "failed-pod") {
		t.Error("failed-pod should not be included when only Succeeded is targeted")
	}
}

func TestCleanupPods_NoPods(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowDestructive = true

	running := testPodWithStatus("running-pod", "default", "Running", "")

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(running)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "cleanup_pods", func(s *server.MCPServer) {
		registerCleanupPods(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "No pods in states") {
		t.Errorf("expected no matching pods message, got: %s", text)
	}
}

func TestCleanupPods_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}, AllowDestructive: true}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "cleanup_pods", func(s *server.MCPServer) {
		registerCleanupPods(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
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

func TestDetectPodState(t *testing.T) {
	tests := []struct {
		name   string
		phase  string
		reason string
		want   string
	}{
		{"evicted", "Failed", "Evicted", "Evicted"},
		{"failed", "Failed", "", "Failed"},
		{"succeeded", "Succeeded", "", "Succeeded"},
		{"running", "Running", "", "Running"},
		{"pending", "Pending", "", "Pending"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := testPodWithStatus("test", "default", tt.phase, tt.reason)
			got := detectPodState(*pod)
			if got != tt.want {
				t.Errorf("detectPodState() = %q, want %q", got, tt.want)
			}
		})
	}
}
