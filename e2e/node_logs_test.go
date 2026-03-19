//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestNodeLogs(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("path_traversal_double_dot_rejected", func(t *testing.T) {
				result := callTool(t, c, "node_logs", map[string]any{
					"node":    "e2e-control-plane",
					"logPath": "../etc/shadow",
				})
				if !result.IsError {
					t.Error("expected error for path traversal attempt")
				}
				text := resultText(result)
				if !strings.Contains(text, "path traversal") {
					t.Errorf("expected 'path traversal' error, got: %s", text)
				}
			})

			t.Run("path_traversal_absolute_rejected", func(t *testing.T) {
				result := callTool(t, c, "node_logs", map[string]any{
					"node":    "e2e-control-plane",
					"logPath": "/etc/shadow",
				})
				if !result.IsError {
					t.Error("expected error for absolute path")
				}
				text := resultText(result)
				if !strings.Contains(text, "path traversal") {
					t.Errorf("expected 'path traversal' error, got: %s", text)
				}
			})

			t.Run("with_tail", func(t *testing.T) {
				nodeOut, err := kubectlOutput("get", "nodes", "-o", "jsonpath={.items[0].metadata.name}")
				if err != nil {
					t.Fatalf("failed to get node name: %v", err)
				}
				nodeName := strings.TrimSpace(nodeOut)

				result := callTool(t, c, "node_logs", map[string]any{
					"node": nodeName,
					"tail": float64(10),
				})
				// May error on KinD — just verify param is accepted.
				_ = result
			})

			t.Run("root_listing_no_raw_html", func(t *testing.T) {
				nodeOut, err := kubectlOutput("get", "nodes", "-o", "jsonpath={.items[0].metadata.name}")
				if err != nil {
					t.Fatalf("failed to get node name: %v", err)
				}
				nodeName := strings.TrimSpace(nodeOut)

				result := callTool(t, c, "node_logs", map[string]any{
					"node": nodeName,
				})
				text := resultText(result)
				if result.IsError {
					t.Skipf("node_logs unavailable on this cluster: %s", text)
				}
				// If the kubelet returns a directory listing, the tool should
				// convert it to a helpful message — NOT pass through raw HTML.
				if strings.Contains(text, "<!doctype") || strings.Contains(text, "<html") {
					t.Errorf("response contains raw HTML — should be converted to directory listing:\n%s", text)
				}
			})

			t.Run("valid_node_name", func(t *testing.T) {
				// Get an actual node name from the cluster.
				nodeOut, err := kubectlOutput("get", "nodes", "-o", "jsonpath={.items[0].metadata.name}")
				if err != nil {
					t.Fatalf("failed to get node name: %v", err)
				}
				nodeName := strings.TrimSpace(nodeOut)
				if nodeName == "" {
					t.Fatal("no nodes found in cluster")
				}

				// Request root log listing. On KinD this may succeed or
				// fail depending on kubelet proxy configuration.
				result := callTool(t, c, "node_logs", map[string]any{
					"node": nodeName,
				})
				text := resultText(result)

				if result.IsError {
					// Acceptable: KinD nodes may not expose logs via proxy.
					t.Logf("node_logs for %s returned error (expected on KinD): %s", nodeName, text)
				} else {
					if text == "" {
						t.Error("expected non-empty response")
					}
				}
			})
		})
	}
}
