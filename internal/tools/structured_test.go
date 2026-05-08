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

	// Verify it's an object envelope (not a raw array).
	envelope, ok := res.StructuredContent.(map[string]interface{})
	if !ok {
		t.Fatalf("expected StructuredContent to be map[string]interface{}, got %T", res.StructuredContent)
	}

	// Verify envelope has "items" key with the actual list.
	rawItems, ok := envelope["items"]
	if !ok {
		t.Fatal("expected 'items' key in structured content envelope")
	}
	items, ok := rawItems.([]map[string]interface{})
	if !ok {
		t.Fatalf("expected items to be []map[string]interface{}, got %T", rawItems)
	}
	if len(items) == 0 {
		t.Error("expected at least one item in structured content")
	}

	// Verify "count" matches items length.
	count, ok := envelope["count"]
	if !ok {
		t.Fatal("expected 'count' key in structured content envelope")
	}
	if count.(int) != len(items) {
		t.Errorf("expected count=%d, got %v", len(items), count)
	}
}

func TestListResources_SummaryStructuredContentIsCompact(t *testing.T) {
	pod := testPod("compact-pod", "default")
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient(pod)
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "list_resources", func(s *server.MCPServer) {
		registerListResources(s, pool, cfg)
	})

	// Default format is "summary".
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

	envelope, ok := res.StructuredContent.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map envelope, got %T", res.StructuredContent)
	}
	items, ok := envelope["items"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected items to be []map[string]interface{}, got %T", envelope["items"])
	}
	if len(items) == 0 {
		t.Fatal("expected at least one item")
	}

	item := items[0]
	// Summary items must have compact fields, not full Kubernetes objects.
	if _, hasSpec := item["spec"]; hasSpec {
		t.Error("summary structuredContent should NOT contain 'spec' (full object leaked)")
	}
	if statusVal, hasStatus := item["status"]; hasStatus {
		// "status" as a string (e.g. "Running") is fine; as a map is not.
		if _, isMap := statusVal.(map[string]interface{}); isMap {
			t.Error("summary structuredContent should NOT contain 'status' as a nested object (full object leaked)")
		}
	}
	// Verify it has the expected compact fields.
	if _, hasName := item["name"]; !hasName {
		t.Error("summary item should have 'name'")
	}
	if _, hasAge := item["age"]; !hasAge {
		t.Error("summary item should have 'age'")
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
