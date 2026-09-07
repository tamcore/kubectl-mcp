package tools

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerListContexts(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("list_contexts",
		mcp.WithDescription("List available Kubernetes contexts"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	)

	s.AddTool(tool, func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		contexts := pool.Contexts()
		slices.Sort(contexts)
		defaultCtx := pool.DefaultContext()

		type ctxInfo struct {
			Name      string `json:"name"`
			IsDefault bool   `json:"isDefault,omitzero"`
		}

		items := make([]ctxInfo, 0, len(contexts))
		for _, name := range contexts {
			items = append(items, ctxInfo{
				Name:      name,
				IsDefault: name == defaultCtx,
			})
		}

		out, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	})
}
