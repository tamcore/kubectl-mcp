package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"
)

// ---------------------------------------------------------------------------
// Unit tests for sortUnstructured
// ---------------------------------------------------------------------------

func TestSortUnstructured(t *testing.T) {
	makeItem := func(name string) unstructured.Unstructured {
		return unstructured.Unstructured{Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": name},
		}}
	}

	items := []unstructured.Unstructured{
		makeItem("charlie"),
		makeItem("alpha"),
		makeItem("bravo"),
	}

	t.Run("ascending by metadata.name", func(t *testing.T) {
		got := make([]unstructured.Unstructured, len(items))
		copy(got, items)

		if err := sortUnstructured(got, "metadata.name", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got[0].GetName() != "alpha" || got[1].GetName() != "bravo" || got[2].GetName() != "charlie" {
			t.Errorf("unexpected order: %v %v %v", got[0].GetName(), got[1].GetName(), got[2].GetName())
		}
	})

	t.Run("descending by metadata.name", func(t *testing.T) {
		got := make([]unstructured.Unstructured, len(items))
		copy(got, items)

		if err := sortUnstructured(got, "metadata.name", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got[0].GetName() != "charlie" || got[1].GetName() != "bravo" || got[2].GetName() != "alpha" {
			t.Errorf("unexpected order: %v %v %v", got[0].GetName(), got[1].GetName(), got[2].GetName())
		}
	})

	t.Run("unknown field path returns error", func(t *testing.T) {
		got := make([]unstructured.Unstructured, len(items))
		copy(got, items)

		err := sortUnstructured(got, "nonexistent.field", false)
		if err == nil {
			t.Fatal("expected error for unknown field path")
		}
		if !strings.Contains(err.Error(), "nonexistent.field") {
			t.Errorf("error should mention field path, got: %v", err)
		}
	})

	t.Run("empty list does not error", func(t *testing.T) {
		if err := sortUnstructured(nil, "metadata.name", false); err != nil {
			t.Fatalf("unexpected error on empty list: %v", err)
		}
	})

	t.Run("single item does not error", func(t *testing.T) {
		single := []unstructured.Unstructured{makeItem("only")}
		if err := sortUnstructured(single, "metadata.name", false); err != nil {
			t.Fatalf("unexpected error on single item: %v", err)
		}
	})
}

func TestSortUnstructuredByTimestamp(t *testing.T) {
	makeItemWithTS := func(name, ts string) unstructured.Unstructured {
		return unstructured.Unstructured{Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":              name,
				"creationTimestamp": ts,
			},
		}}
	}

	items := []unstructured.Unstructured{
		makeItemWithTS("newest", "2024-03-01T00:00:00Z"),
		makeItemWithTS("oldest", "2024-01-01T00:00:00Z"),
		makeItemWithTS("middle", "2024-02-01T00:00:00Z"),
	}

	t.Run("ascending timestamp (oldest first)", func(t *testing.T) {
		got := make([]unstructured.Unstructured, len(items))
		copy(got, items)

		if err := sortUnstructured(got, "metadata.creationTimestamp", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got[0].GetName() != "oldest" || got[1].GetName() != "middle" || got[2].GetName() != "newest" {
			t.Errorf("unexpected order: %v %v %v", got[0].GetName(), got[1].GetName(), got[2].GetName())
		}
	})

	t.Run("descending timestamp (newest first)", func(t *testing.T) {
		got := make([]unstructured.Unstructured, len(items))
		copy(got, items)

		if err := sortUnstructured(got, "metadata.creationTimestamp", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got[0].GetName() != "newest" || got[1].GetName() != "middle" || got[2].GetName() != "oldest" {
			t.Errorf("unexpected order: %v %v %v", got[0].GetName(), got[1].GetName(), got[2].GetName())
		}
	})
}

// ---------------------------------------------------------------------------
// Integration tests via the list_resources handler
// ---------------------------------------------------------------------------

func TestListResourcesSortBy(t *testing.T) {
	pod1 := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":              "pod-charlie",
			"namespace":         "default",
			"creationTimestamp": "2024-03-01T00:00:00Z",
		},
		"status": map[string]interface{}{"phase": "Running"},
	}}
	pod2 := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":              "pod-alpha",
			"namespace":         "default",
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"status": map[string]interface{}{"phase": "Running"},
	}}
	pod3 := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":              "pod-bravo",
			"namespace":         "default",
			"creationTimestamp": "2024-02-01T00:00:00Z",
		},
		"status": map[string]interface{}{"phase": "Running"},
	}}

	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient(pod1, pod2, pod3)
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "list_resources", func(s *server.MCPServer) {
		registerListResources(s, pool, cfg)
	})

	t.Run("sortBy=.metadata.name ascending", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"namespace": "default",
			"sortBy":    ".metadata.name",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		alphaPos := strings.Index(text, "pod-alpha")
		bravoPos := strings.Index(text, "pod-bravo")
		charliePos := strings.Index(text, "pod-charlie")
		if alphaPos < 0 || bravoPos < 0 || charliePos < 0 {
			t.Fatal("not all pods found in output")
		}
		if alphaPos >= bravoPos || bravoPos >= charliePos {
			t.Errorf("expected alphabetical order alpha < bravo < charlie, got positions %d %d %d", alphaPos, bravoPos, charliePos)
		}
	})

	t.Run("sortBy=-.metadata.name descending", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"namespace": "default",
			"sortBy":    "-.metadata.name",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		alphaPos := strings.Index(text, "pod-alpha")
		bravoPos := strings.Index(text, "pod-bravo")
		charliePos := strings.Index(text, "pod-charlie")
		if alphaPos < 0 || bravoPos < 0 || charliePos < 0 {
			t.Fatal("not all pods found in output")
		}
		if charliePos >= bravoPos || bravoPos >= alphaPos {
			t.Errorf("expected reverse order charlie < bravo < alpha, got positions %d %d %d", charliePos, bravoPos, alphaPos)
		}
	})

	t.Run("sortBy=.metadata.creationTimestamp ascending (oldest first)", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"namespace": "default",
			"sortBy":    ".metadata.creationTimestamp",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		alphaPos := strings.Index(text, "pod-alpha")
		bravoPos := strings.Index(text, "pod-bravo")
		charliePos := strings.Index(text, "pod-charlie")
		if alphaPos < 0 || bravoPos < 0 || charliePos < 0 {
			t.Fatal("not all pods found in output")
		}
		// alpha=Jan, bravo=Feb, charlie=Mar → oldest first
		if alphaPos >= bravoPos || bravoPos >= charliePos {
			t.Errorf("expected oldest-first order alpha < bravo < charlie, got positions %d %d %d", alphaPos, bravoPos, charliePos)
		}
	})

	t.Run("sortBy=-.metadata.creationTimestamp descending (newest first)", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"namespace": "default",
			"sortBy":    "-.metadata.creationTimestamp",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		alphaPos := strings.Index(text, "pod-alpha")
		bravoPos := strings.Index(text, "pod-bravo")
		charliePos := strings.Index(text, "pod-charlie")
		if alphaPos < 0 || bravoPos < 0 || charliePos < 0 {
			t.Fatal("not all pods found in output")
		}
		// charlie=Mar (newest), bravo=Feb, alpha=Jan (oldest) → newest first
		if charliePos >= bravoPos || bravoPos >= alphaPos {
			t.Errorf("expected newest-first order charlie < bravo < alpha, got positions %d %d %d", charliePos, bravoPos, alphaPos)
		}
	})

	t.Run("sortBy=.nonexistent.field returns tool error", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"namespace": "default",
			"sortBy":    ".nonexistent.field",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Errorf("expected error for unknown field path, got: %s", resultText(t, res))
		}
	})

	t.Run("sortBy works with format=json", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"namespace": "default",
			"sortBy":    ".metadata.name",
			"format":    "json",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		alphaPos := strings.Index(text, "pod-alpha")
		bravoPos := strings.Index(text, "pod-bravo")
		charliePos := strings.Index(text, "pod-charlie")
		if alphaPos < 0 || bravoPos < 0 || charliePos < 0 {
			t.Fatal("not all pods found in json output")
		}
		if alphaPos >= bravoPos || bravoPos >= charliePos {
			t.Errorf("expected alphabetical order in JSON, got positions %d %d %d", alphaPos, bravoPos, charliePos)
		}
	})

	t.Run("sortBy without leading dot also works", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"namespace": "default",
			"sortBy":    "metadata.name",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		alphaPos := strings.Index(text, "pod-alpha")
		bravoPos := strings.Index(text, "pod-bravo")
		charliePos := strings.Index(text, "pod-charlie")
		if alphaPos < 0 || bravoPos < 0 || charliePos < 0 {
			t.Fatal("not all pods found in output")
		}
		if alphaPos >= bravoPos || bravoPos >= charliePos {
			t.Errorf("expected alphabetical order without leading dot, got positions %d %d %d", alphaPos, bravoPos, charliePos)
		}
	})
}
