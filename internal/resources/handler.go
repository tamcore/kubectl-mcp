package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// readResource handles a MCP resource read request for a k8s:// URI.
// It parses the URI, fetches the resource, applies redaction and cleanup,
// and returns JSON text resource contents.
func readResource(
	ctx context.Context,
	req mcp.ReadResourceRequest,
	pool *kube.ClientPool,
	cfg *config.Config,
) ([]mcp.ResourceContents, error) {
	parsed, err := ParseK8sURI(req.Params.URI)
	if err != nil {
		return nil, fmt.Errorf("invalid resource URI: %w", err)
	}

	ctxName, err := pool.ResolveContext(parsed.Context)
	if err != nil {
		return nil, fmt.Errorf("context resolution failed: %w", err)
	}

	cc, err := pool.ClientFor(ctxName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %q: %w", ctxName, err)
	}

	gvr := schema.GroupVersionResource{
		Group:    parsed.Group,
		Version:  parsed.Version,
		Resource: parsed.Resource,
	}

	obj, err := fetchResource(ctx, cc, gvr, parsed.Namespace, parsed.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s/%s: %w", parsed.Resource, parsed.Name, err)
	}

	if !cfg.AllowSecrets {
		kube.RedactSecrets(obj)
	}

	stripNoise(obj)

	out, err := json.MarshalIndent(obj.Object, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resource: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(out),
		},
	}, nil
}

// fetchResource retrieves a single resource from the cluster.
func fetchResource(
	ctx context.Context,
	cc *kube.ContextClient,
	gvr schema.GroupVersionResource,
	namespace, name string,
) (*unstructured.Unstructured, error) {
	if namespace != "" {
		return cc.Dynamic.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	}
	return cc.Dynamic.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
}

// stripNoise removes managedFields and last-applied-configuration annotation
// to reduce payload size.
func stripNoise(obj *unstructured.Unstructured) {
	meta, ok := obj.Object["metadata"].(map[string]interface{})
	if !ok {
		return
	}

	delete(meta, "managedFields")

	annotations, ok := meta["annotations"].(map[string]interface{})
	if ok {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		if len(annotations) == 0 {
			delete(meta, "annotations")
		}
	}
}
