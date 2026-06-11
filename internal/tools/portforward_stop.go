package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerStopPortForward(s *server.MCPServer) {
	tool := mcp.NewTool("stop_port_forward",
		mcp.WithDescription("Stop an active port-forward session by session key, or list all active sessions if no key is provided"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("sessionId",
			mcp.Description("Session key to stop (e.g. 'namespace/pod/localPort'). If omitted, lists all active sessions."),
		),
	)

	s.AddTool(tool, func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessionID := req.GetString("sessionId", "")

		// If no sessionId, list active forwards.
		if sessionID == "" {
			return listActiveForwards()
		}

		return stopForwardSession(sessionID)
	})
}

// listActiveForwards returns a JSON list of all active port-forward session keys.
func listActiveForwards() (*mcp.CallToolResult, error) {
	var keys []string
	activeForwards.Range(func(key, _ any) bool {
		if k, ok := key.(string); ok {
			keys = append(keys, k)
		}
		return true
	})

	sort.Strings(keys)

	if len(keys) == 0 {
		return mcp.NewToolResultText("No active port-forward sessions"), nil
	}

	out, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal sessions: %v", err)), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

// stopForwardSession stops a specific port-forward session by key.
func stopForwardSession(sessionID string) (*mcp.CallToolResult, error) {
	val, loaded := activeForwards.LoadAndDelete(sessionID)
	if !loaded {
		return mcp.NewToolResultError(fmt.Sprintf("no active port-forward session with key %q", sessionID)), nil
	}

	if session, ok := val.(*portForwardSession); ok {
		session.stop()
	}

	return mcp.NewToolResultText(fmt.Sprintf("Stopped port-forward session %q", sessionID)), nil
}
