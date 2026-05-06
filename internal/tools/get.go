package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerGetResource(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	tool := mcp.NewTool("get_resource",
		mcp.WithDescription("Get a single Kubernetes resource by kind and name, returned as JSON"),
		mcp.WithRawOutputSchema(rawK8sObjectSchema),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("apiVersion",
			mcp.Description("API version (e.g. v1, apps/v1). If omitted, the server will try to discover it."),
		),
		mcp.WithString("kind",
			mcp.Required(),
			mcp.Description("Resource kind (e.g. Pod, Deployment, Service)"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Resource name"),
		),
		mcp.WithString("namespace",
			mcp.Description("Namespace (required for namespaced resources)"),
		),
		mcp.WithString("include_annotations",
			mcp.Description("Comma-separated glob patterns for annotation keys to include (e.g. 'app.kubernetes.io/*'). If set, only matching annotations are returned."),
		),
		mcp.WithString("exclude_annotations",
			mcp.Description("Comma-separated glob patterns for annotation keys to exclude (e.g. 'kubectl.kubernetes.io/*'). "+
				"kubectl.kubernetes.io/last-applied-configuration is always excluded."),
		),
		mcp.WithString("format",
			mcp.Description("Output format: 'full' (default, JSON with noisy metadata stripped), 'summary' (compact key fields), 'yaml' (YAML with noisy metadata stripped)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctxName, err := pool.ResolveContext(req.GetString("context", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		cc, err := pool.ClientFor(ctxName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get client: %v", err)), nil
		}

		kind, _ := req.RequireString("kind")
		name, _ := req.RequireString("name")
		namespace := req.GetString("namespace", "")
		apiVersion := req.GetString("apiVersion", "")

		gvr, err := resolveGVR(cc, kind, apiVersion)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		var obj *unstructured.Unstructured
		if namespace != "" {
			obj, err = cc.Dynamic.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		} else {
			obj, err = cc.Dynamic.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get %s/%s: %v", kind, name, err)), nil
		}

		if !cfg.AllowSecrets {
			kube.RedactSecrets(obj)
		}
		filterObjAnnotations(obj, req)

		format := req.GetString("format", "full")

		return formatGetResult(obj, format)
	})
}

// gvrCandidate is a candidate GVR match during kind resolution.
type gvrCandidate struct {
	gvr     schema.GroupVersionResource
	exact   bool // exact kind match (case-insensitive)
	coreAPI bool // belongs to core API group ("")
}

// resolveGVR resolves a kind (and optional apiVersion) to a GroupVersionResource
// using the discovery client. Prefers core API group and exact kind matches.
// Supports kubectl short names (e.g. "deploy", "svc") and suggests corrections
// for typos via fuzzy matching.
func resolveGVR(cc *kube.ContextClient, kind, apiVersion string) (schema.GroupVersionResource, error) {
	// Try kubectl short name first.
	if fullKind, ok := resolveShortName(kind); ok {
		kind = fullKind
	}

	_, apiLists, err := cc.Discovery.ServerGroupsAndResources()
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("discovery error: %w", err)
	}

	lowerKind := strings.ToLower(kind)

	var candidates []gvrCandidate

	for _, list := range apiLists {
		if apiVersion != "" && list.GroupVersion != apiVersion {
			continue
		}
		gv, parseErr := schema.ParseGroupVersion(list.GroupVersion)
		if parseErr != nil {
			continue
		}
		for _, r := range list.APIResources {
			if strings.Contains(r.Name, "/") {
				continue
			}
			exact := strings.EqualFold(r.Kind, kind)
			nameMatch := strings.ToLower(r.Name) == lowerKind
			pluralMatch := matchesPlural(r.Name, lowerKind)

			if exact || nameMatch || pluralMatch {
				candidates = append(candidates, gvrCandidate{
					gvr: schema.GroupVersionResource{
						Group:    gv.Group,
						Version:  gv.Version,
						Resource: r.Name,
					},
					exact:   exact,
					coreAPI: gv.Group == "",
				})
			}
		}
	}

	if len(candidates) == 0 {
		// Collect all known kinds for fuzzy suggestion.
		var knownKinds []string
		for _, list := range apiLists {
			for _, r := range list.APIResources {
				if !strings.Contains(r.Name, "/") {
					knownKinds = append(knownKinds, r.Kind)
				}
			}
		}
		if suggestion := suggestKind(kind, knownKinds); suggestion != "" {
			return schema.GroupVersionResource{}, fmt.Errorf("could not resolve resource for kind %q (did you mean %q?)", kind, suggestion)
		}
		return schema.GroupVersionResource{}, fmt.Errorf("could not resolve resource for kind %q (apiVersion=%q)", kind, apiVersion)
	}

	// Pick best candidate: prefer exact+core > exact > core > first.
	best := candidates[0]
	for _, c := range candidates[1:] {
		if betterGVRCandidate(c, best) {
			best = c
		}
	}
	return best.gvr, nil
}

func betterGVRCandidate(a, b gvrCandidate) bool {
	// Exact match beats non-exact.
	if a.exact != b.exact {
		return a.exact
	}
	// Core API group beats extensions.
	if a.coreAPI != b.coreAPI {
		return a.coreAPI
	}
	return false
}

func matchesPlural(resourceName, input string) bool {
	// Handle common plural/singular mismatches.
	return strings.ToLower(resourceName) == input+"s" ||
		strings.ToLower(resourceName) == input+"es" ||
		strings.TrimSuffix(strings.ToLower(resourceName), "s") == input
}
