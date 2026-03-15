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

func registerListContexts(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("list_contexts",
		mcp.WithDescription("List available Kubernetes contexts"),
	)

	s.AddTool(tool, func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		contexts := pool.Contexts()
		sort.Strings(contexts)
		defaultCtx := pool.DefaultContext()

		var sb strings.Builder
		for _, name := range contexts {
			marker := "  "
			if name == defaultCtx {
				marker = "* "
			}
			fmt.Fprintf(&sb, "%s%s\n", marker, name)
		}
		if sb.Len() == 0 {
			return mcp.NewToolResultText("No contexts available"), nil
		}
		return mcp.NewToolResultText(sb.String()), nil
	})
}
