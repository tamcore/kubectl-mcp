package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/kube"
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

	dep := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":              "my-deploy",
			"namespace":         "default",
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"spec": map[string]any{
			"selector": map[string]any{
				"matchLabels": map[string]any{
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

func TestResolveResourceToLabelSelector_CronJob(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	cronJob := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "CronJob",
		"metadata": map[string]any{
			"name":              "my-cronjob",
			"namespace":         "default",
			"uid":               "cj-uid-123",
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"spec": map[string]any{
			"schedule": "*/5 * * * *",
		},
	}}

	// Older Job owned by the CronJob.
	oldJob := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]any{
			"name":              "my-cronjob-111",
			"namespace":         "default",
			"creationTimestamp": "2024-01-01T00:00:00Z",
			"ownerReferences": []any{
				map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "CronJob",
					"name":       "my-cronjob",
					"uid":        "cj-uid-123",
				},
			},
		},
		"spec": map[string]any{
			"selector": map[string]any{
				"matchLabels": map[string]any{
					"batch.kubernetes.io/controller-uid": "old-job-uid",
				},
			},
		},
	}}

	// Newer Job owned by the CronJob — should be selected.
	newJob := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]any{
			"name":              "my-cronjob-222",
			"namespace":         "default",
			"creationTimestamp": "2024-06-01T00:00:00Z",
			"ownerReferences": []any{
				map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "CronJob",
					"name":       "my-cronjob",
					"uid":        "cj-uid-123",
				},
			},
		},
		"spec": map[string]any{
			"selector": map[string]any{
				"matchLabels": map[string]any{
					"batch.kubernetes.io/controller-uid": "new-job-uid",
				},
			},
		},
	}}

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(cronJob, oldJob, newJob)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	cc, err := pool.ClientFor("test-ctx")
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}

	selector, err := resolveResourceToLabelSelector(context.Background(), cc, "default", "cronjob/my-cronjob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selector != "batch.kubernetes.io/controller-uid=new-job-uid" {
		t.Errorf("expected selector from newest job, got: %s", selector)
	}
}

func TestResolveResourceToLabelSelector_CronJob_ShortName(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	cronJob := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "CronJob",
		"metadata": map[string]any{
			"name":              "my-cj",
			"namespace":         "default",
			"uid":               "cj-uid-456",
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"spec": map[string]any{
			"schedule": "0 * * * *",
		},
	}}

	job := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]any{
			"name":              "my-cj-run1",
			"namespace":         "default",
			"creationTimestamp": "2024-01-01T01:00:00Z",
			"ownerReferences": []any{
				map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "CronJob",
					"name":       "my-cj",
					"uid":        "cj-uid-456",
				},
			},
		},
		"spec": map[string]any{
			"selector": map[string]any{
				"matchLabels": map[string]any{
					"app": "worker",
				},
			},
		},
	}}

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(cronJob, job)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	cc, err := pool.ClientFor("test-ctx")
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}

	// Use short name "cj" instead of "cronjob".
	selector, err := resolveResourceToLabelSelector(context.Background(), cc, "default", "cj/my-cj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selector != "app=worker" {
		t.Errorf("expected 'app=worker', got: %s", selector)
	}
}

func TestResolveResourceToLabelSelector_CronJob_NoJobs(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	cronJob := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "CronJob",
		"metadata": map[string]any{
			"name":              "empty-cj",
			"namespace":         "default",
			"uid":               "cj-uid-789",
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"spec": map[string]any{
			"schedule": "0 0 * * *",
		},
	}}

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(cronJob)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	cc, err := pool.ClientFor("test-ctx")
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}

	_, err = resolveResourceToLabelSelector(context.Background(), cc, "default", "cronjob/empty-cj")
	if err == nil {
		t.Error("expected error when no jobs exist for cronjob")
	}
	if !strings.Contains(err.Error(), "no jobs found owned by CronJob") {
		t.Errorf("expected 'no jobs found' error, got: %v", err)
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

// --- Coverage tests for error branches ---

func TestResolveResourceToLabelSelector_ResolveGVRError(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	// Build pool with empty discovery so resolveGVR fails.
	disc := fakeCS.Discovery().(*fakediscovery.FakeDiscovery)
	disc.Resources = []*metav1.APIResourceList{} // no resources

	pool := kube.NewClientPoolForTest(cfg, defaultRawConfig(), map[string]*kube.ContextClient{
		"test-ctx": {
			Dynamic:   dynClient,
			Clientset: fakeCS,
			Discovery: disc,
		},
	})

	cc, err := pool.ClientFor("test-ctx")
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}

	_, err = resolveResourceToLabelSelector(context.Background(), cc, "default", "deployment/test")
	if err == nil {
		t.Error("expected error when resolveGVR fails")
	}
	if !strings.Contains(err.Error(), "failed to resolve resource kind") {
		t.Errorf("expected resolveGVR error, got: %v", err)
	}
}

func TestResolveCronJobToLabelSelector_ListJobsError(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	cronJob := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "CronJob",
		"metadata": map[string]any{
			"name":              "fail-cj",
			"namespace":         "default",
			"uid":               "cj-uid-fail",
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"spec": map[string]any{
			"schedule": "0 0 * * *",
		},
	}}

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(cronJob)

	// Inject an error when listing Jobs.
	dynClient.PrependReactor("list", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("simulated list error")
	})

	pool := buildWritePool(cfg, dynClient, fakeCS)

	cc, err := pool.ClientFor("test-ctx")
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}

	_, err = resolveCronJobToLabelSelector(context.Background(), cc, "default", cronJob)
	if err == nil {
		t.Error("expected error when listing jobs fails")
	}
	if !strings.Contains(err.Error(), "failed to list jobs for CronJob") {
		t.Errorf("expected list error, got: %v", err)
	}
}

func TestExtractMatchLabels_NoSpec(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "test"},
	}}

	_, err := extractMatchLabels("Deployment", "test", obj)
	if err == nil {
		t.Error("expected error for missing spec")
	}
	if !strings.Contains(err.Error(), "has no spec") {
		t.Errorf("expected 'no spec' error, got: %v", err)
	}
}

func TestExtractMatchLabels_NoSelector(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "test"},
		"spec":       map[string]any{"replicas": int64(1)},
	}}

	_, err := extractMatchLabels("Deployment", "test", obj)
	if err == nil {
		t.Error("expected error for missing selector")
	}
	if !strings.Contains(err.Error(), "has no spec.selector") {
		t.Errorf("expected 'no spec.selector' error, got: %v", err)
	}
}

func TestExtractMatchLabels_NoMatchLabels(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "test"},
		"spec": map[string]any{
			"selector": map[string]any{
				"matchExpressions": []any{},
			},
		},
	}}

	_, err := extractMatchLabels("Deployment", "test", obj)
	if err == nil {
		t.Error("expected error for missing matchLabels")
	}
	if !strings.Contains(err.Error(), "has no spec.selector.matchLabels") {
		t.Errorf("expected 'no matchLabels' error, got: %v", err)
	}
}

func TestExtractMatchLabels_EmptyMatchLabels(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "test"},
		"spec": map[string]any{
			"selector": map[string]any{
				"matchLabels": map[string]any{},
			},
		},
	}}

	_, err := extractMatchLabels("Deployment", "test", obj)
	if err == nil {
		t.Error("expected error for empty matchLabels")
	}
	if !strings.Contains(err.Error(), "has no spec.selector.matchLabels") {
		t.Errorf("expected 'no matchLabels' error, got: %v", err)
	}
}
