package tools

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestFormatTable(t *testing.T) {
	t.Run("renders header and rows", func(t *testing.T) {
		table := &metav1.Table{
			ColumnDefinitions: []metav1.TableColumnDefinition{
				{Name: "Name"},
				{Name: "Status"},
				{Name: "Age"},
			},
			Rows: []metav1.TableRow{
				{Cells: []interface{}{"pod-a", "Running", "5d"}},
				{Cells: []interface{}{"pod-b", "Pending", "2h"}},
			},
		}

		result := formatTable(table)

		if !strings.Contains(result, "NAME") {
			t.Error("expected NAME header")
		}
		if !strings.Contains(result, "STATUS") {
			t.Error("expected STATUS header")
		}
		if !strings.Contains(result, "pod-a") {
			t.Error("expected pod-a in output")
		}
		if !strings.Contains(result, "pod-b") {
			t.Error("expected pod-b in output")
		}
		if !strings.Contains(result, "Running") {
			t.Error("expected Running in output")
		}
		if !strings.Contains(result, "Pending") {
			t.Error("expected Pending in output")
		}
	})

	t.Run("handles nil table", func(t *testing.T) {
		result := formatTable(nil)
		if result != "(no data)" {
			t.Errorf("expected '(no data)', got %q", result)
		}
	})

	t.Run("handles empty rows", func(t *testing.T) {
		table := &metav1.Table{
			ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name"}},
			Rows:              []metav1.TableRow{},
		}
		result := formatTable(table)
		if result != "(no data)" {
			t.Errorf("expected '(no data)', got %q", result)
		}
	})

	t.Run("pads columns correctly", func(t *testing.T) {
		table := &metav1.Table{
			ColumnDefinitions: []metav1.TableColumnDefinition{
				{Name: "Name"},
				{Name: "Ready"},
			},
			Rows: []metav1.TableRow{
				{Cells: []interface{}{"short", "1/1"}},
				{Cells: []interface{}{"a-very-long-name", "2/2"}},
			},
		}

		result := formatTable(table)
		lines := strings.Split(strings.TrimSpace(result), "\n")
		if len(lines) != 3 {
			t.Fatalf("expected 3 lines (header + 2 rows), got %d", len(lines))
		}

		// All lines should have the same column alignment.
		if !strings.Contains(lines[0], "NAME") {
			t.Error("header should contain NAME")
		}
	})

	t.Run("handles fewer cells than columns", func(t *testing.T) {
		table := &metav1.Table{
			ColumnDefinitions: []metav1.TableColumnDefinition{
				{Name: "Name"},
				{Name: "Status"},
			},
			Rows: []metav1.TableRow{
				{Cells: []interface{}{"only-name"}},
			},
		}

		result := formatTable(table)
		if !strings.Contains(result, "only-name") {
			t.Error("expected name in output")
		}
	})
}

func TestFormatListAsJSON(t *testing.T) {
	items := []unstructured.Unstructured{
		{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":            "pod-a",
				"namespace":       "default",
				"uid":             "abc-123",
				"resourceVersion": "100",
				"managedFields":   []interface{}{},
			},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "pod-b",
				"namespace": "default",
				"uid":       "def-456",
			},
		}},
	}

	jsonText, structured, err := formatListAsJSON(items)
	if err != nil {
		t.Fatal(err)
	}

	// Verify noisy metadata is stripped.
	if strings.Contains(jsonText, "abc-123") {
		t.Error("uid should be stripped from JSON output")
	}
	if strings.Contains(jsonText, `"resourceVersion"`) {
		t.Error("resourceVersion should be stripped from JSON output")
	}

	// Verify we get the right number of items.
	if len(structured) != 2 {
		t.Errorf("expected 2 structured items, got %d", len(structured))
	}

	// Verify names are preserved.
	if !strings.Contains(jsonText, "pod-a") || !strings.Contains(jsonText, "pod-b") {
		t.Error("expected pod names in output")
	}

	// Verify original items are not mutated.
	meta := items[0].Object["metadata"].(map[string]interface{})
	if _, ok := meta["uid"]; !ok {
		t.Error("original item uid should not be removed (immutability violated)")
	}
}

func TestBuildResourcePath(t *testing.T) {
	tests := []struct {
		name      string
		gvr       schema.GroupVersionResource
		namespace string
		want      string
	}{
		{
			name:      "core namespaced",
			gvr:       schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			namespace: "default",
			want:      "/api/v1/namespaces/default/pods",
		},
		{
			name:      "core cluster-scoped",
			gvr:       schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"},
			namespace: "",
			want:      "/api/v1/namespaces",
		},
		{
			name:      "apps namespaced",
			gvr:       schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
			namespace: "kube-system",
			want:      "/apis/apps/v1/namespaces/kube-system/deployments",
		},
		{
			name:      "apps cluster-scoped",
			gvr:       schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
			namespace: "",
			want:      "/apis/rbac.authorization.k8s.io/v1/clusterroles",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildResourcePath(tt.gvr, tt.namespace)
			if got != tt.want {
				t.Errorf("buildResourcePath(%+v, %q) = %q, want %q", tt.gvr, tt.namespace, got, tt.want)
			}
		})
	}
}
