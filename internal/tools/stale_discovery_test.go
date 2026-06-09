package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// TestResolveGVRToleratesPartialDiscovery verifies that resolveGVR succeeds when
// discovery returns ErrGroupDiscoveryFailed alongside partial results (e.g. stale
// metrics-server), instead of failing entirely.
func TestResolveGVRToleratesPartialDiscovery(t *testing.T) {
	disc := &partialDiscovery{FakeDiscovery: newFakeDiscovery()}
	cc := &kube.ContextClient{Discovery: disc}

	gvr, err := resolveGVR(cc, "Pod", "")
	if err != nil {
		t.Fatalf("resolveGVR returned error: %v", err)
	}
	if gvr.Resource != "pods" {
		t.Errorf("expected resource %q, got %q", "pods", gvr.Resource)
	}
	if gvr.Version != "v1" {
		t.Errorf("expected version %q, got %q", "v1", gvr.Version)
	}
}

// TestListAPIResourcesToleratesPartialDiscovery verifies that the list_api_resources
// tool returns results from healthy API groups when discovery returns
// ErrGroupDiscoveryFailed.
func TestListAPIResourcesToleratesPartialDiscovery(t *testing.T) {
	pool := buildPoolPartialDiscovery(defaultCfg(), defaultRawConfig(),
		newFakeDynClient(), fake.NewClientset())

	handler := getHandler(t, "list_api_resources", func(s *server.MCPServer) {
		registerListAPIResources(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"context": "test-ctx",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, res))
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Pod") {
		t.Errorf("expected Pod in result, got: %s", text)
	}
}

// TestFindAPIResourceToleratesPartialDiscovery verifies that findAPIResource
// succeeds when discovery returns ErrGroupDiscoveryFailed alongside partial results.
func TestFindAPIResourceToleratesPartialDiscovery(t *testing.T) {
	disc := &partialDiscovery{FakeDiscovery: newFakeDiscovery()}
	cc := &kube.ContextClient{Discovery: disc}

	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	_, _, err := findAPIResource(cc, gvr)
	if err != nil {
		t.Fatalf("findAPIResource returned error: %v", err)
	}
}
