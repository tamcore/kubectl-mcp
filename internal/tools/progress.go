package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// sendProgress emits a notifications/progress notification to the client.
// It is a no-op when the request contains no progressToken or when no MCP
// server is available in the context (e.g. unit tests).
func sendProgress(ctx context.Context, req mcp.CallToolRequest, current, total int, message string) {
	if req.Params.Meta == nil || req.Params.Meta.ProgressToken == nil {
		return
	}
	srv := server.ServerFromContext(ctx)
	if srv == nil {
		return
	}
	_ = srv.SendNotificationToClient(ctx, "notifications/progress", map[string]any{
		"progressToken": req.Params.Meta.ProgressToken,
		"progress":      float64(current),
		"total":         float64(total),
		"message":       message,
	})
}
