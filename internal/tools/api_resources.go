package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerListAPIResources(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("list_api_resources",
		mcp.WithDescription("List available API resources (kinds) in the cluster"),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
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

		_, apiLists, err := cc.Discovery.ServerGroupsAndResources()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to discover API resources: %v", err)), nil
		}

		type entry struct {
			apiVersion string
			kind       string
			namespaced bool
			verbs      string
		}

		var entries []entry
		for _, list := range apiLists {
			for _, r := range list.APIResources {
				// Skip sub-resources (contain a slash).
				if strings.Contains(r.Name, "/") {
					continue
				}
				entries = append(entries, entry{
					apiVersion: list.GroupVersion,
					kind:       r.Kind,
					namespaced: r.Namespaced,
					verbs:      strings.Join(r.Verbs, ","),
				})
			}
		}

		sort.Slice(entries, func(i, j int) bool {
			if entries[i].kind != entries[j].kind {
				return entries[i].kind < entries[j].kind
			}
			return entries[i].apiVersion < entries[j].apiVersion
		})

		var sb strings.Builder
		fmt.Fprintf(&sb, "%-40s %-30s %-12s %s\n", "KIND", "APIVERSION", "NAMESPACED", "VERBS")
		for _, e := range entries {
			fmt.Fprintf(&sb, "%-40s %-30s %-12v %s\n", e.kind, e.apiVersion, e.namespaced, e.verbs)
		}

		return mcp.NewToolResultText(sb.String()), nil
	})
}
