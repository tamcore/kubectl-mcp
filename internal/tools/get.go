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

// resolveGVR resolves a kind (and optional apiVersion) to a GroupVersionResource
// using the discovery client.
func resolveGVR(cc *kube.ContextClient, kind, apiVersion string) (schema.GroupVersionResource, error) {
	_, apiLists, err := cc.Discovery.ServerGroupsAndResources()
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("discovery error: %w", err)
	}

	lowerKind := strings.ToLower(kind)

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
			if strings.EqualFold(r.Kind, kind) || strings.ToLower(r.Name) == lowerKind || matchesPlural(r.Name, lowerKind) {
				return schema.GroupVersionResource{
					Group:    gv.Group,
					Version:  gv.Version,
					Resource: r.Name,
				}, nil
			}
		}
	}

	return schema.GroupVersionResource{}, fmt.Errorf("could not resolve resource for kind %q (apiVersion=%q)", kind, apiVersion)
}

func matchesPlural(resourceName, input string) bool {
	// Handle common plural/singular mismatches.
	return strings.ToLower(resourceName) == input+"s" ||
		strings.ToLower(resourceName) == input+"es" ||
		strings.TrimSuffix(strings.ToLower(resourceName), "s") == input
}
