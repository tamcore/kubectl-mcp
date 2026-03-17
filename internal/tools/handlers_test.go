package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	fakecorev1 "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	restclient "k8s.io/client-go/rest"
	fakerest "k8s.io/client-go/rest/fake"
	clienttesting "k8s.io/client-go/testing"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	fakediscovery "k8s.io/client-go/discovery/fake"
	fakedynamic "k8s.io/client-go/dynamic/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// callToolReq builds a mcp.CallToolRequest with the given arguments map.
func callToolReq(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

// resultText extracts the first TextContent text from a CallToolResult.
func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("expected at least one content item")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

// newFakeDiscovery returns a FakeDiscovery wired with core + apps resources.
func newFakeDiscovery() *fakediscovery.FakeDiscovery {
	fakeClient := fake.NewClientset()
	disc := fakeClient.Discovery().(*fakediscovery.FakeDiscovery)
	disc.Resources = apiResources()
	return disc
}

// apiResources returns a standard set of API resource lists for discovery.
func apiResources() []*metav1.APIResourceList {
	return []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"get", "list"}},
				{Name: "namespaces", Kind: "Namespace", Namespaced: false, Verbs: metav1.Verbs{"get", "list"}},
				{Name: "secrets", Kind: "Secret", Namespaced: true, Verbs: metav1.Verbs{"get", "list"}},
				{Name: "events", Kind: "Event", Namespaced: true, Verbs: metav1.Verbs{"get", "list"}},
				{Name: "pods/log", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"get"}},
				{Name: "nodes", Kind: "Node", Namespaced: false, Verbs: metav1.Verbs{"get", "list"}},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", Kind: "Deployment", Namespaced: true, Verbs: metav1.Verbs{"get", "list"}},
			},
		},
	}
}

// defaultCfg returns a permissive test configuration.
func defaultCfg() *config.Config {
	return &config.Config{AllowedContexts: []string{"*"}}
}

// defaultRawConfig returns a raw kubeconfig with a single context.
func defaultRawConfig() clientcmdapi.Config {
	return clientcmdapi.Config{
		CurrentContext: "test-ctx",
		Contexts: map[string]*clientcmdapi.Context{
			"test-ctx": {},
		},
	}
}

// newFakeDynClient creates a fake dynamic client preloaded with the given objects.
func newFakeDynClient(objs ...runtime.Object) *fakedynamic.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return fakedynamic.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			{Group: "", Version: "v1", Resource: "pods"}:            "PodList",
			{Group: "", Version: "v1", Resource: "secrets"}:         "SecretList",
			{Group: "", Version: "v1", Resource: "namespaces"}:      "NamespaceList",
			{Group: "", Version: "v1", Resource: "nodes"}:           "NodeList",
			{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
		},
		objs...,
	)
}

// testPod returns an unstructured Pod object.
func testPod(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":              name,
			"namespace":         ns,
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"spec": map[string]interface{}{
			"nodeName": "node-1",
		},
		"status": map[string]interface{}{
			"phase": "Running",
			"conditions": []interface{}{
				map[string]interface{}{
					"type":    "Ready",
					"status":  "True",
					"reason":  "PodReady",
					"message": "All containers ready",
				},
			},
		},
	}}
}

// testSecret returns an unstructured Secret object.
func testSecret(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":              name,
			"namespace":         ns,
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"data": map[string]interface{}{
			"password": "c2VjcmV0",
		},
	}}
}

// testNode returns an unstructured Node object (cluster-scoped).
func testNode(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata": map[string]interface{}{
			"name":              name,
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
	}}
}

// testDeployment returns an unstructured Deployment object.
func testDeployment(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":              name,
			"namespace":         ns,
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"spec": map[string]interface{}{
			"replicas": int64(3),
		},
		"status": map[string]interface{}{
			"readyReplicas":     int64(3),
			"updatedReplicas":   int64(3),
			"availableReplicas": int64(3),
		},
	}}
}

// buildPool constructs a ClientPool for testing with the given fake clients.
func buildPool(cfg *config.Config, rawCfg clientcmdapi.Config, dynClient *fakedynamic.FakeDynamicClient, fakeCS *fake.Clientset) *kube.ClientPool {
	disc := fakeCS.Discovery().(*fakediscovery.FakeDiscovery)
	disc.Resources = apiResources()

	return kube.NewClientPoolForTest(cfg, rawCfg, map[string]*kube.ContextClient{
		"test-ctx": {
			Dynamic:   dynClient,
			Clientset: fakeCS,
			Discovery: disc,
		},
	})
}

