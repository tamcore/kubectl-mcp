package tools

import (
	"context"
	"encoding/json"
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
			Kind       string   `json:"kind"`
			APIVersion string   `json:"apiVersion"`
			Namespaced bool     `json:"namespaced"`
			Verbs      []string `json:"verbs"`
		}

		var entries []entry
		for _, list := range apiLists {
			for _, r := range list.APIResources {
				if strings.Contains(r.Name, "/") {
					continue
				}
				entries = append(entries, entry{
					Kind:       r.Kind,
					APIVersion: list.GroupVersion,
					Namespaced: r.Namespaced,
					Verbs:      r.Verbs,
				})
			}
		}

		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Kind != entries[j].Kind {
				return entries[i].Kind < entries[j].Kind
			}
			return entries[i].APIVersion < entries[j].APIVersion
		})

		out, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	})
}
