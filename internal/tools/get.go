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
	"sigs.k8s.io/yaml"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerGetResource(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	tool := mcp.NewTool("get_resource",
		mcp.WithDescription("Get a single Kubernetes resource by kind and name, returned as YAML"),
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

		out, err := yaml.Marshal(obj.Object)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal resource: %v", err)), nil
		}

		return mcp.NewToolResultText(string(out)), nil
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
func resolveGVR(cc *kube.ContextClient, kind, apiVersion string) (schema.GroupVersionResource, error) {
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