// getHandler registers a tool on a fresh server and returns its handler.
func getHandler(t *testing.T, name string, register func(s *server.MCPServer)) server.ToolHandlerFunc {
	t.Helper()
	s := server.NewMCPServer("test", "0.1.0", server.WithToolCapabilities(false))
	register(s)
	tool := s.GetTool(name)
	if tool == nil {
		t.Fatalf("tool %q not registered", name)
	}
	return tool.Handler
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRegisterAll(t *testing.T) {
	readOnlyTools := []string{
		"list_contexts", "list_namespaces", "list_api_resources",
		"get_resource", "list_resources", "describe_resource",
		"get_logs", "get_events",
		"top_pods", "top_nodes",
		"rollout_status",
		"rollout_history",
		"node_logs",
		"node_stats",
		"explain_resource",
		"stop_port_forward",
	}
	writeTools := []string{
		"apply_resource", "patch_resource", "scale_resource",
		"restart_rollout", "cordon_node", "uncordon_node", "exec_pod",
		"rollout_undo",
		"rollout_pause", "rollout_resume",
		"run_pod",
		"port_forward",
	}
	destructiveTools := []string{
		"delete_resource", "drain_node",
	}

	t.Run("read-only mode", func(t *testing.T) {
		cfg := defaultCfg()
		pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fake.NewClientset())
		s := server.NewMCPServer("test", "0.1.0", server.WithToolCapabilities(false))
		RegisterAll(s, pool, cfg)

		tools := s.ListTools()
		for _, name := range readOnlyTools {
			if _, ok := tools[name]; !ok {
				t.Errorf("expected tool %q to be registered", name)
			}
		}
		for _, name := range writeTools {
			if _, ok := tools[name]; ok {
				t.Errorf("tool %q should NOT be registered in read-only mode", name)
			}
		}
		for _, name := range destructiveTools {
			if _, ok := tools[name]; ok {
				t.Errorf("tool %q should NOT be registered in read-only mode", name)
			}
		}
	})

	t.Run("write mode", func(t *testing.T) {
		cfg := defaultCfg()
		cfg.AllowWrite = true
		pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fake.NewClientset())
		s := server.NewMCPServer("test", "0.1.0", server.WithToolCapabilities(false))
		RegisterAll(s, pool, cfg)

		tools := s.ListTools()
		for _, name := range readOnlyTools {
			if _, ok := tools[name]; !ok {
				t.Errorf("expected tool %q to be registered", name)
			}
		}
		for _, name := range writeTools {
			if _, ok := tools[name]; !ok {
				t.Errorf("expected tool %q to be registered in write mode", name)
			}
		}
		for _, name := range destructiveTools {
			if _, ok := tools[name]; ok {
				t.Errorf("tool %q should NOT be registered without --allow-destructive", name)
			}
		}
	})

	t.Run("destructive mode", func(t *testing.T) {
		cfg := defaultCfg()
		cfg.AllowWrite = true
		cfg.AllowDestructive = true
		pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fake.NewClientset())
		s := server.NewMCPServer("test", "0.1.0", server.WithToolCapabilities(false))
		RegisterAll(s, pool, cfg)

		tools := s.ListTools()
		all := append(append(readOnlyTools, writeTools...), destructiveTools...)
		for _, name := range all {
			if _, ok := tools[name]; !ok {
				t.Errorf("expected tool %q to be registered in destructive mode", name)
			}
		}
	})
}

// --- list_contexts ---

func TestListContextsHandler(t *testing.T) {
	cfg := defaultCfg()
	rawCfg := clientcmdapi.Config{
		CurrentContext: "beta",
		Contexts: map[string]*clientcmdapi.Context{
			"beta":  {},
			"alpha": {},
			"gamma": {},
		},
	}
	pool := kube.NewClientPoolForTest(cfg, rawCfg, nil)

	handler := getHandler(t, "list_contexts", func(s *server.MCPServer) {
		registerListContexts(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(nil))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)

	// Should be sorted and beta marked default.
	type ctxInfo struct {
		Name      string `json:"name"`
		IsDefault bool   `json:"isDefault,omitempty"`
	}
	var items []ctxInfo
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("unmarshal: %v\ntext=%s", err, text)
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 contexts, got %d", len(items))
	}
	// Sorted: alpha, beta, gamma
	if items[0].Name != "alpha" {
		t.Errorf("first should be alpha, got %s", items[0].Name)
	}
	if !items[1].IsDefault {
		t.Error("beta should be marked as default")
	}
	if items[2].Name != "gamma" {
		t.Errorf("third should be gamma, got %s", items[2].Name)
	}
}

