package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

func TestStopPortForward_ListEmpty(t *testing.T) {
	// Clear any leftover entries from other tests.
	activeForwards.Range(func(key, _ any) bool {
		activeForwards.Delete(key)
		return true
	})

	handler := getHandler(t, "stop_port_forward", func(s *server.MCPServer) {
		registerStopPortForward(s)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "No active port-forward sessions") {
		t.Errorf("expected no active sessions message, got: %s", text)
	}
}

func TestStopPortForward_ListActive(t *testing.T) {
	// Clear and set up test entries.
	activeForwards.Range(func(key, _ any) bool {
		activeForwards.Delete(key)
		return true
	})

	ch1 := make(chan struct{})
	ch2 := make(chan struct{})
	activeForwards.Store("default/pod-a/8080", ch1)
	activeForwards.Store("default/pod-b/9090", ch2)

	t.Cleanup(func() {
		activeForwards.Delete("default/pod-a/8080")
		activeForwards.Delete("default/pod-b/9090")
	})

	handler := getHandler(t, "stop_port_forward", func(s *server.MCPServer) {
		registerStopPortForward(s)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "default/pod-a/8080") {
		t.Errorf("expected pod-a session in list, got: %s", text)
	}
	if !strings.Contains(text, "default/pod-b/9090") {
		t.Errorf("expected pod-b session in list, got: %s", text)
	}
}

func TestStopPortForward_StopSession(t *testing.T) {
	// Clear and set up a test entry.
	activeForwards.Range(func(key, _ any) bool {
		activeForwards.Delete(key)
		return true
	})

	ch := make(chan struct{})
	activeForwards.Store("default/my-pod/8080", ch)

	handler := getHandler(t, "stop_port_forward", func(s *server.MCPServer) {
		registerStopPortForward(s)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"sessionId": "default/my-pod/8080",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Stopped port-forward session") {
		t.Errorf("expected stop confirmation, got: %s", text)
	}

	// Verify channel was closed.
	select {
	case <-ch:
		// OK, channel is closed.
	default:
		t.Error("expected stop channel to be closed")
	}

	// Verify it was removed from the map.
	if _, loaded := activeForwards.Load("default/my-pod/8080"); loaded {
		t.Error("expected session to be removed from activeForwards")
	}
}

func TestStopPortForward_SessionNotFound(t *testing.T) {
	// Clear any leftover entries.
	activeForwards.Range(func(key, _ any) bool {
		activeForwards.Delete(key)
		return true
	})

	handler := getHandler(t, "stop_port_forward", func(s *server.MCPServer) {
		registerStopPortForward(s)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"sessionId": "nonexistent/session/key",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !res.IsError {
		t.Error("expected error for nonexistent session")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "no active port-forward session") {
		t.Errorf("expected not found error, got: %s", text)
	}
}
