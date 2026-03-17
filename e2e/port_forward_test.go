//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestPortForward(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("remote_port_zero_returns_error", func(t *testing.T) {
				result := callTool(t, c, "port_forward", map[string]any{
					"namespace":  testNamespace,
					"pod":        "any-pod",
					"remotePort": float64(0),
				})
				if !result.IsError {
					t.Error("expected error for remotePort=0")
				}
				text := resultText(result)
				if !strings.Contains(text, "remotePort must be a valid port number") {
					t.Errorf("expected port validation error, got: %s", text)
				}
			})

			t.Run("nonexistent_pod", func(t *testing.T) {
				// The SPDY forwarder is a placeholder that always errors,
				// so this tests that the tool produces an error on invocation.
				result := callTool(t, c, "port_forward", map[string]any{
					"namespace":  testNamespace,
					"pod":        "no-such-pod-xyz",
					"remotePort": float64(8080),
				})
				if !result.IsError {
					t.Error("expected error for nonexistent pod / placeholder forwarder")
				}
			})
		})
	}
}

func TestPortForward_RejectedWithoutAllowWrite(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowWrite = false
			cfg.AllowDestructive = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			_, err := callToolMayFail(t, c, "port_forward", map[string]any{
				"namespace":  testNamespace,
				"pod":        "any-pod",
				"remotePort": float64(8080),
			})
			if err == nil {
				t.Error("expected error -- port_forward should not be registered without --allow-write")
			}
			if !strings.Contains(err.Error(), "not found") {
				t.Errorf("expected 'not found' error, got: %v", err)
			}
		})
	}
}