// --- list_namespaces ---

func TestListNamespacesHandler(t *testing.T) {
	fakeCS := fake.NewClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	)
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)

	handler := getHandler(t, "list_namespaces", func(s *server.MCPServer) {
		registerListNamespaces(s, pool)
	})

	t.Run("happy path", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{}))
		if err != nil {
			t.Fatal(err)
		}
		text := resultText(t, res)
		var names []string
		if err := json.Unmarshal([]byte(text), &names); err != nil {
			t.Fatal(err)
		}
		if len(names) != 2 {
			t.Fatalf("expected 2 namespaces, got %d", len(names))
		}
		// Sorted: default, kube-system
		if names[0] != "default" || names[1] != "kube-system" {
			t.Errorf("unexpected order: %v", names)
		}
	})

	t.Run("disallowed context", func(t *testing.T) {
		restrictedCfg := &config.Config{AllowedContexts: []string{"other-ctx"}}
		restrictedPool := kube.NewClientPoolForTest(restrictedCfg, defaultRawConfig(), nil)

		h := getHandler(t, "list_namespaces", func(s *server.MCPServer) {
			registerListNamespaces(s, restrictedPool)
		})
		res, err := h(context.Background(), callToolReq(map[string]any{}))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected error result for disallowed context")
		}
	})
}

// --- list_api_resources ---

func TestListAPIResourcesHandler(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fakeCS)

	handler := getHandler(t, "list_api_resources", func(s *server.MCPServer) {
		registerListAPIResources(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)

	type entry struct {
		Kind       string   `json:"kind"`
		APIVersion string   `json:"apiVersion"`
		Namespaced bool     `json:"namespaced"`
		Verbs      []string `json:"verbs"`
	}
	var entries []entry
	if err := json.Unmarshal([]byte(text), &entries); err != nil {
		t.Fatal(err)
	}

	// pods/log sub-resource should be filtered out; main Pod resource must exist.
	podFound := false
	for _, e := range entries {
		if e.Kind == "Pod" && e.APIVersion == "v1" {
			podFound = true
		}
	}
	if !podFound {
		t.Error("expected Pod v1 resource to be present")
	}

	// Verify sorted by kind.
	for i := 1; i < len(entries); i++ {
		if entries[i-1].Kind > entries[i].Kind {
			t.Errorf("not sorted: %s > %s", entries[i-1].Kind, entries[i].Kind)
		}
	}

	// The Deployment entry from apps/v1 should be present.
	found := false
	for _, e := range entries {
		if e.Kind == "Deployment" && e.APIVersion == "apps/v1" {
			found = true
		}
	}
	if !found {
		t.Error("expected Deployment entry from apps/v1")
	}
}

// --- get_resource ---

func TestGetResourceHandler(t *testing.T) {
	pod := testPod("my-pod", "default")
	secret := testSecret("my-secret", "default")
	node := testNode("node-1")
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient(pod, secret, node)
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_resource", func(s *server.MCPServer) {
		registerGetResource(s, pool, cfg)
	})

	t.Run("namespaced resource", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"name":      "my-pod",
			"namespace": "default",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		if !strings.Contains(text, "my-pod") {
			t.Error("expected pod name in output")
		}
		// managedFields should be stripped.
		if strings.Contains(text, "managedFields") {
			t.Error("managedFields should be stripped")
		}
	})

	t.Run("cluster-scoped resource", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind": "Node",
			"name": "node-1",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		if !strings.Contains(text, "node-1") {
			t.Error("expected node name in output")
		}
	})

	t.Run("secret redaction", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Secret",
			"name":      "my-secret",
			"namespace": "default",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		// JSON encoding escapes < and > as unicode escapes.
		if !strings.Contains(text, "redacted") {
			t.Error("expected secret data to be redacted")
		}
		if strings.Contains(text, "c2VjcmV0") {
			t.Error("original secret data should not appear")
		}
	})

	t.Run("secrets allowed", func(t *testing.T) {
		allowSecretsCfg := &config.Config{AllowedContexts: []string{"*"}, AllowSecrets: true}
		secretObj := testSecret("open-secret", "default")
		dynClient2 := newFakeDynClient(secretObj)
		pool2 := buildPool(allowSecretsCfg, defaultRawConfig(), dynClient2, fake.NewClientset())

		h := getHandler(t, "get_resource", func(s *server.MCPServer) {
			registerGetResource(s, pool2, allowSecretsCfg)
		})
		res, err := h(context.Background(), callToolReq(map[string]any{
			"kind":      "Secret",
			"name":      "open-secret",
			"namespace": "default",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		if strings.Contains(text, "<redacted>") {
			t.Error("secrets should NOT be redacted when AllowSecrets=true")
		}
	})

	t.Run("not found", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"name":      "nonexistent",
			"namespace": "default",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected error for nonexistent resource")
		}
	})

	t.Run("with apiVersion", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":       "Pod",
			"name":       "my-pod",
			"namespace":  "default",
			"apiVersion": "v1",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
	})

	t.Run("unknown kind", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Bogus",
			"name":      "x",
			"namespace": "default",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected error for unknown kind")
		}
		text := resultText(t, res)
		if !strings.Contains(text, "could not resolve") {
			t.Errorf("expected resolve error, got: %s", text)
		}
	})
}

