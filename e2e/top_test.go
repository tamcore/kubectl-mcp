//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestTopPods(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			// KinD typically does not have metrics-server installed,
			// so we expect a graceful error indicating metrics-server is not available.
			result := callTool(t, c, "top_pods", map[string]any{
				"namespace": "kube-system",
			})
			text := resultText(result)

			if result.IsError {
				if !strings.Contains(text, "metrics-server") && !strings.Contains(text, "metrics.k8s.io") {
					t.Errorf("expected metrics-server error, got: %s", text)
				}
			} else {
				// If metrics-server happens to be installed, just verify non-empty output.
				t.Log("metrics-server is available; top_pods returned data")
				if text == "" {
					t.Error("expected non-empty response")
				}
			}
		})
	}
}

func TestTopNodes(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			result := callTool(t, c, "top_nodes", nil)
			text := resultText(result)

			if result.IsError {
				if !strings.Contains(text, "metrics-server") && !strings.Contains(text, "metrics.k8s.io") {
					t.Errorf("expected metrics-server error, got: %s", text)
				}
			} else {
				t.Log("metrics-server is available; top_nodes returned data")
				if text == "" {
					t.Error("expected non-empty response")
				}
			}
		})
	}
}
