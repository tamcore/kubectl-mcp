package tools

import (
	"context"
	"errors"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// confirmDestructiveAction sends an elicitation request to the client asking
// for confirmation before proceeding with a destructive operation.
// Returns true if the user confirms, false if they decline or if elicitation
// is not supported (graceful degradation — proceeds without confirmation).
func confirmDestructiveAction(ctx context.Context, s *server.MCPServer, message string) (bool, error) {
	if s == nil {
		// No server reference: proceed without confirmation.
		return true, nil
	}

	request := mcp.ElicitationRequest{
		Params: mcp.ElicitationParams{
			Message: message,
			RequestedSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"confirm": map[string]any{
						"type":        "boolean",
						"description": "Confirm the destructive operation",
					},
				},
				"required": []string{"confirm"},
			},
		},
	}

	result, err := s.RequestElicitation(ctx, request)
	if err != nil {
		// Graceful degradation: if elicitation is not supported, proceed.
		if errors.Is(err, server.ErrElicitationNotSupported) || errors.Is(err, server.ErrNoActiveSession) {
			return true, nil
		}
		return false, err
	}

	if result.Action != mcp.ElicitationResponseActionAccept {
		return false, nil
	}

	// Extract the confirm field from the response.
	data, ok := result.Content.(map[string]any)
	if !ok {
		// If we can't parse the response, proceed (graceful degradation).
		return true, nil
	}

	confirm, ok := data["confirm"].(bool)
	if !ok {
		return true, nil
	}

	return confirm, nil
}