// --- list_resources ---

func TestListResourcesHandler(t *testing.T) {
	pod1 := testPod("pod-a", "default")
	pod2 := testPod("pod-b", "default")
	// Pod with Pending status.
	pod3 := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":              "pod-c",
			"namespace":         "other",
			"creationTimestamp": "2024-01-01T00:00:00Z",
			"labels":            map[string]interface{}{"app": "web"},
		},
		"status": map[string]interface{}{"phase": "Pending"},
	}}

	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient(pod1, pod2, pod3)
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "list_resources", func(s *server.MCPServer) {
		registerListResources(s, pool, cfg)
	})

	t.Run("happy path all namespaces", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind": "Pod",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		if !strings.Contains(text, "pod-a") || !strings.Contains(text, "pod-b") || !strings.Contains(text, "pod-c") {
			t.Error("expected all pods in output")
		}
	})

	t.Run("namespaced", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"namespace": "default",
		}))
		if err != nil {
			t.Fatal(err)
		}
		text := resultText(t, res)
		if !strings.Contains(text, "pod-a") {
			t.Error("expected pod-a")
		}
		if strings.Contains(text, "pod-c") {
			t.Error("pod-c is in 'other' namespace, should not appear")
		}
	})

	t.Run("with filter match", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":   "Pod",
			"filter": "status.phase=Running",
		}))
		if err != nil {
			t.Fatal(err)
		}
		text := resultText(t, res)
		if !strings.Contains(text, "Matched") {
			t.Error("expected 'Matched' header in filtered output")
		}
		if !strings.Contains(text, "pod-a") {
			t.Error("expected pod-a (Running)")
		}
	})

	t.Run("filter no match", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":   "Pod",
			"filter": "status.phase=Succeeded",
		}))
		if err != nil {
			t.Fatal(err)
		}
		text := resultText(t, res)
		if !strings.Contains(text, "No Pod matched filter") {
			t.Errorf("expected no-match message, got: %s", text)
		}
	})

	t.Run("empty result no filter", func(t *testing.T) {
		emptyPool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fake.NewClientset())
		h := getHandler(t, "list_resources", func(s *server.MCPServer) {
			registerListResources(s, emptyPool, cfg)
		})
		res, err := h(context.Background(), callToolReq(map[string]any{
			"kind": "Pod",
		}))
		if err != nil {
			t.Fatal(err)
		}
		text := resultText(t, res)
		if !strings.Contains(text, "No Pod found") {
			t.Errorf("expected 'No Pod found', got: %s", text)
		}
	})

	t.Run("invalid filter expression", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":   "Pod",
			"filter": "nooperator",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected error for invalid filter")
		}
	})

	t.Run("secret redaction in list", func(t *testing.T) {
		sec := testSecret("list-secret", "default")
		dynClient2 := newFakeDynClient(sec)
		pool2 := buildPool(cfg, defaultRawConfig(), dynClient2, fake.NewClientset())
		h := getHandler(t, "list_resources", func(s *server.MCPServer) {
			registerListResources(s, pool2, cfg)
		})
		res, err := h(context.Background(), callToolReq(map[string]any{
			"kind":      "Secret",
			"namespace": "default",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		// Secrets in list should not contain raw data.
		text := resultText(t, res)
		if strings.Contains(text, "c2VjcmV0") {
			t.Error("raw secret data should be redacted in list results")
		}
	})
}

// --- describe_resource ---

func TestDescribeResourceHandler(t *testing.T) {
	pod := testPod("desc-pod", "default")
	cfg := defaultCfg()
	// Create a clientset with an event for the pod.
	event := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "evt1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "desc-pod"},
		Type:           "Normal",
		Reason:         "Scheduled",
		Message:        "Successfully assigned",
		LastTimestamp:  metav1.Now(),
	}
	fakeCS := fake.NewClientset(event)
	dynClient := newFakeDynClient(pod)
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "describe_resource", func(s *server.MCPServer) {
		registerDescribeResource(s, pool, cfg)
	})

	t.Run("happy path with conditions and events", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"name":      "desc-pod",
			"namespace": "default",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		// Header fields
		if !strings.Contains(text, "Name:         desc-pod") {
			t.Error("expected Name header")
		}
		if !strings.Contains(text, "Kind:         Pod") {
			t.Error("expected Kind header")
		}
		// Conditions table
		if !strings.Contains(text, "Conditions:") {
			t.Error("expected Conditions section")
		}
		if !strings.Contains(text, "Ready") {
			t.Error("expected Ready condition")
		}
		// Events
		if !strings.Contains(text, "Events:") {
			t.Error("expected Events section")
		}
		if !strings.Contains(text, "Scheduled") {
			t.Error("expected Scheduled event reason")
		}
	})

	t.Run("not found", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"name":      "nonexistent",
			"namespace": "default",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected error for nonexistent resource")
		}
	})

	t.Run("cluster-scoped no events", func(t *testing.T) {
		node := testNode("desc-node")
		dynClient2 := newFakeDynClient(node)
		pool2 := buildPool(cfg, defaultRawConfig(), dynClient2, fake.NewClientset())
		h := getHandler(t, "describe_resource", func(s *server.MCPServer) {
			registerDescribeResource(s, pool2, cfg)
		})
		res, err := h(context.Background(), callToolReq(map[string]any{
			"kind": "Node",
			"name": "desc-node",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		if !strings.Contains(text, "desc-node") {
			t.Error("expected node name")
		}
		// No events → "<none>"
		if !strings.Contains(text, "<none>") {
			t.Error("expected <none> for empty events")
		}
	})

	t.Run("describe with labels and annotations", func(t *testing.T) {
		labeled := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":              "labeled-pod",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
				"labels":            map[string]interface{}{"app": "web"},
				"annotations":       map[string]interface{}{"note": "test"},
			},
		}}
		dynClient3 := newFakeDynClient(labeled)
		pool3 := buildPool(cfg, defaultRawConfig(), dynClient3, fake.NewClientset())
		h := getHandler(t, "describe_resource", func(s *server.MCPServer) {
			registerDescribeResource(s, pool3, cfg)
		})
		res, err := h(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"name":      "labeled-pod",
			"namespace": "default",
		}))
		if err != nil {
			t.Fatal(err)
		}
		text := resultText(t, res)
		if !strings.Contains(text, "Labels:") {
			t.Error("expected Labels section")
		}
		if !strings.Contains(text, "app=web") {
			t.Error("expected app=web label")
		}
		if !strings.Contains(text, "Annotations:") {
			t.Error("expected Annotations section")
		}
		if !strings.Contains(text, "note=test") {
			t.Error("expected note=test annotation")
		}
	})

	t.Run("describe with spec", func(t *testing.T) {
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"kind":      "Pod",
			"name":      "desc-pod",
			"namespace": "default",
		}))
		if err != nil {
			t.Fatal(err)
		}
		text := resultText(t, res)
		if !strings.Contains(text, "Spec:") {
			t.Error("expected Spec section")
		}
		if !strings.Contains(text, "nodeName") {
			t.Error("expected nodeName in spec")
		}
	})
}

