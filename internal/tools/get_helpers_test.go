package tools

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestBetterGVRCandidate(t *testing.T) {
	core := func(exact bool) gvrCandidate {
		return gvrCandidate{
			gvr:     schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			exact:   exact,
			coreAPI: true,
		}
	}
	ext := func(exact bool) gvrCandidate {
		return gvrCandidate{
			gvr:     schema.GroupVersionResource{Group: "extensions", Version: "v1beta1", Resource: "pods"},
			exact:   exact,
			coreAPI: false,
		}
	}

	tests := []struct {
		name string
		a, b gvrCandidate
		want bool
	}{
		{"exact beats non-exact", core(true), ext(false), true},
		{"non-exact loses to exact", ext(false), core(true), false},
		{"core beats non-core (both exact)", core(true), ext(true), true},
		{"non-core loses to core (both exact)", ext(true), core(true), false},
		{"core beats non-core (both non-exact)", core(false), ext(false), true},
		{"same exactness and group returns false", core(true), core(true), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := betterGVRCandidate(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("betterGVRCandidate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchesPlural(t *testing.T) {
	tests := []struct {
		name         string
		resourceName string
		input        string
		want         bool
	}{
		{"pods matches pod", "pods", "pod", true},
		{"services matches service", "services", "service", true},
		{"ingresses matches ingress", "ingresses", "ingress", true},
		{"deployments matches deployment", "deployments", "deployment", true},
		{"no match", "pods", "service", false},
		{"exact plural input does not match", "pods", "pods", false},
		{"case insensitive resource", "Pods", "pod", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesPlural(tt.resourceName, tt.input)
			if got != tt.want {
				t.Errorf("matchesPlural(%q, %q) = %v, want %v", tt.resourceName, tt.input, got, tt.want)
			}
		})
	}
}
