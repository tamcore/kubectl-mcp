package resources

import (
	"fmt"
	"strings"
)

// ParsedURI holds the components extracted from a k8s:// URI.
type ParsedURI struct {
	Context   string
	Group     string
	Version   string
	Resource  string
	Namespace string // empty for cluster-scoped resources
	Name      string
}

// ParseK8sURI parses a k8s:// URI into its components.
//
// Two patterns are supported:
//   - Namespaced:     k8s://{context}/namespaces/{namespace}/{group}/{version}/{resource}/{name}
//   - Cluster-scoped: k8s://{context}/{group}/{version}/{resource}/{name}
//
// The special group name "core" maps to the empty string "" for the core API group.
func ParseK8sURI(uri string) (ParsedURI, error) {
	const scheme = "k8s://"
	if !strings.HasPrefix(uri, scheme) {
		return ParsedURI{}, fmt.Errorf("URI must start with %s, got %q", scheme, uri)
	}

	path := strings.TrimPrefix(uri, scheme)
	segments := strings.Split(path, "/")

	if len(segments) < 5 {
		return ParsedURI{}, fmt.Errorf("URI too short: expected at least 5 path segments, got %d in %q", len(segments), uri)
	}

	for i, seg := range segments {
		if seg == "" {
			return ParsedURI{}, fmt.Errorf("empty path segment at position %d in %q", i, uri)
		}
	}

	context := segments[0]

	// Check if this is namespaced (has "namespaces" as second segment).
	if segments[1] == "namespaces" {
		return parseNamespacedURI(context, segments, uri)
	}
	return parseClusterScopedURI(context, segments, uri)
}

func parseNamespacedURI(context string, segments []string, uri string) (ParsedURI, error) {
	// k8s://{context}/namespaces/{namespace}/{group}/{version}/{resource}/{name}
	// segments: [context, "namespaces", namespace, group, version, resource, name]
	if len(segments) != 7 {
		return ParsedURI{}, fmt.Errorf(
			"namespaced URI requires exactly 7 path segments (context/namespaces/namespace/group/version/resource/name), got %d in %q",
			len(segments), uri,
		)
	}

	return ParsedURI{
		Context:   context,
		Namespace: segments[2],
		Group:     normalizeGroup(segments[3]),
		Version:   segments[4],
		Resource:  segments[5],
		Name:      segments[6],
	}, nil
}

func parseClusterScopedURI(context string, segments []string, uri string) (ParsedURI, error) {
	// k8s://{context}/{group}/{version}/{resource}/{name}
	// segments: [context, group, version, resource, name]
	if len(segments) != 5 {
		return ParsedURI{}, fmt.Errorf(
			"cluster-scoped URI requires exactly 5 path segments (context/group/version/resource/name), got %d in %q",
			len(segments), uri,
		)
	}

	return ParsedURI{
		Context:  context,
		Group:    normalizeGroup(segments[1]),
		Version:  segments[2],
		Resource: segments[3],
		Name:     segments[4],
	}, nil
}

// normalizeGroup maps "core" to "" for the core API group.
func normalizeGroup(group string) string {
	if group == "core" {
		return ""
	}
	return group
}
