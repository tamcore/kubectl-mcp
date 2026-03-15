package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerListResources(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	tool := mcp.NewTool("list_resources",
		mcp.WithDescription("List Kubernetes resources of a given kind"),
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
		mcp.WithString("namespace",
			mcp.Description("Namespace to list in (omit for cluster-scoped or all namespaces)"),
		),
		mcp.WithString("labelSelector",
			mcp.Description("Label selector (e.g. app=nginx). Preferred way to filter by labels."),
		),
		mcp.WithString("filter",
			mcp.Description("Client-side filter on any resource field using dot-notation. "+
				"Supports any field path (e.g. status.phase=Running, spec.replicas=3, "+
				"status.containerStatuses.0.ready=true). Multiple filters comma-separated. "+
				"Use this instead of fieldSelector for non-metadata fields."),
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
		namespace := req.GetString("namespace", "")
		apiVersion := req.GetString("apiVersion", "")
		labelSelector := req.GetString("labelSelector", "")
		filter := req.GetString("filter", "")

		gvr, err := resolveGVR(cc, kind, apiVersion)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		opts := metav1.ListOptions{
			LabelSelector: labelSelector,
		}

		var list *unstructured.UnstructuredList
		if namespace != "" {
			list, err = cc.Dynamic.Resource(gvr).Namespace(namespace).List(ctx, opts)
		} else {
			list, err = cc.Dynamic.Resource(gvr).List(ctx, opts)
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to list %s: %v", kind, err)), nil
		}

		if !cfg.AllowSecrets {
			kube.RedactSecretsList(list)
		}

		// Apply client-side filters.
		filters, err := parseFilters(filter)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid filter: %v", err)), nil
		}
		items := applyFilters(list.Items, filters)

		if len(items) == 0 {
			if len(filters) > 0 {
				return mcp.NewToolResultText(fmt.Sprintf("No %s matched filter (checked %d resources)", kind, len(list.Items))), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("No %s found", kind)), nil
		}

		jsonOut, err := formatResourceList(items)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to format results: %v", err)), nil
		}

		if len(filters) > 0 {
			header := fmt.Sprintf("Matched %d of %d %s\n\n", len(items), len(list.Items), kind)
			return mcp.NewToolResultText(header + jsonOut), nil
		}
		return mcp.NewToolResultText(jsonOut), nil
	})
}

// filterExpr represents a single field=value filter.
type filterExpr struct {
	path   []string
	value  string
	negate bool
}

// parseFilters parses a comma-separated list of field=value or field!=value expressions.
func parseFilters(raw string) ([]filterExpr, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var filters []filterExpr
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		var key, val string
		var negate bool

		if idx := strings.Index(part, "!="); idx > 0 {
			key = strings.TrimSpace(part[:idx])
			val = strings.TrimSpace(part[idx+2:])
			negate = true
		} else if idx := strings.Index(part, "="); idx > 0 {
			key = strings.TrimSpace(part[:idx])
			val = strings.TrimSpace(part[idx+1:])
		} else {
			return nil, fmt.Errorf("invalid filter expression %q: expected field=value or field!=value", part)
		}

		filters = append(filters, filterExpr{
			path:   strings.Split(key, "."),
			value:  val,
			negate: negate,
		})
	}
	return filters, nil
}

// applyFilters returns only the items that match all filter expressions.
func applyFilters(items []unstructured.Unstructured, filters []filterExpr) []unstructured.Unstructured {
	if len(filters) == 0 {
		return items
	}

	var result []unstructured.Unstructured
	for _, item := range items {
		if matchesAllFilters(item.Object, filters) {
			result = append(result, item)
		}
	}
	return result
}

func matchesAllFilters(obj map[string]interface{}, filters []filterExpr) bool {
	for _, f := range filters {
		actual, found := nestedFieldValue(obj, f.path)
		if !found {
			// Field doesn't exist: equality fails, negation also fails
			// (can't assert != on a missing field).
			return false
		}
		matches := actual == f.value
		if f.negate {
			matches = !matches
		}
		if !matches {
			return false
		}
	}
	return true
}

// nestedFieldValue traverses the object using the dot-path and returns
// the value as a string plus whether the field was found.
func nestedFieldValue(obj interface{}, path []string) (string, bool) {
	current := obj
	for _, key := range path {
		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[key]
			if !ok {
				return "", false
			}
			current = val
		case []interface{}:
			idx := 0
			for _, c := range key {
				if c < '0' || c > '9' {
					return "", false
				}
				idx = idx*10 + int(c-'0')
			}
			if idx >= len(v) {
				return "", false
			}
			current = v[idx]
		default:
			return "", false
		}
	}
	return fmt.Sprintf("%v", current), true
}
