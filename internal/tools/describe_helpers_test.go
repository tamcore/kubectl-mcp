package tools

import (
	"testing"
)

func TestUnstructuredNestedSlice(t *testing.T) {
	tests := []struct {
		name      string
		obj       map[string]any
		fields    []string
		wantLen   int
		wantFound bool
	}{
		{
			name: "valid path",
			obj: map[string]any{
				"status": map[string]any{
					"conditions": []any{
						map[string]any{"type": "Ready"},
					},
				},
			},
			fields:    []string{"status", "conditions"},
			wantLen:   1,
			wantFound: true,
		},
		{
			name:      "missing field",
			obj:       map[string]any{},
			fields:    []string{"status", "conditions"},
			wantLen:   0,
			wantFound: false,
		},
		{
			name: "non-slice type at leaf",
			obj: map[string]any{
				"status": map[string]any{
					"conditions": "not-a-slice",
				},
			},
			fields:    []string{"status", "conditions"},
			wantLen:   0,
			wantFound: false,
		},
		{
			name: "intermediate non-map",
			obj: map[string]any{
				"status": "string-not-map",
			},
			fields:    []string{"status", "conditions"},
			wantLen:   0,
			wantFound: false,
		},
		{
			name: "single field path",
			obj: map[string]any{
				"items": []any{"a", "b"},
			},
			fields:    []string{"items"},
			wantLen:   2,
			wantFound: true,
		},
		{
			name:      "empty fields returns nil (no iteration)",
			obj:       map[string]any{"a": "b"},
			fields:    []string{},
			wantLen:   0,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found, err := unstructuredNestedSlice(tt.obj, tt.fields...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
			}
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestMapStr(t *testing.T) {
	m := map[string]any{
		"type":   "Ready",
		"count":  42,
		"nested": map[string]any{"a": "b"},
	}

	tests := []struct {
		name string
		key  string
		want string
	}{
		{"string value", "type", "Ready"},
		{"int value", "count", "42"},
		{"missing key", "missing", ""},
		{"nested map", "nested", "map[a:b]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapStr(m, tt.key)
			if got != tt.want {
				t.Errorf("mapStr() = %q, want %q", got, tt.want)
			}
		})
	}
}
