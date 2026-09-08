package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

type apiResourceEntry struct {
	Kind       string   `json:"kind"`
	APIVersion string   `json:"apiVersion"`
	Namespaced bool     `json:"namespaced"`
	Verbs      []string `json:"verbs"`
}

func registerListAPIResources(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("list_api_resources",
		mcp.WithDescription(
			"List available API resources (kinds) in the cluster. "+
				"Default output is a compact table; use format=json for full details. "+
				"Use group, namespaced, and verb filters to narrow results.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("format",
			mcp.Description("Output format: 'table' (default, compact) or 'json' (full details with structuredContent)"),
		),
		mcp.WithString("group",
			mcp.Description("Filter by API group (e.g. 'apps', 'batch'). Use 'core' for the core/v1 group."),
		),
		mcp.WithString("namespaced",
			mcp.Description("Filter by scope: 'true' for namespaced, 'false' for cluster-scoped"),
		),
		mcp.WithString("verb",
			mcp.Description("Filter to resources supporting this verb (e.g. 'list', 'create', 'delete')"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cc, _, errResult := resolveClient(pool, req)
		if errResult != nil {
			return errResult, nil
		}

		apiLists, err := discoverAPILists(cc)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to discover API resources: %v", err)), nil
		}

		groupFilter := req.GetString("group", "")
		namespacedFilter := req.GetString("namespaced", "")
		verbFilter := req.GetString("verb", "")
		format := req.GetString("format", "table")

		entries := collectAPIResources(apiLists, groupFilter, namespacedFilter, verbFilter)

		slices.SortFunc(entries, func(a, b apiResourceEntry) int {
			return cmp.Or(
				cmp.Compare(a.Kind, b.Kind),
				cmp.Compare(a.APIVersion, b.APIVersion),
			)
		})

		switch format {
		case "json":
			return formatAPIResourcesJSON(entries)
		default:
			return formatAPIResourcesTable(entries), nil
		}
	})
}

// collectAPIResources gathers and filters API resources from discovery.
func collectAPIResources(
	apiLists []*metav1.APIResourceList,
	groupFilter, namespacedFilter, verbFilter string,
) []apiResourceEntry {
	var entries []apiResourceEntry

	for _, list := range apiLists {
		gv := list.GroupVersion
		group := groupFromGV(gv)

		if !matchGroup(group, groupFilter) {
			continue
		}

		for _, r := range list.APIResources {
			if strings.Contains(r.Name, "/") {
				continue
			}
			if !matchNamespaced(r.Namespaced, namespacedFilter) {
				continue
			}
			if !matchVerb(r.Verbs, verbFilter) {
				continue
			}
			entries = append(entries, apiResourceEntry{
				Kind:       r.Kind,
				APIVersion: gv,
				Namespaced: r.Namespaced,
				Verbs:      r.Verbs,
			})
		}
	}
	return entries
}

// groupFromGV extracts the API group from a GroupVersion string.
// For "v1" (core), returns "". For "apps/v1", returns "apps".
func groupFromGV(gv string) string {
	if before, _, ok := strings.Cut(gv, "/"); ok {
		return before
	}
	return ""
}

// matchGroup returns true if the resource's group matches the filter.
func matchGroup(group, filter string) bool {
	if filter == "" {
		return true
	}
	if filter == "core" {
		return group == ""
	}
	return group == filter
}

// matchNamespaced returns true if the resource's namespaced flag matches the filter.
func matchNamespaced(namespaced bool, filter string) bool {
	switch filter {
	case "true":
		return namespaced
	case "false":
		return !namespaced
	default:
		return true
	}
}

// matchVerb returns true if the resource supports the requested verb.
func matchVerb(verbs []string, filter string) bool {
	if filter == "" {
		return true
	}
	return slices.Contains(verbs, filter)
}

// formatAPIResourcesTable renders entries as a compact columnar table.
func formatAPIResourcesTable(entries []apiResourceEntry) *mcp.CallToolResult {
	if len(entries) == 0 {
		return mcp.NewToolResultText("No API resources found")
	}

	var sb strings.Builder
	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KIND\tAPIVERSION\tNAMESPACED\tVERBS")
	for _, e := range entries {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%t\t%s\n", e.Kind, e.APIVersion, e.Namespaced, strings.Join(e.Verbs, ","))
	}
	_ = tw.Flush()

	return mcp.NewToolResultText(strings.TrimRight(sb.String(), "\n"))
}

// formatAPIResourcesJSON renders entries as JSON with structuredContent.
func formatAPIResourcesJSON(entries []apiResourceEntry) (*mcp.CallToolResult, error) {
	out, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Build structuredContent as an object envelope.
	items := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		items = append(items, map[string]any{
			"kind":       e.Kind,
			"apiVersion": e.APIVersion,
			"namespaced": e.Namespaced,
			"verbs":      e.Verbs,
		})
	}
	envelope := map[string]any{
		"items": items,
		"count": len(items),
	}

	return mcp.NewToolResultStructured(envelope, string(out)), nil
}
