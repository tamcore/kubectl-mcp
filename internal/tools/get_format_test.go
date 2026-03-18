package tools

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/mark3labs/mcp-go/server"
)

func TestGetResourceFormatParameter(t *testing.T) {
	pod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":              "fmt-pod",
			"namespace":         "default",
			"creationTimestamp": "2024-01-01T00:00:00Z",
			"uid":               "uid-abc",
			"resourceVersion":   "42",
			"managedFields":     []interface{}{map[string]interface{}{"manager": "kubectl"}},
		},
		"spec": map[string]interface{}{
			"nodeName": "node-1",
		},
		"status": map[string]interface{}{
			"phase": "Running",
			"containerStatuses": []interface{}{
				map[string]interface{}{
					"ready":        true,
					"restartCount": int64(0),
					"state": map[string]interface{}{
						"running": map[string]interface{}{
							"startedAt": "2024-01-01T00:00:00Z",
						},
					},
				},
			},
		},
	}}

	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient(pod)
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_resource", func(s *server.MCPServer) {
		registerGetResource(s, pool, cfg)
	})

	t.Run("format=full returns JSON with metadata stripped", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"name":      "fmt-pod",
			"namespace": "default",
			"format":    "full",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		if !strings.Contains(text, "nodeName") {
			t.Error("expected spec.nodeName in full output")
		}
		if strings.Contains(text, "uid-abc") {
			t.Error("uid should be stripped from full output")
		}
		if strings.Contains(text, "managedFields") {
			t.Error("managedFields should be stripped from full output")
		}
	})

	t.Run("default format is full", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"name":      "fmt-pod",
			"namespace": "default",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		// Default should be full JSON with spec etc.
		if !strings.Contains(text, "nodeName") {
			t.Error("default format should include full object (spec)")
		}
	})

	t.Run("format=summary returns compact output", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"name":      "fmt-pod",
			"namespace": "default",
			"format":    "summary",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		if !strings.Contains(text, "fmt-pod") {
			t.Error("expected pod name in summary")
		}
		if !strings.Contains(text, "Running") {
			t.Error("expected status in summary")
		}
		// Summary should NOT contain spec/status raw objects.
		if strings.Contains(text, "containerStatuses") {
			t.Error("summary should not contain raw containerStatuses")
		}
	})

	t.Run("format=yaml returns YAML output", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"name":      "fmt-pod",
			"namespace": "default",
			"format":    "yaml",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		// YAML should contain field names without JSON quotes.
		if !strings.Contains(text, "nodeName:") {
			t.Error("expected YAML-style nodeName: in output")
		}
		if strings.Contains(text, "uid-abc") {
			t.Error("uid should be stripped from YAML output")
		}
		if strings.Contains(text, "managedFields") {
			t.Error("managedFields should be stripped from YAML output")
		}
	})

	t.Run("invalid format returns error", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"name":      "fmt-pod",
			"namespace": "default",
			"format":    "invalid",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected error for invalid format")
		}
		text := resultText(t, res)
		if !strings.Contains(text, "invalid format") {
			t.Errorf("expected 'invalid format' error, got: %s", text)
		}
	})
}

func TestGetResourceSummaryGenericKind(t *testing.T) {
	// Test summary format for an unknown kind (not Pod, Deployment, etc.)
	ns := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name":              "test-ns",
			"creationTimestamp": "2024-01-01T00:00:00Z",
			"labels":            map[string]interface{}{"env": "test"},
		},
	}}

	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient(ns)
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_resource", func(s *server.MCPServer) {
		registerGetResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":   "Namespace",
		"name":   "test-ns",
		"format": "summary",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}
	text := resultText(t, res)
	if !strings.Contains(text, "test-ns") {
		t.Error("expected namespace name in summary")
	}
	if !strings.Contains(text, "Namespace") {
		t.Error("expected kind in generic summary")
	}
	if !strings.Contains(text, "apiVersion") {
		t.Error("expected apiVersion in generic summary")
	}
	if !strings.Contains(text, "env") {
		t.Error("expected labels in generic summary")
	}
}

func TestEnrichGeneric(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name": "test-cm",
			"labels": map[string]interface{}{
				"app": "web",
			},
		},
	}}

	s := baseFields(*obj)
	enrichGeneric(s, obj)

	if s["kind"] != "ConfigMap" {
		t.Errorf("expected kind=ConfigMap, got %v", s["kind"])
	}
	if s["apiVersion"] != "v1" {
		t.Errorf("expected apiVersion=v1, got %v", s["apiVersion"])
	}
	labels, ok := s["labels"].(map[string]string)
	if !ok {
		t.Fatal("expected labels to be map[string]string")
	}
	if labels["app"] != "web" {
		t.Errorf("expected label app=web, got %v", labels["app"])
	}
}

func TestEnrichGenericNoLabels(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name": "test-cm",
		},
	}}

	s := baseFields(*obj)
	enrichGeneric(s, obj)

	if _, ok := s["labels"]; ok {
		t.Error("labels should not be present when empty")
	}
}
