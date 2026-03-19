package tools

import (
	"context"
	"strings"
	"testing"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func TestCreateResource_NewResource(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "create_resource", func(s *server.MCPServer) {
		registerCreateResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"new-cm","namespace":"default"},"data":{"key":"value"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("expected success, got error: %s", text)
	}
	if !strings.Contains(text, "Created ConfigMap/new-cm") {
		t.Errorf("expected create confirmation, got: %s", text)
	}
}

func TestCreateResource_Conflict(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	dynClient.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewAlreadyExists(
			schema.GroupResource{Group: "", Resource: "pods"}, "existing-pod",
		)
	})

	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)
	handler := getHandler(t, "create_resource", func(s *server.MCPServer) {
		registerCreateResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"existing-pod","namespace":"default"},"spec":{"containers":[{"name":"app","image":"nginx"}]}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !res.IsError {
		t.Fatalf("expected conflict error, got success: %s", text)
	}
	if !strings.Contains(text, "already exists") {
		t.Errorf("expected 'already exists' error, got: %s", text)
	}
	if !strings.Contains(text, "Pod/existing-pod") {
		t.Errorf("expected resource name in error, got: %s", text)
	}
}

func TestCreateResource_DryRun(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "create_resource", func(s *server.MCPServer) {
		registerCreateResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"dry-run-cm","namespace":"default"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
		"dryRun":   true,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("expected success, got error: %s", text)
	}
	if !strings.Contains(text, "DRY RUN") {
		t.Errorf("expected dry run prefix, got: %s", text)
	}
}

func TestCreateResource_InvalidManifest(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)
	handler := getHandler(t, "create_resource", func(s *server.MCPServer) {
		registerCreateResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": "not valid yaml or json {{{",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	if !res.IsError {
		t.Fatalf("expected error for invalid manifest, got: %s", text)
	}
	if !strings.Contains(text, "failed to parse manifest") {
		t.Errorf("expected parse error, got: %s", text)
	}
}

func TestCreateResource_MultiDocYAML(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "create_resource", func(s *server.MCPServer) {
		registerCreateResource(s, pool, cfg)
	})

	manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-one
  namespace: default
data:
  key: val1
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-two
  namespace: default
data:
  key: val2`

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("expected success for multi-doc YAML, got error: %s", text)
	}
	if !strings.Contains(text, "Created ConfigMap/cm-one") {
		t.Errorf("expected cm-one in result, got: %s", text)
	}
	if !strings.Contains(text, "Created ConfigMap/cm-two") {
		t.Errorf("expected cm-two in result, got: %s", text)
	}
}

func TestCreateResource_MultiDocConflict(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	dynClient.PrependReactor("create", "configmaps", func(action clienttesting.Action) (bool, runtime.Object, error) {
		createAction, ok := action.(clienttesting.CreateAction)
		if !ok {
			return false, nil, nil
		}
		obj := createAction.GetObject().(*unstructured.Unstructured)
		if obj.GetName() == "cm-first" {
			return true, nil, k8serrors.NewAlreadyExists(
				schema.GroupResource{Group: "", Resource: "configmaps"}, "cm-first",
			)
		}
		return false, nil, nil
	})

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "create_resource", func(s *server.MCPServer) {
		registerCreateResource(s, pool, cfg)
	})

	manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-first
  namespace: default
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-second
  namespace: default`

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !res.IsError {
		t.Fatalf("expected conflict error for multi-doc, got success: %s", text)
	}
	if !strings.Contains(text, "already exists") {
		t.Errorf("expected 'already exists' error, got: %s", text)
	}
	// cm-second should NOT appear since we stopped on cm-first conflict.
	if strings.Contains(text, "cm-second") {
		t.Errorf("should have stopped at cm-first conflict, but cm-second appears: %s", text)
	}
}

func TestCreateResource_ClusterScoped(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)
	handler := getHandler(t, "create_resource", func(s *server.MCPServer) {
		registerCreateResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"new-ns"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("expected success for cluster-scoped resource, got: %s", text)
	}
	if !strings.Contains(text, "Created Namespace/new-ns") {
		t.Errorf("expected create confirmation, got: %s", text)
	}
}

func TestCreateResource_SecretRedaction(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowSecrets = false

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)
	handler := getHandler(t, "create_resource", func(s *server.MCPServer) {
		registerCreateResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"my-secret","namespace":"default"},"data":{"password":"c2VjcmV0"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("expected success, got error: %s", text)
	}
	if !strings.Contains(text, "Created Secret/my-secret") {
		t.Errorf("expected create confirmation, got: %s", text)
	}
	if strings.Contains(text, "c2VjcmV0") {
		t.Error("secret data should be redacted in response")
	}
}

func TestCreateResource_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other-ctx"}, AllowWrite: true}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)
	handler := getHandler(t, "create_resource", func(s *server.MCPServer) {
		registerCreateResource(s, pool, cfg)
	})

	manifest := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test-cm","namespace":"default"}}`
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"manifest": manifest,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	if !res.IsError {
		t.Fatalf("expected not-allowed error, got: %s", text)
	}
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not allowed error, got: %s", text)
	}
}

func TestCreateResource_MissingManifest(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)
	handler := getHandler(t, "create_resource", func(s *server.MCPServer) {
		registerCreateResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, res)
	if !res.IsError {
		t.Fatalf("expected error for missing manifest, got: %s", text)
	}
}

// --- parseManifests unit tests ---

func TestParseManifests_SingleDoc(t *testing.T) {
	objs, err := parseManifests(`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"test"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(objs))
	}
	if objs[0].GetKind() != "Pod" {
		t.Errorf("kind = %q, want Pod", objs[0].GetKind())
	}
}

func TestParseManifests_MultiDoc(t *testing.T) {
	manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: cm1
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm2`

	objs, err := parseManifests(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(objs))
	}
	if objs[0].GetName() != "cm1" {
		t.Errorf("first doc name = %q, want cm1", objs[0].GetName())
	}
	if objs[1].GetName() != "cm2" {
		t.Errorf("second doc name = %q, want cm2", objs[1].GetName())
	}
}

func TestParseManifests_EmptyDocsSkipped(t *testing.T) {
	manifest := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: only-doc
---
`
	objs, err := parseManifests(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 doc (empty docs skipped), got %d", len(objs))
	}
	if objs[0].GetName() != "only-doc" {
		t.Errorf("name = %q, want only-doc", objs[0].GetName())
	}
}

func TestParseManifests_InvalidDoc(t *testing.T) {
	_, err := parseManifests("{{{invalid")
	if err == nil {
		t.Fatal("expected error for invalid manifest")
	}
}

func TestParseManifests_EmptyInput(t *testing.T) {
	_, err := parseManifests("   ")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}
