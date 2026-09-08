package tools

import (
	"testing"
)

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
