package tools

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// supportedResourceKinds lists the resource kinds that can be resolved to a label selector.
var supportedResourceKinds = map[string]bool{
	"deployment":  true,
	"job":         true,
	"statefulset": true,
	"replicaset":  true,
	"daemonset":   true,
}

// parseResourceRef parses a "kind/name" string into kind and name.
// Returns an error if the format is invalid.
func parseResourceRef(resource string) (string, string, error) {
	parts := strings.SplitN(resource, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid resource format %q: expected 'kind/name' (e.g. 'deployment/nginx')", resource)
	}
	return parts[0], parts[1], nil
}

// resolveResourceToLabelSelector resolves a resource reference to a label selector
// by fetching the resource and extracting spec.selector.matchLabels.
func resolveResourceToLabelSelector(ctx context.Context, cc *kube.ContextClient, namespace, resource string) (string, error) {
	kind, name, err := parseResourceRef(resource)
	if err != nil {
		return "", err
	}

	lowerKind := strings.ToLower(kind)
	// Resolve short names.
	if fullKind, ok := resolveShortName(lowerKind); ok {
		lowerKind = strings.ToLower(fullKind)
		kind = fullKind
	}

	if !supportedResourceKinds[lowerKind] {
		return "", fmt.Errorf("resource kind %q is not supported for log resolution (supported: Deployment, Job, StatefulSet, ReplicaSet, DaemonSet)", kind)
	}

	gvr, err := resolveGVR(cc, kind, "")
	if err != nil {
		return "", fmt.Errorf("failed to resolve resource kind %q: %w", kind, err)
	}

	obj, err := cc.Dynamic.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get %s/%s: %w", kind, name, err)
	}

	// Extract spec.selector.matchLabels.
	spec, ok := obj.Object["spec"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("%s/%s has no spec", kind, name)
	}

	selector, ok := spec["selector"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("%s/%s has no spec.selector", kind, name)
	}

	matchLabels, ok := selector["matchLabels"].(map[string]interface{})
	if !ok || len(matchLabels) == 0 {
		return "", fmt.Errorf("%s/%s has no spec.selector.matchLabels", kind, name)
	}

	// Build label selector string.
	var parts []string
	for k, v := range matchLabels {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}

	return strings.Join(parts, ","), nil
}