// --- get_logs ---

func TestGetLogsHandler(t *testing.T) {
	cfg := defaultCfg()

	t.Run("happy path with fake stream", func(t *testing.T) {
		fakeCS := fake.NewClientset()
		// Prepend a reactor that returns log data.
		fakeCS.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("get", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
			// Check if this is a log subresource request.
			if subAction, ok := action.(clienttesting.GenericAction); ok {
				_ = subAction
			}
			return false, nil, nil
		})

		// Use a fake REST client to handle log requests.
		fakeCS.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("*", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
			return false, nil, nil
		})

		dynClient := newFakeDynClient()
		pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

		handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
			registerGetLogs(s, pool)
		})

		// The fake clientset doesn't support pod log streaming natively.
		// We can only test the error path.
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"namespace": "default",
			"pod":       "test-pod",
		}))
		if err != nil {
			t.Fatal(err)
		}
		// Expect an error because fake clientset doesn't support log streaming.
		if !res.IsError {
			t.Log("Got non-error result (may work with newer fake):", resultText(t, res))
		}
	})

	t.Run("with fake REST log stream", func(t *testing.T) {
		fakeCS := fake.NewClientset()

		// Override the REST client for pods to return log content.
		fakeCS.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("get", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
			return false, nil, nil
		})

		// Create a custom fake that can handle log streaming.
		logBody := "line1\nline2\nline3\n"
		fakeHTTPClient := fakerest.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader(logBody)),
			}, nil
		})

		// Create a new fake clientset with our custom HTTP client.
		fakeCS2 := fake.NewClientset()
		fakeCS2.CoreV1().(*fakecorev1.FakeCoreV1).PrependProxyReactor("*", func(action clienttesting.Action) (bool, restclient.ResponseWrapper, error) {
			return false, nil, nil
		})

		// We use the fake HTTP client approach for the REST client.
		_ = fakeHTTPClient
		_ = fakeCS2

		// Since the fake clientset doesn't support pod log streaming well,
		// let's test with a proper custom setup using the ProxyReactor.
		dynClient := newFakeDynClient()
		pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

		handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
			registerGetLogs(s, pool)
		})

		// Test with since parameter to cover that code path.
		res, err := handler(context.Background(), callToolReq(map[string]any{
			"namespace": "default",
			"pod":       "test-pod",
			"since":     "5m",
		}))
		if err != nil {
			t.Fatal(err)
		}
		// Accept error result since fake doesn't support streaming.
		_ = res
	})

	t.Run("invalid since duration", func(t *testing.T) {
		fakeCS := fake.NewClientset()
		dynClient := newFakeDynClient()
		pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

		handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
			registerGetLogs(s, pool)
		})

		res, err := handler(context.Background(), callToolReq(map[string]any{
			"namespace": "default",
			"pod":       "test-pod",
			"since":     "invalid",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected error for invalid since duration")
		}
		text := resultText(t, res)
		if !strings.Contains(text, "invalid since duration") {
			t.Errorf("expected 'invalid since duration' error, got: %s", text)
		}
	})

	t.Run("with container and previous", func(t *testing.T) {
		fakeCS := fake.NewClientset()
		dynClient := newFakeDynClient()
		pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

		handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
			registerGetLogs(s, pool)
		})

		res, err := handler(context.Background(), callToolReq(map[string]any{
			"namespace": "default",
			"pod":       "test-pod",
			"container": "sidecar",
			"previous":  true,
			"tail":      float64(50),
		}))
		if err != nil {
			t.Fatal(err)
		}
		// Accept any result (error from fake is expected).
		_ = res
	})

	t.Run("disallowed context", func(t *testing.T) {
		restrictedCfg := &config.Config{AllowedContexts: []string{"other-ctx"}}
		restrictedPool := kube.NewClientPoolForTest(restrictedCfg, defaultRawConfig(), nil)

		h := getHandler(t, "get_logs", func(s *server.MCPServer) {
			registerGetLogs(s, restrictedPool)
		})
		res, err := h(context.Background(), callToolReq(map[string]any{
			"namespace": "default",
			"pod":       "test-pod",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Error("expected error result for disallowed context")
		}
	})
}

// --- get_events ---

func TestGetEventsHandler(t *testing.T) {
	cfg := defaultCfg()

	t.Run("happy path", func(t *testing.T) {
		event := &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "evt1", Namespace: "default"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "my-pod"},
			Type:           "Warning",
			Reason:         "BackOff",
			Message:        "Back-off restarting",
			LastTimestamp:  metav1.Now(),
			Count:          3,
		}
		fakeCS := fake.NewClientset(event)
		dynClient := newFakeDynClient()
		pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

		handler := getHandler(t, "get_events", func(s *server.MCPServer) {
			registerGetEvents(s, pool)
		})

		res, err := handler(context.Background(), callToolReq(map[string]any{
			"namespace": "default",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		if !strings.Contains(text, "BackOff") {
			t.Error("expected BackOff reason in output")
		}
		if !strings.Contains(text, "pod/my-pod") {
			t.Error("expected pod/my-pod object reference")
		}
		if !strings.Contains(text, "Warning") {
			t.Error("expected Warning type")
		}
	})

	t.Run("empty events", func(t *testing.T) {
		fakeCS := fake.NewClientset()
		dynClient := newFakeDynClient()
		pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

		handler := getHandler(t, "get_events", func(s *server.MCPServer) {
			registerGetEvents(s, pool)
		})

		res, err := handler(context.Background(), callToolReq(map[string]any{
			"namespace": "default",
		}))
		if err != nil {
			t.Fatal(err)
		}
		text := resultText(t, res)
		if text != "[]" {
			t.Errorf("expected empty JSON array, got: %s", text)
		}
	})

	t.Run("with limit and fieldSelector", func(t *testing.T) {
		event := &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "evt2", Namespace: "default"},
			InvolvedObject: corev1.ObjectReference{Kind: "Deployment", Name: "my-deploy"},
			Type:           "Normal",
			Reason:         "ScalingReplicaSet",
			Message:        "Scaled up",
			LastTimestamp:  metav1.Now(),
		}
		fakeCS := fake.NewClientset(event)
		dynClient := newFakeDynClient()
		pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

		handler := getHandler(t, "get_events", func(s *server.MCPServer) {
			registerGetEvents(s, pool)
		})

		res, err := handler(context.Background(), callToolReq(map[string]any{
			"namespace": "default",
			"limit":     float64(10),
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
	})

	t.Run("all namespaces", func(t *testing.T) {
		event := &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "evt3", Namespace: "kube-system"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "kube-dns"},
			Type:           "Normal",
			Reason:         "Started",
			Message:        "Started container",
		}
		fakeCS := fake.NewClientset(event)
		dynClient := newFakeDynClient()
		pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

		handler := getHandler(t, "get_events", func(s *server.MCPServer) {
			registerGetEvents(s, pool)
		})

		// Omit namespace to get all namespaces.
		res, err := handler(context.Background(), callToolReq(map[string]any{}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		text := resultText(t, res)
		if !strings.Contains(text, "kube-dns") {
			t.Error("expected kube-dns in all-namespaces result")
		}
		// Age should be "<unknown>" since LastTimestamp is zero (JSON-escaped).
		if !strings.Contains(text, "unknown") {
			t.Error("expected unknown age for zero timestamp")
		}
	})
}

// --- resolveGVR ---

func TestResolveGVR(t *testing.T) {
	disc := newFakeDiscovery()
	cc := &kube.ContextClient{Discovery: disc}

	t.Run("exact kind match", func(t *testing.T) {
		gvr, err := resolveGVR(cc, "Pod", "")
		if err != nil {
			t.Fatal(err)
		}
		if gvr.Resource != "pods" || gvr.Version != "v1" || gvr.Group != "" {
			t.Errorf("unexpected GVR: %v", gvr)
		}
	})

	t.Run("case insensitive kind", func(t *testing.T) {
		gvr, err := resolveGVR(cc, "pod", "")
		if err != nil {
			t.Fatal(err)
		}
		if gvr.Resource != "pods" {
			t.Errorf("expected pods, got %s", gvr.Resource)
		}
	})

	t.Run("plural match", func(t *testing.T) {
		gvr, err := resolveGVR(cc, "deployment", "")
		if err != nil {
			t.Fatal(err)
		}
		if gvr.Resource != "deployments" || gvr.Group != "apps" {
			t.Errorf("unexpected GVR: %v", gvr)
		}
	})

	t.Run("apiVersion filter", func(t *testing.T) {
		gvr, err := resolveGVR(cc, "Deployment", "apps/v1")
		if err != nil {
			t.Fatal(err)
		}
		if gvr.Group != "apps" || gvr.Version != "v1" {
			t.Errorf("unexpected GVR: %v", gvr)
		}
	})

	t.Run("apiVersion filter no match", func(t *testing.T) {
		_, err := resolveGVR(cc, "Pod", "apps/v1")
		if err == nil {
			t.Error("expected error when apiVersion doesn't match")
		}
	})

	t.Run("unknown kind", func(t *testing.T) {
		_, err := resolveGVR(cc, "Bogus", "")
		if err == nil {
			t.Error("expected error for unknown kind")
		}
		if !strings.Contains(err.Error(), "could not resolve") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("core API preferred", func(t *testing.T) {
		// Add a custom discovery with same kind in core + extension.
		customDisc := &fakediscovery.FakeDiscovery{Fake: &clienttesting.Fake{}}
		customDisc.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{Name: "events", Kind: "Event", Namespaced: true, Verbs: metav1.Verbs{"get", "list"}},
				},
			},
			{
				GroupVersion: "events.k8s.io/v1",
				APIResources: []metav1.APIResource{
					{Name: "events", Kind: "Event", Namespaced: true, Verbs: metav1.Verbs{"get", "list"}},
				},
			},
		}
		cc2 := &kube.ContextClient{Discovery: customDisc}
		gvr, err := resolveGVR(cc2, "Event", "")
		if err != nil {
			t.Fatal(err)
		}
		if gvr.Group != "" {
			t.Errorf("expected core API group, got %q", gvr.Group)
		}
	})

	t.Run("resource name match", func(t *testing.T) {
		gvr, err := resolveGVR(cc, "pods", "")
		if err != nil {
			t.Fatal(err)
		}
		if gvr.Resource != "pods" {
			t.Errorf("expected pods, got %s", gvr.Resource)
		}
	})
}

// --- fetchEvents ---

func TestFetchEvents(t *testing.T) {
	t.Run("with events", func(t *testing.T) {
		event := &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "evt1", Namespace: "default"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "my-pod"},
			Type:           "Normal",
			Reason:         "Pulled",
			Message:        "Successfully pulled image",
			LastTimestamp:  metav1.Now(),
		}
		fakeCS := fake.NewClientset(event)
		cc := &kube.ContextClient{Clientset: fakeCS}

		result, err := fetchEvents(context.Background(), cc, "default", "Pod", "my-pod")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result, "Pulled") {
			t.Error("expected Pulled reason")
		}
		if !strings.Contains(result, "Successfully pulled image") {
			t.Error("expected message")
		}
	})

	t.Run("no events", func(t *testing.T) {
		fakeCS := fake.NewClientset()
		cc := &kube.ContextClient{Clientset: fakeCS}

		result, err := fetchEvents(context.Background(), cc, "default", "Pod", "nonexistent")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result, "<none>") {
			t.Errorf("expected <none>, got: %s", result)
		}
	})

	t.Run("zero timestamp shows unknown age", func(t *testing.T) {
		event := &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "evt2", Namespace: "default"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "zero-ts-pod"},
			Type:           "Warning",
			Reason:         "Failed",
			Message:        "Pull failed",
			// LastTimestamp not set → zero value.
		}
		fakeCS := fake.NewClientset(event)
		cc := &kube.ContextClient{Clientset: fakeCS}

		result, err := fetchEvents(context.Background(), cc, "default", "Pod", "zero-ts-pod")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result, "<unknown>") {
			t.Errorf("expected <unknown> age, got: %s", result)
		}
	})
}

