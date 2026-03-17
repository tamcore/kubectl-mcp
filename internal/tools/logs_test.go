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

// testRunningPod returns an unstructured Pod with the given labels and Running phase.
func testRunningPod(name, ns string, labels map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":              name,
			"namespace":         ns,
			"labels":            labels,
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"spec": map[string]interface{}{
			"nodeName": "node-1",
		},
		"status": map[string]interface{}{
			"phase": "Running",
		},
	}}
}

func TestGetLogs_RequiresPodOrLabelSelector(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	// Neither pod nor labelSelector provided.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error when neither pod nor labelSelector is provided")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "pod") || !strings.Contains(text, "labelSelector") {
		t.Errorf("expected error mentioning pod and labelSelector, got: %s", text)
	}
}

func TestGetLogs_LabelSelectorNoPods(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient() // no pods
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":     "default",
		"labelSelector": "app=nginx",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error when no pods match selector")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "no pods") {
		t.Errorf("expected 'no pods' message, got: %s", text)
	}
}

func TestGetLogs_LabelSelectorFindsPods(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient(
		testRunningPod("nginx-1", "default", map[string]interface{}{"app": "nginx"}),
		testRunningPod("nginx-2", "default", map[string]interface{}{"app": "nginx"}),
		testRunningPod("redis-1", "default", map[string]interface{}{"app": "redis"}),
	)
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	// The fake clientset can't stream logs, so the handler will try and
	// fail to stream for each matched pod. But we can verify that it found
	// the right pods by checking the error mentions both nginx pods but not redis.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":     "default",
		"labelSelector": "app=nginx",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	// Should attempt logs for nginx-1 and nginx-2 (error or success).
	if !strings.Contains(text, "nginx-1") || !strings.Contains(text, "nginx-2") {
		t.Errorf("expected both nginx pods in output, got: %s", text)
	}
	// Should NOT include redis.
	if strings.Contains(text, "redis-1") {
		t.Errorf("should not include redis pod, got: %s", text)
	}
}

func TestGetLogs_LabelSelectorContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}}
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":     "default",
		"labelSelector": "app=nginx",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for disallowed context")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not-allowed error, got: %s", text)
	}
}

func TestGetLogs_SinceAndSinceTimeMutuallyExclusive(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"since":     "5m",
		"sinceTime": "2024-01-15T10:00:00Z",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error when both since and sinceTime are provided")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "mutually exclusive") {
		t.Errorf("expected mutual exclusion error, got: %s", text)
	}
}

func TestGetLogs_SinceTimeInvalidFormat(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"sinceTime": "not-a-date",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for invalid sinceTime format")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "RFC3339") {
		t.Errorf("expected RFC3339 format error, got: %s", text)
	}
}

func TestGetLogs_PodStillWorksAlone(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	// Pod-only path should still work (will error on streaming, but not on validation).
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
	}))
	if err != nil {
		t.Fatal(err)
	}
	// The fake fails on streaming but should NOT fail on validation.
	text := resultText(t, res)
	if strings.Contains(text, "pod") && strings.Contains(text, "labelSelector") {
		t.Errorf("should not get validation error when pod is provided, got: %s", text)
	}
}
