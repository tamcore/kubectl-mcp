package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// explainResult holds the discovery-based explanation for a resource.
type explainResult struct {
	Kind         string   `json:"kind"`
	APIVersion   string   `json:"apiVersion"`
	Resource     string   `json:"resource"`
	Namespaced   bool     `json:"namespaced"`
	Verbs        []string `json:"verbs"`
	ShortNames   []string `json:"shortNames,omitempty"`
	FieldPath    string   `json:"fieldPath,omitempty"`
	Description  string   `json:"description"`
	SubResources []string `json:"subResources,omitempty"`
}

func registerExplainResource(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("explain_resource",
		mcp.WithDescription("Explain a Kubernetes resource kind. Returns metadata, verbs, and scope from the discovery API. "+
			"Supports dotted paths like 'Deployment.spec.replicas'."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("resource",
			mcp.Required(),
			mcp.Description("Resource kind or dotted path (e.g. 'Pod', 'Deployment', 'Deployment.spec.replicas')"),
		),
		mcp.WithString("apiVersion",
			mcp.Description("API version (e.g. v1, apps/v1). If omitted, the server will try to discover it."),
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

		resourceArg, _ := req.RequireString("resource")
		apiVersion := req.GetString("apiVersion", "")

		// Parse dotted path: "Deployment.spec.replicas" -> kind="Deployment", fieldPath="spec.replicas"
		kind, fieldPath := parseResourcePath(resourceArg)

		// Resolve to GVR and find the API resource details.
		gvr, err := resolveGVR(cc, kind, apiVersion)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		apiRes, gv, err := findAPIResource(cc, gvr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Find sub-resources.
		subResources := findSubResources(cc, gvr)

		result := explainResult{
			Kind:         apiRes.Kind,
			APIVersion:   gv,
			Resource:     apiRes.Name,
			Namespaced:   apiRes.Namespaced,
			Verbs:        apiRes.Verbs,
			ShortNames:   apiRes.ShortNames,
			FieldPath:    fieldPath,
			Description:  buildDescription(apiRes, gv, fieldPath),
			SubResources: subResources,
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(out)), nil
	})
}

// parseResourcePath splits a dotted resource path into kind and field path.
// "Deployment.spec.replicas" -> ("Deployment", "spec.replicas")
// "Pod" -> ("Pod", "")
func parseResourcePath(s string) (string, string) {
	idx := strings.IndexByte(s, '.')
	if idx < 0 {
		return s, ""
	}
	return s[:idx], s[idx+1:]
}

// findAPIResource looks up the metav1.APIResource for the given GVR.
func findAPIResource(cc *kube.ContextClient, gvr schema.GroupVersionResource) (metav1.APIResource, string, error) {
	_, apiLists, err := cc.Discovery.ServerGroupsAndResources()
	if err != nil {
		return metav1.APIResource{}, "", fmt.Errorf("discovery error: %w", err)
	}

	for _, list := range apiLists {
		gv, parseErr := schema.ParseGroupVersion(list.GroupVersion)
		if parseErr != nil {
			continue
		}
		if gv.Group != gvr.Group || gv.Version != gvr.Version {
			continue
		}
		for _, r := range list.APIResources {
			if r.Name == gvr.Resource {
				return r, list.GroupVersion, nil
			}
		}
	}
	return metav1.APIResource{}, "", fmt.Errorf("API resource %q not found in group %q", gvr.Resource, gvr.Group)
}

// findSubResources returns names of sub-resources for the given GVR.
func findSubResources(cc *kube.ContextClient, gvr schema.GroupVersionResource) []string {
	_, apiLists, err := cc.Discovery.ServerGroupsAndResources()
	if err != nil {
		return nil
	}

	prefix := gvr.Resource + "/"
	var subs []string
	for _, list := range apiLists {
		gv, parseErr := schema.ParseGroupVersion(list.GroupVersion)
		if parseErr != nil {
			continue
		}
		if gv.Group != gvr.Group || gv.Version != gvr.Version {
			continue
		}
		for _, r := range list.APIResources {
			if strings.HasPrefix(r.Name, prefix) {
				subs = append(subs, r.Name[len(prefix):])
			}
		}
	}
	return subs
}

// buildDescription creates a human-readable description for the resource.
func buildDescription(res metav1.APIResource, apiVersion, fieldPath string) string {
	scope := "namespaced"
	if !res.Namespaced {
		scope = "cluster-scoped"
	}

	desc := fmt.Sprintf("%s is a %s resource in %s.", res.Kind, scope, apiVersion)

	if len(res.ShortNames) > 0 {
		desc += fmt.Sprintf(" Short names: %s.", strings.Join(res.ShortNames, ", "))
	}

	if fieldPath != "" {
		desc += fmt.Sprintf(" Requested field path: %s.", fieldPath)
	}

	return desc
}
