package tools

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"
)

func TestParseResourceRef(t *testing.T) {
	tests := []struct {
		input     string
		wantKind  string
		wantName  string
		wantError bool
	}{
		{"deployment/nginx", "deployment", "nginx", false},
		{"job/my-job", "job", "my-job", false},
		{"StatefulSet/redis", "StatefulSet", "redis", false},
		{"invalid", "", "", true},
		{"/noname", "", "", true},
		{"nokind/", "", "", true},
		{"", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			kind, name, err := parseResourceRef(tt.input)
			if tt.wantError {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if kind != tt.wantKind {
				t.Errorf("kind = %q, want %q", kind, tt.wantKind)
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestResolveResourceToLabelSelector_Deployment(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	dep := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":              "my-deploy",
			"namespace":         "default",
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "nginx",
				},
			},
		},
	}}

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(dep)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	cc, err := pool.ClientFor("test-ctx")
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}

	selector, err := resolveResourceToLabelSelector(context.Background(), cc, "default", "deployment/my-deploy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selector != "app=nginx" {
		t.Errorf("expected 'app=nginx', got: %s", selector)
	}
}

func TestResolveResourceToLabelSelector_UnsupportedKind(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	cc, err := pool.ClientFor("test-ctx")
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}

	_, err = resolveResourceToLabelSelector(context.Background(), cc, "default", "configmap/test")
	if err == nil {
		t.Error("expected error for unsupported kind")
	}
	if !strings.Contains(err.Error(), "not supported for log resolution") {
		t.Errorf("expected unsupported error, got: %v", err)
	}
}

func TestResolveResourceToLabelSelector_NotFound(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	cc, err := pool.ClientFor("test-ctx")
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}

	_, err = resolveResourceToLabelSelector(context.Background(), cc, "default", "deployment/nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent resource")
	}
}

func TestResolveResourceToLabelSelector_InvalidRef(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	cc, err := pool.ClientFor("test-ctx")
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}

	_, err = resolveResourceToLabelSelector(context.Background(), cc, "default", "invalid")
	if err == nil {
		t.Error("expected error for invalid resource ref")
	}
}
