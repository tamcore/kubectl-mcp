package resources

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	fakedynamic "k8s.io/client-go/dynamic/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newFakeDynClient(objs ...runtime.Object) *fakedynamic.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return fakedynamic.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			{Group: "", Version: "v1", Resource: "configmaps"}: "ConfigMapList",
			{Group: "", Version: "v1", Resource: "secrets"}:    "SecretList",
			{Group: "", Version: "v1", Resource: "nodes"}:      "NodeList",
		},
		objs...,
	)
}

func testPool(dynClient *fakedynamic.FakeDynamicClient) *kube.ClientPool {
	cfg := &config.Config{AllowedContexts: []string{"*"}}
	rawConfig := clientcmdapi.Config{
		CurrentContext: "test-ctx",
		Contexts: map[string]*clientcmdapi.Context{
			"test-ctx": {},
		},
	}
	return kube.NewClientPoolForTest(cfg, rawConfig, map[string]*kube.ContextClient{
		"test-ctx": {Dynamic: dynClient},
	})
}

func testConfigMap(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		"data": map[string]interface{}{
			"key": "value",
		},
	}}
}

func testSecret(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		"data": map[string]interface{}{
			"password": "c2VjcmV0",
		},
	}}
}

func testNode(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata": map[string]interface{}{
			"name": name,
		},
	}}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestReadResource_ConfigMap(t *testing.T) {
	dyn := newFakeDynClient(testConfigMap("my-cm", "default"))
	pool := testPool(dyn)
	cfg := &config.Config{AllowedContexts: []string{"*"}}

	req := mcp.ReadResourceRequest{}
	req.Params.URI = "k8s://test-ctx/namespaces/default/core/v1/configmaps/my-cm"

	contents, err := readResource(context.Background(), req, pool, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}

	tc, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", contents[0])
	}
	if tc.MIMEType != "application/json" {
		t.Errorf("expected MIME type application/json, got %s", tc.MIMEType)
	}
	if !strings.Contains(tc.Text, `"key": "value"`) {
		t.Errorf("expected configmap data in response, got: %s", tc.Text)
	}
}

func TestReadResource_SecretRedacted(t *testing.T) {
	dyn := newFakeDynClient(testSecret("my-secret", "default"))
	pool := testPool(dyn)
	cfg := &config.Config{
		AllowedContexts: []string{"*"},
		AllowSecrets:    false,
	}

	req := mcp.ReadResourceRequest{}
	req.Params.URI = "k8s://test-ctx/namespaces/default/core/v1/secrets/my-secret"

	contents, err := readResource(context.Background(), req, pool, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tc := contents[0].(mcp.TextResourceContents)
	if strings.Contains(tc.Text, "c2VjcmV0") {
		t.Error("secret data should be redacted but was not")
	}
	// json.Marshal escapes < and > as \u003c and \u003e
	if !strings.Contains(tc.Text, "redacted") {
		t.Error("expected redacted placeholder in secret data")
	}
}

func TestReadResource_SecretAllowed(t *testing.T) {
	dyn := newFakeDynClient(testSecret("my-secret", "default"))
	pool := testPool(dyn)
	cfg := &config.Config{
		AllowedContexts: []string{"*"},
		AllowSecrets:    true,
	}

	req := mcp.ReadResourceRequest{}
	req.Params.URI = "k8s://test-ctx/namespaces/default/core/v1/secrets/my-secret"

	contents, err := readResource(context.Background(), req, pool, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tc := contents[0].(mcp.TextResourceContents)
	if !strings.Contains(tc.Text, "c2VjcmV0") {
		t.Error("secret data should be visible when AllowSecrets is true")
	}
}

func TestReadResource_ClusterScoped(t *testing.T) {
	dyn := newFakeDynClient(testNode("worker-1"))
	pool := testPool(dyn)
	cfg := &config.Config{AllowedContexts: []string{"*"}}

	req := mcp.ReadResourceRequest{}
	req.Params.URI = "k8s://test-ctx/core/v1/nodes/worker-1"

	contents, err := readResource(context.Background(), req, pool, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tc := contents[0].(mcp.TextResourceContents)
	if !strings.Contains(tc.Text, "worker-1") {
		t.Errorf("expected node name in response, got: %s", tc.Text)
	}
}

func TestReadResource_InvalidURI(t *testing.T) {
	dyn := newFakeDynClient()
	pool := testPool(dyn)
	cfg := &config.Config{AllowedContexts: []string{"*"}}

	req := mcp.ReadResourceRequest{}
	req.Params.URI = "http://bad/uri"

	_, err := readResource(context.Background(), req, pool, cfg)
	if err == nil {
		t.Fatal("expected error for invalid URI")
	}
}

func TestReadResource_NotFound(t *testing.T) {
	dyn := newFakeDynClient()
	pool := testPool(dyn)
	cfg := &config.Config{AllowedContexts: []string{"*"}}

	req := mcp.ReadResourceRequest{}
	req.Params.URI = "k8s://test-ctx/namespaces/default/core/v1/configmaps/nonexistent"

	_, err := readResource(context.Background(), req, pool, cfg)
	if err == nil {
		t.Fatal("expected error for nonexistent resource")
	}
}

func TestStripNoise(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":          "test",
			"managedFields": []interface{}{"something"},
			"annotations": map[string]interface{}{
				"kubectl.kubernetes.io/last-applied-configuration": "{}",
				"app.kubernetes.io/name":                           "test",
			},
		},
	}}

	stripNoise(obj)

	meta := obj.Object["metadata"].(map[string]interface{})
	if _, ok := meta["managedFields"]; ok {
		t.Error("managedFields should have been stripped")
	}
	annotations := meta["annotations"].(map[string]interface{})
	if _, ok := annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		t.Error("last-applied-configuration should have been stripped")
	}
	if _, ok := annotations["app.kubernetes.io/name"]; !ok {
		t.Error("other annotations should be preserved")
	}
}

func TestStripNoise_EmptyAnnotations(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "test",
			"annotations": map[string]interface{}{
				"kubectl.kubernetes.io/last-applied-configuration": "{}",
			},
		},
	}}

	stripNoise(obj)

	meta := obj.Object["metadata"].(map[string]interface{})
	if _, ok := meta["annotations"]; ok {
		t.Error("empty annotations map should have been removed")
	}
}
