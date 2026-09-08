package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// confirmInputID keys the confirmation elicitation inside the multi round-trip
// input requests of a destructive tool call.
const confirmInputID = "confirm"

// confirmDestructiveAction resolves the confirmation for a destructive tool
// call using multi round-trip requests (SEP-2322).
//
// It returns a non-nil result when the caller must return immediately: the
// client is asked to collect the confirmation and retries the tool call with
// the answer attached. mcp-go performs the equivalent server-initiated
// elicitation on behalf of clients using a protocol version before 2026-07-28.
//
// proceed reports whether the operation may run. It is false only when the
// user declined. Clients that do not declare the elicitation capability
// proceed without confirmation (graceful degradation).
func confirmDestructiveAction(ctx context.Context, req mcp.CallToolRequest, message string) (proceed bool, pending *mcp.CallToolResult) {
	if answer := server.ElicitationResponse(req.Params.InputResponses, confirmInputID); answer != nil {
		return elicitationConfirms(answer), nil
	}

	if !clientSupportsElicitation(ctx) {
		return true, nil
	}

	params := mcp.ElicitationParams{
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
	}

	return false, server.NewInputRequestBuilder("").Elicit(confirmInputID, params).ToolResult()
}

// elicitationConfirms reports whether an elicitation answer approves the
// operation. Responses that cannot be parsed proceed (graceful degradation).
func elicitationConfirms(answer *mcp.ElicitationResult) bool {
	if answer.Action != mcp.ElicitationResponseActionAccept {
		return false
	}

	data, ok := answer.Content.(map[string]any)
	if !ok {
		return true
	}

	confirm, ok := data["confirm"].(bool)
	if !ok {
		return true
	}
	return confirm
}

// clientSupportsElicitation reports whether the current session declared the
// elicitation capability during initialization. Sessions that track no client
// info cannot answer an input request, so they count as unsupported and the
// operation proceeds without confirmation.
func clientSupportsElicitation(ctx context.Context) bool {
	session, ok := server.ClientSessionFromContext(ctx).(server.SessionWithClientInfo)
	if !ok {
		return false
	}
	return session.GetClientCapabilities().Elicitation != nil
}
