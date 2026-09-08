package resources

import (
	"fmt"
	"strings"

	"github.com/yosida95/uritemplate/v3"
)

// URI templates for the k8s:// scheme, also registered as MCP resource templates.
const (
	namespacedURITemplate = "k8s://{context}/namespaces/{namespace}/{group}/{version}/{resource}/{name}"
	clusterURITemplate    = "k8s://{context}/{group}/{version}/{resource}/{name}"
)

var (
	namespacedMatcher = uritemplate.MustNew(namespacedURITemplate)
	clusterMatcher    = uritemplate.MustNew(clusterURITemplate)
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
	if vals := namespacedMatcher.Match(uri); vals != nil {
		return buildParsedURI(vals, uri, true)
	}
	if vals := clusterMatcher.Match(uri); vals != nil {
		// "namespaces" in group position means a malformed namespaced URI,
		// not a cluster-scoped resource in an API group of that name.
		if vals.Get("group").String() != "namespaces" {
			return buildParsedURI(vals, uri, false)
		}
	}
	return ParsedURI{}, fmt.Errorf("URI %q does not match %q or %q", uri, namespacedURITemplate, clusterURITemplate)
}

func buildParsedURI(vals uritemplate.Values, uri string, namespaced bool) (ParsedURI, error) {
	parsed := ParsedURI{
		Context:  vals.Get("context").String(),
		Group:    normalizeGroup(vals.Get("group").String()),
		Version:  vals.Get("version").String(),
		Resource: vals.Get("resource").String(),
		Name:     vals.Get("name").String(),
	}
	if namespaced {
		parsed.Namespace = vals.Get("namespace").String()
	}

	// uritemplate expansion accepts empty values; every segment must be present.
	for _, seg := range []string{parsed.Context, vals.Get("group").String(), parsed.Version, parsed.Resource, parsed.Name} {
		if strings.TrimSpace(seg) == "" {
			return ParsedURI{}, fmt.Errorf("empty path segment in %q", uri)
		}
	}
	if namespaced && strings.TrimSpace(parsed.Namespace) == "" {
		return ParsedURI{}, fmt.Errorf("empty namespace segment in %q", uri)
	}

	return parsed, nil
}

// normalizeGroup maps "core" to "" for the core API group.
func normalizeGroup(group string) string {
	if group == "core" {
		return ""
	}
	return group
}
