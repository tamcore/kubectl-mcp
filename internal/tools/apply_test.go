package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"

	fakediscovery "k8s.io/client-go/discovery/fake"
	fakedynamic "k8s.io/client-go/dynamic/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func TestApplyResource_JSONManifest(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"test-pod","namespace":"default"},"spec":{"containers":[{"name":"app","image":"nginx"}]}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Applied Pod/test-pod") {
		t.Errorf("expected apply confirmation, got: %s", text)
	}
}

func TestApplyResource_YAMLManifest(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	manifest := `apiVersion: v1
kind: Pod
metadata:
  name: yaml-pod
  namespace: default
spec:
  containers:
  - name: app
    image: nginx`

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Applied Pod/yaml-pod") {
		t.Errorf("expected apply confirmation, got: %s", text)
	}
}

func TestApplyResource_ClusterScoped(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"test-ns"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Applied Namespace/test-ns") {
		t.Errorf("expected apply confirmation, got: %s", text)
	}
}

func TestApplyResource_InvalidManifest(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": "not valid yaml or json {{{",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "failed to parse manifest") {
		t.Errorf("expected parse error, got: %s", text)
	}
}

func TestApplyResource_MissingKind(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","metadata":{"name":"test"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "must include 'kind'") {
		t.Errorf("expected kind error, got: %s", text)
	}
}

func TestApplyResource_MissingName(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","kind":"Pod"}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "must include 'metadata.name'") {
		t.Errorf("expected name error, got: %s", text)
	}
}

func TestApplyResource_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other-ctx"}, AllowWrite: true}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"test-pod","namespace":"default"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not allowed error, got: %s", text)
	}
}

func TestApplyResource_SecretRedaction(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowSecrets = false

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"my-secret","namespace":"default"},"data":{"password":"c2VjcmV0"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Applied Secret/my-secret") {
		t.Errorf("expected apply confirmation, got: %s", text)
	}
	if strings.Contains(text, "c2VjcmV0") {
		t.Error("secret data should be redacted in response")
	}
}

func TestParseManifest_JSON(t *testing.T) {
	obj, err := parseManifest(`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"test"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if obj.GetKind() != "Pod" {
		t.Errorf("kind = %q, want Pod", obj.GetKind())
	}
	if obj.GetName() != "test" {
		t.Errorf("name = %q, want test", obj.GetName())
	}
}

func TestParseManifest_YAML(t *testing.T) {
	obj, err := parseManifest("apiVersion: v1\nkind: Pod\nmetadata:\n  name: test")
	if err != nil {
		t.Fatal(err)
	}
	if obj.GetKind() != "Pod" {
		t.Errorf("kind = %q, want Pod", obj.GetKind())
	}
}

func TestParseManifest_InvalidYAML(t *testing.T) {
	_, err := parseManifest("{{{invalid")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestParseManifest_MissingAPIVersion(t *testing.T) {
	_, err := parseManifest(`{"kind":"Pod","metadata":{"name":"test"}}`)
	if err == nil {
		t.Fatal("expected error for missing apiVersion")
	}
	if !strings.Contains(err.Error(), "apiVersion") {
		t.Errorf("expected apiVersion error, got: %v", err)
	}
}

// newWriteFakeDynClient creates a fake dynamic client that supports write operations.
func newWriteFakeDynClient(objs ...runtime.Object) *fakedynamic.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return fakedynamic.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			{Group: "", Version: "v1", Resource: "pods"}:                       "PodList",
			{Group: "", Version: "v1", Resource: "secrets"}:                    "SecretList",
			{Group: "", Version: "v1", Resource: "namespaces"}:                 "NamespaceList",
			{Group: "", Version: "v1", Resource: "nodes"}:                      "NodeList",
			{Group: "apps", Version: "v1", Resource: "deployments"}:            "DeploymentList",
			{Group: "apps", Version: "v1", Resource: "statefulsets"}:           "StatefulSetList",
			{Group: "apps", Version: "v1", Resource: "daemonsets"}:             "DaemonSetList",
			{Group: "apps", Version: "v1", Resource: "replicasets"}:            "ReplicaSetList",
			{Group: "", Version: "v1", Resource: "services"}:                   "ServiceList",
			{Group: "", Version: "v1", Resource: "configmaps"}:                 "ConfigMapList",
			{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}: "PodDisruptionBudgetList",
		},
		objs...,
	)
}

// writeAPIResources returns API resources including apps/* and scale subresources.
func writeAPIResources() []*metav1.APIResourceList {
	return []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "create", "update", "patch", "delete"}},
				{Name: "namespaces", Kind: "Namespace", Namespaced: false, Verbs: metav1.Verbs{"get", "list", "create", "update", "patch", "delete"}},
				{Name: "secrets", Kind: "Secret", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "create", "update", "patch", "delete"}},
				{Name: "events", Kind: "Event", Namespaced: true, Verbs: metav1.Verbs{"get", "list"}},
				{Name: "nodes", Kind: "Node", Namespaced: false, Verbs: metav1.Verbs{"get", "list", "update", "patch"}},
				{Name: "pods/log", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"get"}},
				{Name: "services", Kind: "Service", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "create", "update", "patch", "delete"}},
				{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "create", "update", "patch", "delete"}},
				{Name: "pods/eviction", Kind: "Eviction", Namespaced: true, Verbs: metav1.Verbs{"create"}},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", Kind: "Deployment", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "create", "update", "patch", "delete"}},
				{Name: "deployments/scale", Kind: "Scale", Namespaced: true, Verbs: metav1.Verbs{"get", "update", "patch"}},
				{Name: "statefulsets", Kind: "StatefulSet", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "create", "update", "patch", "delete"}},
				{Name: "statefulsets/scale", Kind: "Scale", Namespaced: true, Verbs: metav1.Verbs{"get", "update", "patch"}},
				{Name: "replicasets", Kind: "ReplicaSet", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "create", "update", "patch", "delete"}},
				{Name: "replicasets/scale", Kind: "Scale", Namespaced: true, Verbs: metav1.Verbs{"get", "update", "patch"}},
				{Name: "daemonsets", Kind: "DaemonSet", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "create", "update", "patch", "delete"}},
			},
		},
		{
			GroupVersion: "policy/v1",
			APIResources: []metav1.APIResource{
				{Name: "poddisruptionbudgets", Kind: "PodDisruptionBudget", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "create", "update", "patch", "delete"}},
			},
		},
	}
}

// buildWritePool constructs a ClientPool for write operation testing.
func buildWritePool(cfg *config.Config, dynClient *fakedynamic.FakeDynamicClient, fakeCS *fake.Clientset) *kube.ClientPool {
	disc := fakeCS.Discovery().(*fakediscovery.FakeDiscovery)
	disc.Resources = writeAPIResources()

	return kube.NewClientPoolForTest(cfg, defaultRawConfig(), map[string]*kube.ContextClient{
		"test-ctx": {
			Dynamic:   dynClient,
			Clientset: fakeCS,
			Discovery: disc,
		},
	})
}

// testDaemonSet returns an unstructured DaemonSet object.
func testDaemonSet(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "DaemonSet",
		"metadata": map[string]interface{}{
			"name":              name,
			"namespace":         ns,
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{},
			},
		},
	}}
}
