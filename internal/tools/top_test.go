package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	fakedynamic "k8s.io/client-go/dynamic/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

func testPodMetrics(name, ns, cpu, memory string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "metrics.k8s.io/v1beta1",
		"kind":       "PodMetrics",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		"containers": []interface{}{
			map[string]interface{}{
				"name": "main",
				"usage": map[string]interface{}{
					"cpu":    cpu,
					"memory": memory,
				},
			},
		},
	}}
}

func testPodMetricsMultiContainer(name, ns string, containers []map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "metrics.k8s.io/v1beta1",
		"kind":       "PodMetrics",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		"containers": func() []interface{} {
			out := make([]interface{}, len(containers))
			for i, c := range containers {
				out[i] = c
			}
			return out
		}(),
	}}
}

func testNodeMetrics(name, cpu, memory string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "metrics.k8s.io/v1beta1",
		"kind":       "NodeMetrics",
		"metadata": map[string]interface{}{
			"name": name,
		},
		"usage": map[string]interface{}{
			"cpu":    cpu,
			"memory": memory,
		},
	}}
}

func testNodeWithAllocatable(name, cpu, memory string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata": map[string]interface{}{
			"name":              name,
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"status": map[string]interface{}{
			"allocatable": map[string]interface{}{
				"cpu":    cpu,
				"memory": memory,
			},
		},
	}}
}

// newMetricsFakeDynClient creates a fake dynamic client that includes the
// metrics.k8s.io GVRs alongside core resources. Metrics objects must be
// added via the returned client's tracker because the fake client cannot
// infer the GVR from the object's apiVersion/kind for custom resources.
func newMetricsFakeDynClient(objs ...runtime.Object) *fakedynamic.FakeDynamicClient {
	// Separate metrics objects from core objects.
	var coreObjs []runtime.Object
	var podMetricsObjs []*unstructured.Unstructured
	var nodeMetricsObjs []*unstructured.Unstructured

	for _, o := range objs {
		u, ok := o.(*unstructured.Unstructured)
		if ok && u.GetAPIVersion() == "metrics.k8s.io/v1beta1" {
			switch u.GetKind() {
			case "PodMetrics":
				podMetricsObjs = append(podMetricsObjs, u)
				continue
			case "NodeMetrics":
				nodeMetricsObjs = append(nodeMetricsObjs, u)
				continue
			}
		}
		coreObjs = append(coreObjs, o)
	}

	scheme := runtime.NewScheme()
	client := fakedynamic.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			{Group: "", Version: "v1", Resource: "pods"}:                     "PodList",
			{Group: "", Version: "v1", Resource: "nodes"}:                    "NodeList",
			{Group: "", Version: "v1", Resource: "namespaces"}:               "NamespaceList",
			{Group: "", Version: "v1", Resource: "secrets"}:                  "SecretList",
			{Group: "apps", Version: "v1", Resource: "deployments"}:          "DeploymentList",
			{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}:  "PodMetricsList",
			{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}: "NodeMetricsList",
		},
		coreObjs...,
	)

	// Add metrics objects with explicit GVR so the tracker indexes them correctly.
	pmGVR := schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
	for _, o := range podMetricsObjs {
		_ = client.Tracker().Create(pmGVR, o, o.GetNamespace())
	}
	nmGVR := schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}
	for _, o := range nodeMetricsObjs {
		_ = client.Tracker().Create(nmGVR, o, "")
	}

	return client
}

// ---------------------------------------------------------------------------
// Formatting helper tests
// ---------------------------------------------------------------------------

