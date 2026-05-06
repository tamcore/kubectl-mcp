package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestSendProgressNoOpWithoutToken(t *testing.T) {
	// No progressToken in request — must not panic.
	req := mcp.CallToolRequest{}
	sendProgress(context.Background(), req, 1, 5, "step 1")
}

func TestSendProgressNoOpWithoutServer(t *testing.T) {
	// progressToken present but no MCP server in context — must not panic.
	req := mcp.CallToolRequest{}
	token := "test-token"
	req.Params.Meta = &mcp.Meta{ProgressToken: token}
	sendProgress(context.Background(), req, 1, 5, "step 1")
}
