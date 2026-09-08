package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// capabilitySession is a minimal ClientSession that reports fixed client
// capabilities.
type capabilitySession struct {
	capabilities mcp.ClientCapabilities
}

func (s *capabilitySession) Initialize()       {}
func (s *capabilitySession) Initialized() bool { return true }
func (s *capabilitySession) NotificationChannel() chan<- mcp.JSONRPCNotification {
	return make(chan mcp.JSONRPCNotification, 1)
}
func (s *capabilitySession) SessionID() string                              { return "test-session" }
func (s *capabilitySession) GetClientInfo() mcp.Implementation              { return mcp.Implementation{} }
func (s *capabilitySession) SetClientInfo(mcp.Implementation)               {}
func (s *capabilitySession) GetClientCapabilities() mcp.ClientCapabilities  { return s.capabilities }
func (s *capabilitySession) SetClientCapabilities(c mcp.ClientCapabilities) { s.capabilities = c }

func sessionContext(t *testing.T, capabilities mcp.ClientCapabilities) context.Context {
	t.Helper()
	s := server.NewMCPServer("test", "1.0", server.WithElicitation())
	return s.WithContext(t.Context(), &capabilitySession{capabilities: capabilities})
}

func confirmResponses(t *testing.T, result mcp.ElicitationResult) mcp.InputResponses {
	t.Helper()
	return mcp.InputResponses{confirmInputID: mcp.NewElicitationInputResponse(result)}
}

func TestConfirmDestructiveAction_RequestsInputWhenClientSupportsElicitation(t *testing.T) {
	// Arrange
	ctx := sessionContext(t, mcp.ClientCapabilities{Elicitation: new(mcp.ElicitationCapability)})

	// Act
	confirmed, pending := confirmDestructiveAction(ctx, mcp.CallToolRequest{}, "Delete this?")

	// Assert
	if pending == nil {
		t.Fatal("expected an input request result")
	}
	if confirmed {
		t.Error("expected no confirmation before the client answers")
	}
	if pending.ResultType != mcp.ResultTypeInputRequired {
		t.Errorf("expected result type %q, got %q", mcp.ResultTypeInputRequired, pending.ResultType)
	}
	request, ok := pending.InputRequests[confirmInputID]
	if !ok {
		t.Fatalf("expected an input request keyed %q, got %v", confirmInputID, pending.InputRequests)
	}
	if request.Method != mcp.MethodElicitationCreate {
		t.Errorf("expected an elicitation request, got %q", request.Method)
	}
}

func TestConfirmDestructiveAction_ProceedsWithoutElicitationCapability(t *testing.T) {
	// Arrange
	ctx := sessionContext(t, mcp.ClientCapabilities{})

	// Act
	confirmed, pending := confirmDestructiveAction(ctx, mcp.CallToolRequest{}, "Delete this?")

	// Assert
	if pending != nil {
		t.Fatalf("expected no input request, got %+v", pending)
	}
	if !confirmed {
		t.Error("expected graceful degradation to proceed without confirmation")
	}
}

func TestConfirmDestructiveAction_NoSessionProceeds(t *testing.T) {
	// Arrange, Act
	confirmed, pending := confirmDestructiveAction(t.Context(), mcp.CallToolRequest{}, "Delete this?")

	// Assert
	if pending != nil {
		t.Fatalf("expected no input request, got %+v", pending)
	}
	if !confirmed {
		t.Error("expected sessions without client info to proceed")
	}
}

func TestConfirmDestructiveAction_AnswerDecidesOutcome(t *testing.T) {
	tests := []struct {
		name      string
		result    mcp.ElicitationResult
		wantAllow bool
	}{
		{
			name:      "accepted confirmation proceeds",
			result:    mcp.ElicitationResult{Action: mcp.ElicitationResponseActionAccept, Content: map[string]any{"confirm": true}},
			wantAllow: true,
		},
		{
			name:      "declined confirmation cancels",
			result:    mcp.ElicitationResult{Action: mcp.ElicitationResponseActionAccept, Content: map[string]any{"confirm": false}},
			wantAllow: false,
		},
		{
			name:      "rejected elicitation cancels",
			result:    mcp.ElicitationResult{Action: mcp.ElicitationResponseActionDecline},
			wantAllow: false,
		},
		{
			name:      "unparsable content proceeds",
			result:    mcp.ElicitationResult{Action: mcp.ElicitationResponseActionAccept, Content: "yes"},
			wantAllow: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctx := sessionContext(t, mcp.ClientCapabilities{Elicitation: new(mcp.ElicitationCapability)})
			req := mcp.CallToolRequest{}
			req.Params.InputResponses = confirmResponses(t, tc.result)

			// Act
			confirmed, pending := confirmDestructiveAction(ctx, req, "Delete this?")

			// Assert
			if pending != nil {
				t.Fatalf("expected no further input request, got %+v", pending)
			}
			if confirmed != tc.wantAllow {
				t.Errorf("expected confirmed=%v, got %v", tc.wantAllow, confirmed)
			}
		})
	}
}
