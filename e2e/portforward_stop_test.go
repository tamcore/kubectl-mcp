//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestStopPortForward(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("list_empty", func(t *testing.T) {
				result := callTool(t, c, "stop_port_forward", map[string]any{})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("unexpected error: %s", text)
				}
				if !strings.Contains(text, "No active port-forward sessions") {
					t.Errorf("expected no active sessions message, got: %s", text)
				}
			})

			t.Run("nonexistent_session", func(t *testing.T) {
				result := callTool(t, c, "stop_port_forward", map[string]any{
					"sessionId": "nonexistent/session/key",
				})
				if !result.IsError {
					t.Error("expected error for nonexistent session")
				}
				text := resultText(result)
				if !strings.Contains(text, "no active port-forward session") {
					t.Errorf("expected not found error, got: %s", text)
				}
			})
		})
	}
}
