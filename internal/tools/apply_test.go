package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

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

func TestApplyResource_ValidateStrict(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	var capturedFieldValidation string
	dynClient.PrependReactor("create", "configmaps", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if ua, ok := action.(interface{ GetCreateOptions() metav1.CreateOptions }); ok {
			capturedFieldValidation = ua.GetCreateOptions().FieldValidation
		}
		return false, nil, nil
	})

	manifest := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"strict-cm","namespace":"default"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
		"validate": "strict",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, res))
	}
	if capturedFieldValidation != "Strict" {
		t.Errorf("expected fieldValidation=Strict, got %q", capturedFieldValidation)
	}
}

func TestApplyResource_ValidateWarn(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	var capturedFieldValidation string
	dynClient.PrependReactor("create", "configmaps", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if ua, ok := action.(interface{ GetCreateOptions() metav1.CreateOptions }); ok {
			capturedFieldValidation = ua.GetCreateOptions().FieldValidation
		}
		return false, nil, nil
	})

	manifest := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"warn-cm","namespace":"default"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
		"validate": "warn",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, res))
	}
	if capturedFieldValidation != "Warn" {
		t.Errorf("expected fieldValidation=Warn, got %q", capturedFieldValidation)
	}
}

func TestApplyResource_ValidateIgnore(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	var capturedFieldValidation string
	dynClient.PrependReactor("create", "configmaps", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if ua, ok := action.(interface{ GetCreateOptions() metav1.CreateOptions }); ok {
			capturedFieldValidation = ua.GetCreateOptions().FieldValidation
		}
		return false, nil, nil
	})

	manifest := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"ignore-cm","namespace":"default"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
		"validate": "ignore",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, res))
	}
	if capturedFieldValidation != "Ignore" {
		t.Errorf("expected fieldValidation=Ignore, got %q", capturedFieldValidation)
	}
}

func TestApplyResource_ValidateNoneAliasesIgnore(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	var capturedFieldValidation string
	dynClient.PrependReactor("create", "configmaps", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if ua, ok := action.(interface{ GetCreateOptions() metav1.CreateOptions }); ok {
			capturedFieldValidation = ua.GetCreateOptions().FieldValidation
		}
		return false, nil, nil
	})

	manifest := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"none-cm","namespace":"default"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
		"validate": "none",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, res))
	}
	if capturedFieldValidation != "Ignore" {
		t.Errorf("expected fieldValidation=Ignore for validate=none, got %q", capturedFieldValidation)
	}
}

func TestApplyResource_ValidateDefault(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	var capturedFieldValidation string
	dynClient.PrependReactor("create", "configmaps", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if ua, ok := action.(interface{ GetCreateOptions() metav1.CreateOptions }); ok {
			capturedFieldValidation = ua.GetCreateOptions().FieldValidation
		}
		return false, nil, nil
	})

	// No validate param — should default to Strict.
	manifest := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"default-cm","namespace":"default"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, res))
	}
	if capturedFieldValidation != "Strict" {
		t.Errorf("expected fieldValidation=Strict (default), got %q", capturedFieldValidation)
	}
}

func TestApplyResource_ValidateInvalid(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"bad-cm","namespace":"default"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
		"validate": "bogus",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error for invalid validate value")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "invalid validate") {
		t.Errorf("expected 'invalid validate' error, got: %s", text)
	}
}

func TestApplyResource_ValidateWithDryRun(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	var capturedFieldValidation string
	var capturedDryRun []string
	dynClient.PrependReactor("create", "configmaps", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if ua, ok := action.(interface{ GetCreateOptions() metav1.CreateOptions }); ok {
			opts := ua.GetCreateOptions()
			capturedFieldValidation = opts.FieldValidation
			capturedDryRun = opts.DryRun
		}
		return false, nil, nil
	})

	manifest := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"dryrun-warn-cm","namespace":"default"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
		"validate": "warn",
		"dryRun":   true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, res))
	}
	if capturedFieldValidation != "Warn" {
		t.Errorf("expected fieldValidation=Warn, got %q", capturedFieldValidation)
	}
	if len(capturedDryRun) == 0 || capturedDryRun[0] != "All" {
		t.Errorf("expected dryRun=[All], got %v", capturedDryRun)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "DRY RUN") {
		t.Errorf("expected DRY RUN prefix, got: %s", text)
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
			{Group: "batch", Version: "v1", Resource: "jobs"}:                  "JobList",
			{Group: "batch", Version: "v1", Resource: "cronjobs"}:              "CronJobList",
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
		{
			GroupVersion: "batch/v1",
			APIResources: []metav1.APIResource{
				{Name: "jobs", Kind: "Job", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "create", "update", "patch", "delete"}},
				{Name: "cronjobs", Kind: "CronJob", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "create", "update", "patch", "delete"}},
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

func TestApplyResource_SafetyDelaySkippedOnDryRun(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.SafetyDelayWrite = 2 * time.Second

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm-dry","namespace":"default"}}`
	start := time.Now()
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
		"dryRun":   true,
	}))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed >= 500*time.Millisecond {
		t.Fatalf("dryRun should skip safety delay, elapsed: %v", elapsed)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "DRY RUN") {
		t.Errorf("expected DRY RUN in response, got: %s", text)
	}
}

func TestApplyResource_SafetyDelayApplied(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.SafetyDelayWrite = 200 * time.Millisecond

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "apply_resource", func(s *server.MCPServer) {
		registerApplyResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm-timed","namespace":"default"}}`
	start := time.Now()
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed < 200*time.Millisecond {
		t.Fatalf("expected safety delay >= 200ms, elapsed: %v", elapsed)
	}
	_ = res
}

func TestScaleResource_SafetyDelayApplied(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.SafetyDelayWrite = 200 * time.Millisecond

	dep := testDeployment("scale-dep", "default")
	fakeCS := newScaleFakeClientset(dep)
	dynClient := newWriteFakeDynClient(dep)
	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "scale_resource", func(s *server.MCPServer) {
		registerScaleResource(s, pool, cfg)
	})

	start := time.Now()
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "scale-dep",
		"namespace": "default",
		"replicas":  float64(2),
	}))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed < 200*time.Millisecond {
		t.Fatalf("expected safety delay >= 200ms, elapsed: %v", elapsed)
	}
	_ = res
}

// testDaemonSet returns an unstructured DaemonSet object.
func testDaemonSet(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "DaemonSet",
		"metadata": map[string]any{
			"name":              name,
			"namespace":         ns,
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{},
			},
		},
	}}
}