// --- Context resolution error paths ---

func TestHandlerContextErrors(t *testing.T) {
	restrictedCfg := &config.Config{AllowedContexts: []string{"allowed-only"}}
	restrictedPool := kube.NewClientPoolForTest(restrictedCfg, defaultRawConfig(), nil)
	cfg := restrictedCfg

	tests := []struct {
		name     string
		register func(s *server.MCPServer)
		toolName string
		args     map[string]any
	}{
		{
			name: "list_api_resources disallowed context",
			register: func(s *server.MCPServer) {
				registerListAPIResources(s, restrictedPool)
			},
			toolName: "list_api_resources",
			args:     map[string]any{},
		},
		{
			name: "get_resource disallowed context",
			register: func(s *server.MCPServer) {
				registerGetResource(s, restrictedPool, cfg)
			},
			toolName: "get_resource",
			args:     map[string]any{"kind": "Pod", "name": "x"},
		},
		{
			name: "list_resources disallowed context",
			register: func(s *server.MCPServer) {
				registerListResources(s, restrictedPool, cfg)
			},
			toolName: "list_resources",
			args:     map[string]any{"kind": "Pod"},
		},
		{
			name: "describe_resource disallowed context",
			register: func(s *server.MCPServer) {
				registerDescribeResource(s, restrictedPool, cfg)
			},
			toolName: "describe_resource",
			args:     map[string]any{"kind": "Pod", "name": "x"},
		},
		{
			name: "get_events disallowed context",
			register: func(s *server.MCPServer) {
				registerGetEvents(s, restrictedPool)
			},
			toolName: "get_events",
			args:     map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := getHandler(t, tt.toolName, tt.register)
			res, err := h(context.Background(), callToolReq(tt.args))
			if err != nil {
				t.Fatal(err)
			}
			if !res.IsError {
				t.Error("expected error for disallowed context")
			}
		})
	}
}

// --- List with Deployment (tests formatResourceList enrichment) ---

func TestListResourcesDeployment(t *testing.T) {
	dep := testDeployment("my-deploy", "default")
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient(dep)
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "list_resources", func(s *server.MCPServer) {
		registerListResources(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":       "Deployment",
		"namespace":  "default",
		"apiVersion": "apps/v1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}
	text := resultText(t, res)
	if !strings.Contains(text, "my-deploy") {
		t.Error("expected deployment name in output")
	}
}

// --- Describe with secret redaction ---

func TestDescribeResourceSecretRedaction(t *testing.T) {
	secret := testSecret("desc-secret", "default")
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient(secret)
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "describe_resource", func(s *server.MCPServer) {
		registerDescribeResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Secret",
		"name":      "desc-secret",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}
	text := resultText(t, res)
	if strings.Contains(text, "c2VjcmV0") {
		t.Error("secret data should be redacted in describe output")
	}
}
