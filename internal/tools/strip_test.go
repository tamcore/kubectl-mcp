package tools

import (
	"testing"
)

func TestStripNoisyMetadata(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]any
		checkFn func(t *testing.T, result map[string]any)
	}{
		{
			name: "removes uid and resourceVersion",
			input: map[string]any{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]any{
					"name":            "my-pod",
					"uid":             "abc-123",
					"resourceVersion": "999",
				},
			},
			checkFn: func(t *testing.T, result map[string]any) {
				meta := result["metadata"].(map[string]any)
				if _, ok := meta["uid"]; ok {
					t.Error("uid should be removed")
				}
				if _, ok := meta["resourceVersion"]; ok {
					t.Error("resourceVersion should be removed")
				}
				if meta["name"] != "my-pod" {
					t.Error("name should be preserved")
				}
			},
		},
		{
			name: "removes generation and selfLink",
			input: map[string]any{
				"metadata": map[string]any{
					"name":       "test",
					"generation": int64(5),
					"selfLink":   "/api/v1/pods/test",
				},
			},
			checkFn: func(t *testing.T, result map[string]any) {
				meta := result["metadata"].(map[string]any)
				if _, ok := meta["generation"]; ok {
					t.Error("generation should be removed")
				}
				if _, ok := meta["selfLink"]; ok {
					t.Error("selfLink should be removed")
				}
			},
		},
		{
			name: "removes managedFields",
			input: map[string]any{
				"metadata": map[string]any{
					"name": "test",
					"managedFields": []any{
						map[string]any{"manager": "kubectl"},
					},
				},
			},
			checkFn: func(t *testing.T, result map[string]any) {
				meta := result["metadata"].(map[string]any)
				if _, ok := meta["managedFields"]; ok {
					t.Error("managedFields should be removed")
				}
			},
		},
		{
			name: "removes empty string values from metadata",
			input: map[string]any{
				"metadata": map[string]any{
					"name":         "test",
					"generateName": "",
				},
			},
			checkFn: func(t *testing.T, result map[string]any) {
				meta := result["metadata"].(map[string]any)
				if _, ok := meta["generateName"]; ok {
					t.Error("empty string should be removed")
				}
			},
		},
		{
			name: "removes nil values from metadata",
			input: map[string]any{
				"metadata": map[string]any{
					"name":      "test",
					"nullField": nil,
				},
			},
			checkFn: func(t *testing.T, result map[string]any) {
				meta := result["metadata"].(map[string]any)
				if _, ok := meta["nullField"]; ok {
					t.Error("nil value should be removed")
				}
			},
		},
		{
			name: "removes empty map values from metadata",
			input: map[string]any{
				"metadata": map[string]any{
					"name":   "test",
					"labels": map[string]any{},
				},
			},
			checkFn: func(t *testing.T, result map[string]any) {
				meta := result["metadata"].(map[string]any)
				if _, ok := meta["labels"]; ok {
					t.Error("empty map should be removed")
				}
			},
		},
		{
			name: "removes empty slice values from metadata",
			input: map[string]any{
				"metadata": map[string]any{
					"name":       "test",
					"finalizers": []any{},
				},
			},
			checkFn: func(t *testing.T, result map[string]any) {
				meta := result["metadata"].(map[string]any)
				if _, ok := meta["finalizers"]; ok {
					t.Error("empty slice should be removed")
				}
			},
		},
		{
			name: "preserves non-empty metadata fields",
			input: map[string]any{
				"metadata": map[string]any{
					"name":              "test",
					"namespace":         "default",
					"creationTimestamp": "2024-01-01T00:00:00Z",
					"labels":            map[string]any{"app": "web"},
					"annotations":       map[string]any{"note": "hello"},
				},
			},
			checkFn: func(t *testing.T, result map[string]any) {
				meta := result["metadata"].(map[string]any)
				for _, key := range []string{"name", "namespace", "creationTimestamp", "labels", "annotations"} {
					if _, ok := meta[key]; !ok {
						t.Errorf("expected %q to be preserved", key)
					}
				}
			},
		},
		{
			name: "does not mutate input",
			input: map[string]any{
				"metadata": map[string]any{
					"name":            "test",
					"uid":             "abc",
					"resourceVersion": "1",
				},
			},
			checkFn: func(t *testing.T, _ map[string]any) {
				// Verified via the separate mutation check below.
			},
		},
		{
			name:  "handles missing metadata gracefully",
			input: map[string]any{"apiVersion": "v1"},
			checkFn: func(t *testing.T, result map[string]any) {
				if result["apiVersion"] != "v1" {
					t.Error("apiVersion should be preserved")
				}
			},
		},
		{
			name: "handles non-map metadata gracefully",
			input: map[string]any{
				"metadata": "not-a-map",
			},
			checkFn: func(t *testing.T, result map[string]any) {
				if result["metadata"] != "not-a-map" {
					t.Error("non-map metadata should be preserved as-is")
				}
			},
		},
		{
			name: "preserves spec and status",
			input: map[string]any{
				"metadata": map[string]any{
					"name": "test",
					"uid":  "abc",
				},
				"spec":   map[string]any{"replicas": int64(3)},
				"status": map[string]any{"phase": "Running"},
			},
			checkFn: func(t *testing.T, result map[string]any) {
				spec := result["spec"].(map[string]any)
				if spec["replicas"] != int64(3) {
					t.Error("spec should be preserved")
				}
				status := result["status"].(map[string]any)
				if status["phase"] != "Running" {
					t.Error("status should be preserved")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripNoisyMetadata(tt.input)
			tt.checkFn(t, result)
		})
	}

	// Dedicated immutability test.
	t.Run("immutability", func(t *testing.T) {
		original := map[string]any{
			"metadata": map[string]any{
				"name":            "test",
				"uid":             "abc-123",
				"resourceVersion": "42",
				"managedFields":   []any{map[string]any{"manager": "x"}},
			},
		}

		_ = StripNoisyMetadata(original)

		meta := original["metadata"].(map[string]any)
		if _, ok := meta["uid"]; !ok {
			t.Error("original uid should not be removed (immutability violated)")
		}
		if _, ok := meta["resourceVersion"]; !ok {
			t.Error("original resourceVersion should not be removed (immutability violated)")
		}
		if _, ok := meta["managedFields"]; !ok {
			t.Error("original managedFields should not be removed (immutability violated)")
		}
	})
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want bool
	}{
		{"nil", nil, true},
		{"empty string", "", true},
		{"non-empty string", "hello", false},
		{"empty map", map[string]any{}, true},
		{"non-empty map", map[string]any{"a": 1}, false},
		{"empty slice", []any{}, true},
		{"non-empty slice", []any{1}, false},
		{"int64", int64(0), false},
		{"bool false", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEmpty(tt.val)
			if got != tt.want {
				t.Errorf("isEmpty(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}
