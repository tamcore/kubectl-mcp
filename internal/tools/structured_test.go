package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetResource_HasStructuredContent(t *testing.T) {
	pod := testPod("structured-pod", "default")
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient(pod)
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_resource", func(s *server.MCPServer) {
		registerGetResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Pod",
		"name":      "structured-pod",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	// Verify StructuredContent is populated.
	if res.StructuredContent == nil {
		t.Error("expected StructuredContent to be populated")
	}

	// Verify it's the right type (map).
	obj, ok := res.StructuredContent.(map[string]interface{})
	if !ok {
		t.Fatalf("expected StructuredContent to be map[string]interface{}, got %T", res.StructuredContent)
	}

	// Verify it contains the pod name.
	meta, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("expected metadata in structured content")
	}
	if meta["name"] != "structured-pod" {
		t.Errorf("expected name=structured-pod, got %v", meta["name"])
	}

	// Verify text fallback is also present.
	text := resultText(t, res)
	if text == "" {
		t.Error("expected text fallback to be present")
	}
}

func TestListResources_HasStructuredContent(t *testing.T) {
	pod := testPod("list-struct-pod", "default")
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient(pod)
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "list_resources", func(s *server.MCPServer) {
		registerListResources(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Pod",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	if res.StructuredContent == nil {
		t.Error("expected StructuredContent to be populated")
	}

	// Verify it's a slice.
	items, ok := res.StructuredContent.([]map[string]interface{})
	if !ok {
		t.Fatalf("expected StructuredContent to be []map[string]interface{}, got %T", res.StructuredContent)
	}
	if len(items) == 0 {
		t.Error("expected at least one item in structured content")
	}
}

func TestDescribeResource_HasStructuredContent(t *testing.T) {
	pod := testPod("desc-struct-pod", "default")
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient(pod)
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "describe_resource", func(s *server.MCPServer) {
		registerDescribeResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Pod",
		"name":      "desc-struct-pod",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	if res.StructuredContent == nil {
		t.Error("expected StructuredContent to be populated")
	}

	// Verify text fallback contains formatted describe output.
	text := resultText(t, res)
	if text == "" {
		t.Error("expected text fallback")
	}
}
