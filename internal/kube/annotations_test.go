package kube

import (
	"reflect"
	"testing"
)

func TestFilterAnnotations(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		include  []string
		exclude  []string
		expected map[string]string
	}{
		{
			name: "default excludes last-applied-configuration",
			input: map[string]string{
				"app": "nginx",
				"kubectl.kubernetes.io/last-applied-configuration": `{"big":"json"}`,
			},
			expected: map[string]string{
				"app": "nginx",
			},
		},
		{
			name: "glob exclude pattern",
			input: map[string]string{
				"app":                       "nginx",
				"kubectl.kubernetes.io/foo": "bar",
				"kubectl.kubernetes.io/baz": "qux",
				"helm.sh/chart":             "nginx-1.0",
			},
			exclude: []string{"kubectl.kubernetes.io/*"},
			expected: map[string]string{
				"app":           "nginx",
				"helm.sh/chart": "nginx-1.0",
			},
		},
		{
			name: "include pattern keeps only matched",
			input: map[string]string{
				"app":           "nginx",
				"version":       "v2",
				"helm.sh/chart": "nginx-1.0",
			},
			include: []string{"app", "version"},
			expected: map[string]string{
				"app":     "nginx",
				"version": "v2",
			},
		},
		{
			name: "exclude takes precedence over include",
			input: map[string]string{
				"app":     "nginx",
				"version": "v2",
				"debug":   "true",
			},
			include: []string{"*"},
			exclude: []string{"debug"},
			expected: map[string]string{
				"app":     "nginx",
				"version": "v2",
			},
		},
		{
			name:     "nil annotations returns nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty annotations returns empty",
			input:    map[string]string{},
			expected: map[string]string{},
		},
		{
			name: "no patterns still applies default excludes",
			input: map[string]string{
				"app": "nginx",
				"kubectl.kubernetes.io/last-applied-configuration": `{"spec":{}}`,
			},
			expected: map[string]string{
				"app": "nginx",
			},
		},
		{
			name: "multiple exclude patterns",
			input: map[string]string{
				"app":                       "nginx",
				"kubectl.kubernetes.io/foo": "bar",
				"helm.sh/chart":             "nginx-1.0",
				"meta.helm.sh/release-name": "my-release",
			},
			exclude: []string{"kubectl.kubernetes.io/*", "*.helm.sh/*"},
			expected: map[string]string{
				"app":           "nginx",
				"helm.sh/chart": "nginx-1.0",
			},
		},
		{
			name: "include with glob",
			input: map[string]string{
				"app.kubernetes.io/name":    "nginx",
				"app.kubernetes.io/version": "1.0",
				"helm.sh/chart":             "nginx-1.0",
			},
			include: []string{"app.kubernetes.io/*"},
			expected: map[string]string{
				"app.kubernetes.io/name":    "nginx",
				"app.kubernetes.io/version": "1.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterAnnotations(tt.input, tt.include, tt.exclude)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("FilterAnnotations() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFilterAnnotations_DoesNotMutateInput(t *testing.T) {
	input := map[string]string{
		"app": "nginx",
		"kubectl.kubernetes.io/last-applied-configuration": `{"big":"json"}`,
	}

	_ = FilterAnnotations(input, nil, nil)

	// Original should still have both keys.
	if len(input) != 2 {
		t.Errorf("original map was mutated: len = %d, want 2", len(input))
	}
	if _, ok := input["kubectl.kubernetes.io/last-applied-configuration"]; !ok {
		t.Error("original map lost kubectl.kubernetes.io/last-applied-configuration")
	}
}
