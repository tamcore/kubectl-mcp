package tools

import (
	"context"
	"errors"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const elicitationTimeout = 5 * time.Second

// confirmDestructiveAction sends an elicitation request to the client asking
// for confirmation before proceeding with a destructive operation.
// Returns true if the user confirms, false if they decline or if elicitation
// is not supported (graceful degradation — proceeds without confirmation).
func confirmDestructiveAction(ctx context.Context, s *server.MCPServer, message string) (bool, error) {
	if s == nil {
		// No server reference: proceed without confirmation.
		return true, nil
	}

	if !clientSupportsElicitation(ctx) {
		// Client did not declare elicitation capability: proceed without confirmation.
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

	// Use a bounded timeout so that clients without elicitation support
	// don't block the tool call indefinitely.
	elicitCtx, cancel := context.WithTimeout(ctx, elicitationTimeout)
	defer cancel()

	result, err := s.RequestElicitation(elicitCtx, request)
	if err != nil {
		// Graceful degradation: if elicitation is not supported or times out, proceed.
		if errors.Is(err, server.ErrElicitationNotSupported) ||
			errors.Is(err, server.ErrNoActiveSession) ||
			errors.Is(err, context.DeadlineExceeded) {
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

// clientSupportsElicitation reports whether the current session declared the
// elicitation capability during initialization. Sessions that expose no client
// info are assumed capable so that transports without capability tracking keep
// working.
func clientSupportsElicitation(ctx context.Context) bool {
	session, ok := server.ClientSessionFromContext(ctx).(server.SessionWithClientInfo)
	if !ok {
		return true
	}
	return session.GetClientCapabilities().Elicitation != nil
}
