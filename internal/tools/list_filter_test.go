package tools

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestParseFilters(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
		negate  []bool
	}{
		{"single equality", "status.phase=Running", 1, false, []bool{false}},
		{"equality and negation", "a=b,c!=d", 2, false, []bool{false, true}},
		{"empty string", "", 0, false, nil},
		{"only commas", ",,,", 0, false, nil},
		{"invalid no operator", "nooperator", 0, true, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFilters(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseFilters(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if len(got) != tt.want {
				t.Fatalf("parseFilters(%q) returned %d filters, want %d", tt.raw, len(got), tt.want)
			}
			for i, f := range got {
				if f.negate != tt.negate[i] {
					t.Errorf("filter[%d] negate = %v, want %v", i, f.negate, tt.negate[i])
				}
			}
		})
	}

	// Verify path splitting.
	t.Run("path splitting", func(t *testing.T) {
		filters, _ := parseFilters("status.phase=Running")
		if len(filters[0].path) != 2 || filters[0].path[0] != "status" || filters[0].path[1] != "phase" {
			t.Errorf("expected path [status phase], got %v", filters[0].path)
		}
		if filters[0].value != "Running" {
			t.Errorf("expected value Running, got %q", filters[0].value)
		}
	})
}

func TestApplyFilters(t *testing.T) {
	items := []unstructured.Unstructured{
		{Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "pod1"},
			"status":   map[string]interface{}{"phase": "Running"},
		}},
		{Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "pod2"},
			"status":   map[string]interface{}{"phase": "Pending"},
		}},
	}

	t.Run("no filters returns all", func(t *testing.T) {
		got := applyFilters(items, nil)
		if len(got) != 2 {
			t.Errorf("expected 2 items, got %d", len(got))
		}
	})

	t.Run("filter keeps matching", func(t *testing.T) {
		filters := []filterExpr{{path: []string{"status", "phase"}, value: "Running"}}
		got := applyFilters(items, filters)
		if len(got) != 1 {
			t.Fatalf("expected 1 item, got %d", len(got))
		}
		if got[0].Object["metadata"].(map[string]interface{})["name"] != "pod1" {
			t.Error("expected pod1 to match")
		}
	})

	t.Run("filter removes all", func(t *testing.T) {
		filters := []filterExpr{{path: []string{"status", "phase"}, value: "Succeeded"}}
		got := applyFilters(items, filters)
		if len(got) != 0 {
			t.Errorf("expected 0 items, got %d", len(got))
		}
	})
}

func TestMatchesAllFilters(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{"phase": "Running"},
		"spec":   map[string]interface{}{"nodeName": "node-1"},
	}

	tests := []struct {
		name    string
		filters []filterExpr
		want    bool
	}{
		{
			name: "all match",
			filters: []filterExpr{
				{path: []string{"status", "phase"}, value: "Running"},
				{path: []string{"spec", "nodeName"}, value: "node-1"},
			},
			want: true,
		},
		{
			name: "one doesn't match",
			filters: []filterExpr{
				{path: []string{"status", "phase"}, value: "Running"},
				{path: []string{"spec", "nodeName"}, value: "node-2"},
			},
			want: false,
		},
		{
			name: "negation match",
			filters: []filterExpr{
				{path: []string{"status", "phase"}, value: "Pending", negate: true},
			},
			want: true,
		},
		{
			name: "negation no match",
			filters: []filterExpr{
				{path: []string{"status", "phase"}, value: "Running", negate: true},
			},
			want: false,
		},
		{
			name: "missing field fails equality",
			filters: []filterExpr{
				{path: []string{"missing", "path"}, value: "x"},
			},
			want: false,
		},
		{
			name: "missing field fails negation",
			filters: []filterExpr{
				{path: []string{"missing", "path"}, value: "x", negate: true},
			},
			want: false,
		},
		{
			name:    "empty filters matches",
			filters: nil,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesAllFilters(obj, tt.filters)
			if got != tt.want {
				t.Errorf("matchesAllFilters() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNestedFieldValue(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"phase": "Running",
			"containerStatuses": []interface{}{
				map[string]interface{}{"ready": true},
				map[string]interface{}{"ready": false},
			},
		},
		"notAMap": "string",
	}

	tests := []struct {
		name      string
		path      []string
		wantVal   string
		wantFound bool
	}{
		{"nested map", []string{"status", "phase"}, "Running", true},
		{"array index 0", []string{"status", "containerStatuses", "0", "ready"}, "true", true},
		{"array index 1", []string{"status", "containerStatuses", "1", "ready"}, "false", true},
		{"missing field", []string{"status", "missing"}, "", false},
		{"array index out of bounds", []string{"status", "containerStatuses", "5"}, "", false},
		{"non-numeric array index", []string{"status", "containerStatuses", "abc"}, "", false},
		{"traverse through non-map", []string{"notAMap", "sub"}, "", false},
		{"empty path returns root", []string{}, "", true}, // fmt.Sprintf of the map
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, found := nestedFieldValue(obj, tt.path)
			if found != tt.wantFound {
				t.Errorf("nestedFieldValue() found = %v, want %v", found, tt.wantFound)
			}
			if found && tt.wantVal != "" && val != tt.wantVal {
				t.Errorf("nestedFieldValue() = %q, want %q", val, tt.wantVal)
			}
		})
	}
}
