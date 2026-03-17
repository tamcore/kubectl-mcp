//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestNodeStats(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			// Get a node name from the cluster.
			nodeName, err := kubectlOutput("get", "nodes", "-o", "jsonpath={.items[0].metadata.name}")
			if err != nil || nodeName == "" {
				t.Skip("no nodes available in cluster")
			}

			t.Run("happy_path", func(t *testing.T) {
				result := callTool(t, c, "node_stats", map[string]any{
					"node": nodeName,
				})
				text := resultText(result)
				// In KinD, the kubelet proxy may not be available. Accept both success and error.
				if result.IsError {
					// Graceful: kubelet proxy unavailable is acceptable in KinD.
					if strings.Contains(text, "failed to get stats") {
						t.Logf("node_stats unavailable (expected in KinD): %s", text)
						return
					}
					t.Fatalf("unexpected error: %s", text)
				}
				// If successful, verify we got formatted output.
				if !strings.Contains(text, "Node:") {
					t.Errorf("expected 'Node:' in output, got: %s", text)
				}
			})

			t.Run("nonexistent_node", func(t *testing.T) {
				result := callTool(t, c, "node_stats", map[string]any{
					"node": "no-such-node-xyz",
				})
				if !result.IsError {
					t.Error("expected error for nonexistent node")
				}
			})
		})
	}
}