func TestFormatCPU(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"250m", "250m"},
		{"0", "0m"},
		{"1", "1000m"},
		{"1500m", "1500m"},
		{"100m", "100m"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := formatCPU(tt.input)
			if got != tt.want {
				t.Errorf("formatCPU(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatMemory(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"134217728", "128Mi"},   // 128 * 1024 * 1024
		{"1073741824", "1024Mi"}, // 1 Gi
		{"65536Ki", "64Mi"},
		{"128Mi", "128Mi"},
		{"0", "0Mi"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := formatMemory(tt.input)
			if got != tt.want {
				t.Errorf("formatMemory(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatPercent(t *testing.T) {
	tests := []struct {
		used, alloc int64
		want        string
	}{
		{500, 4000, "12%"},
		{0, 1000, "0%"},
		{1000, 1000, "100%"},
		{100, 0, "N/A"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatPercent(tt.used, tt.alloc)
			if got != tt.want {
				t.Errorf("formatPercent(%d, %d) = %q, want %q", tt.used, tt.alloc, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// top_pods tests
// ---------------------------------------------------------------------------

func TestTopPods_HappyPath(t *testing.T) {
	cfg := defaultCfg()
	dynClient := newMetricsFakeDynClient(
		testPodMetrics("pod-1", "default", "250m", "128Mi"),
		testPodMetrics("pod-2", "kube-system", "100m", "64Mi"),
	)
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "top_pods", func(s *server.MCPServer) {
		registerTopPods(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	// Should be valid JSON array.
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON array, got: %s", text)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	// Verify fields exist.
	for _, item := range items {
		for _, key := range []string{"name", "namespace", "cpu", "memory"} {
			if _, ok := item[key]; !ok {
				t.Errorf("missing key %q in item %v", key, item)
			}
		}
	}
}

func TestTopPods_NamespaceFilter(t *testing.T) {
	cfg := defaultCfg()
	dynClient := newMetricsFakeDynClient(
		testPodMetrics("pod-1", "default", "250m", "128Mi"),
		testPodMetrics("pod-2", "kube-system", "100m", "64Mi"),
	)
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "top_pods", func(s *server.MCPServer) {
		registerTopPods(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON, got: %s", text)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (namespace filter), got %d: %s", len(items), text)
	}
	if items[0]["name"] != "pod-1" {
		t.Errorf("expected pod-1, got %v", items[0]["name"])
	}
}

func TestTopPods_NameFilter(t *testing.T) {
	cfg := defaultCfg()
	dynClient := newMetricsFakeDynClient(
		testPodMetrics("pod-1", "default", "250m", "128Mi"),
		testPodMetrics("pod-2", "default", "100m", "64Mi"),
	)
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "top_pods", func(s *server.MCPServer) {
		registerTopPods(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "pod-1",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON, got: %s", text)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0]["name"] != "pod-1" {
		t.Errorf("expected pod-1, got %v", items[0]["name"])
	}
}

func TestTopPods_ContainersBreakdown(t *testing.T) {
	cfg := defaultCfg()
	dynClient := newMetricsFakeDynClient(
		testPodMetricsMultiContainer("pod-1", "default", []map[string]interface{}{
			{"name": "app", "usage": map[string]interface{}{"cpu": "200m", "memory": "64Mi"}},
			{"name": "sidecar", "usage": map[string]interface{}{"cpu": "50m", "memory": "32Mi"}},
		}),
	)
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "top_pods", func(s *server.MCPServer) {
		registerTopPods(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"containers": true,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON array, got: %s", text)
	}
	// Should have 2 rows: one per container.
	if len(items) != 2 {
		t.Fatalf("expected 2 container rows, got %d: %s", len(items), text)
	}
	// Each row must have "container" field.
	for _, item := range items {
		if _, ok := item["container"]; !ok {
			t.Errorf("expected 'container' field in item %v", item)
		}
		if item["name"] != "pod-1" {
			t.Errorf("expected pod name 'pod-1', got %v", item["name"])
		}
	}
	// Verify individual container values.
	if items[0]["container"] != "app" {
		t.Errorf("expected first container 'app', got %v", items[0]["container"])
	}
	if items[0]["cpu"] != "200m" {
		t.Errorf("expected 200m CPU for app, got %v", items[0]["cpu"])
	}
	if items[1]["container"] != "sidecar" {
		t.Errorf("expected second container 'sidecar', got %v", items[1]["container"])
	}
	if items[1]["cpu"] != "50m" {
		t.Errorf("expected 50m CPU for sidecar, got %v", items[1]["cpu"])
	}
}

func TestTopPods_ContainersFalseAggregates(t *testing.T) {
	cfg := defaultCfg()
	dynClient := newMetricsFakeDynClient(
		testPodMetricsMultiContainer("pod-1", "default", []map[string]interface{}{
			{"name": "app", "usage": map[string]interface{}{"cpu": "200m", "memory": "64Mi"}},
			{"name": "sidecar", "usage": map[string]interface{}{"cpu": "50m", "memory": "32Mi"}},
		}),
	)
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "top_pods", func(s *server.MCPServer) {
		registerTopPods(s, pool)
	})

	// containers=false (default) should aggregate.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON, got: %s", text)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 aggregated row, got %d", len(items))
	}
	// Should NOT have "container" field.
	if _, ok := items[0]["container"]; ok {
		t.Errorf("should not have 'container' field when containers=false")
	}
	// CPU should be 200m + 50m = 250m.
	if items[0]["cpu"] != "250m" {
		t.Errorf("expected 250m aggregated CPU, got %v", items[0]["cpu"])
	}
}

func TestTopPods_MultiContainer(t *testing.T) {
	cfg := defaultCfg()
	dynClient := newMetricsFakeDynClient(
		testPodMetricsMultiContainer("pod-1", "default", []map[string]interface{}{
			{"name": "app", "usage": map[string]interface{}{"cpu": "200m", "memory": "64Mi"}},
			{"name": "sidecar", "usage": map[string]interface{}{"cpu": "50m", "memory": "32Mi"}},
		}),
	)
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "top_pods", func(s *server.MCPServer) {
		registerTopPods(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON, got: %s", text)
	}
	// CPU should be 200m + 50m = 250m, Memory 64Mi + 32Mi = 96Mi.
	if items[0]["cpu"] != "250m" {
		t.Errorf("expected 250m CPU, got %v", items[0]["cpu"])
	}
	if items[0]["memory"] != "96Mi" {
		t.Errorf("expected 96Mi memory, got %v", items[0]["memory"])
	}
}

func TestTopPods_Empty(t *testing.T) {
	cfg := defaultCfg()
	dynClient := newMetricsFakeDynClient() // no metrics
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "top_pods", func(s *server.MCPServer) {
		registerTopPods(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if text != "[]" {
		t.Errorf("expected empty array, got: %s", text)
	}
}

func TestTopPods_MetricsServerNotAvailable(t *testing.T) {
	cfg := defaultCfg()
	dynClient := newMetricsFakeDynClient()
	// Simulate metrics-server not installed by returning a 404 on the metrics GVR.
	dynClient.PrependReactor("list", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetResource().Group == "metrics.k8s.io" {
			return true, nil, errors.NewNotFound(
				schema.GroupResource{Group: "metrics.k8s.io", Resource: "pods"},
				"the server could not find the requested resource",
			)
		}
		return false, nil, nil
	})
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "top_pods", func(s *server.MCPServer) {
		registerTopPods(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "metrics-server") {
		t.Errorf("expected metrics-server error, got: %s", text)
	}
}

func TestTopPods_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}}
	dynClient := newMetricsFakeDynClient()
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "top_pods", func(s *server.MCPServer) {
		registerTopPods(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not-allowed error, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// top_nodes tests
// ---------------------------------------------------------------------------

func TestTopNodes_HappyPath(t *testing.T) {
	cfg := defaultCfg()
	dynClient := newMetricsFakeDynClient(
		testNodeMetrics("node-1", "500m", "2Gi"),
		testNodeWithAllocatable("node-1", "4000m", "8Gi"),
	)
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "top_nodes", func(s *server.MCPServer) {
		registerTopNodes(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON array, got: %s", text)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	for _, key := range []string{"name", "cpuUsed", "cpuAllocatable", "cpuPercent", "memoryUsed", "memoryAllocatable", "memoryPercent"} {
		if _, ok := item[key]; !ok {
			t.Errorf("missing key %q in item %v", key, item)
		}
	}
	if item["cpuPercent"] != "12%" {
		t.Errorf("expected cpuPercent 12%%, got %v", item["cpuPercent"])
	}
}

func TestTopNodes_NameFilter(t *testing.T) {
	cfg := defaultCfg()
	dynClient := newMetricsFakeDynClient(
		testNodeMetrics("node-1", "500m", "2Gi"),
		testNodeMetrics("node-2", "100m", "1Gi"),
		testNodeWithAllocatable("node-1", "4000m", "8Gi"),
		testNodeWithAllocatable("node-2", "4000m", "8Gi"),
	)
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "top_nodes", func(s *server.MCPServer) {
		registerTopNodes(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"name": "node-1",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON, got: %s", text)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0]["name"] != "node-1" {
		t.Errorf("expected node-1, got %v", items[0]["name"])
	}
}

func TestTopNodes_MissingAllocatable(t *testing.T) {
	cfg := defaultCfg()
	// Node metrics exist but no Node object with allocatable.
	dynClient := newMetricsFakeDynClient(
		testNodeMetrics("node-1", "500m", "2Gi"),
	)
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "top_nodes", func(s *server.MCPServer) {
		registerTopNodes(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("expected JSON, got: %s", text)
	}
	if items[0]["cpuPercent"] != "N/A" {
		t.Errorf("expected cpuPercent N/A when no allocatable, got %v", items[0]["cpuPercent"])
	}
	if items[0]["memoryPercent"] != "N/A" {
		t.Errorf("expected memoryPercent N/A, got %v", items[0]["memoryPercent"])
	}
}

func TestTopNodes_Empty(t *testing.T) {
	cfg := defaultCfg()
	dynClient := newMetricsFakeDynClient()
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "top_nodes", func(s *server.MCPServer) {
		registerTopNodes(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if text != "[]" {
		t.Errorf("expected empty array, got: %s", text)
	}
}

func TestTopNodes_MetricsServerNotAvailable(t *testing.T) {
	cfg := defaultCfg()
	dynClient := newMetricsFakeDynClient()
	dynClient.PrependReactor("list", "nodes", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetResource().Group == "metrics.k8s.io" {
			return true, nil, errors.NewNotFound(
				schema.GroupResource{Group: "metrics.k8s.io", Resource: "nodes"},
				"the server could not find the requested resource",
			)
		}
		return false, nil, nil
	})
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "top_nodes", func(s *server.MCPServer) {
		registerTopNodes(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "metrics-server") {
		t.Errorf("expected metrics-server error, got: %s", text)
	}
}
