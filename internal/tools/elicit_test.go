package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

func TestConfirmDestructiveAction_NilServer(t *testing.T) {
	confirmed, err := confirmDestructiveAction(context.Background(), nil, "test message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !confirmed {
		t.Error("expected confirmation when server is nil")
	}
}

func TestConfirmDestructiveAction_NoSession(t *testing.T) {
	// Create a server but don't set up a session in the context.
	// This should gracefully degrade (ErrNoActiveSession).
	s := server.NewMCPServer("test", "1.0", server.WithElicitation())

	confirmed, err := confirmDestructiveAction(context.Background(), s, "Delete this?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !confirmed {
		t.Error("expected confirmation to proceed without session (graceful degradation)")
	}
}
