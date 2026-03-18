//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestCordonUncordon(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			// Get the node name.
			listResult := callTool(t, c, "list_resources", map[string]any{"kind": "Node"})
			nodes := jsonArrayFromResult(t, resultText(listResult))
			if len(nodes) == 0 {
				t.Fatal("no nodes found")
			}
			nodeName := nodes[0]["name"].(string)

			// Always uncordon in cleanup.
			t.Cleanup(func() {
				callTool(t, c, "uncordon_node", map[string]any{"node": nodeName})
			})

			t.Run("cordon_then_uncordon", func(t *testing.T) {
				// Cordon.
				result := callTool(t, c, "cordon_node", map[string]any{"node": nodeName})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("cordon error: %s", text)
				}
				if !strings.Contains(text, "Cordoned node") {
					t.Errorf("expected cordon confirmation, got: %s", text)
				}

				out, _ := kubectlOutput("get", "node", nodeName, "-o", "jsonpath={.spec.unschedulable}")
				if out != "true" {
					t.Errorf("expected unschedulable=true, got: %s", out)
				}

				// Uncordon.
				result = callTool(t, c, "uncordon_node", map[string]any{"node": nodeName})
				text = resultText(result)
				if result.IsError {
					t.Fatalf("uncordon error: %s", text)
				}
				if !strings.Contains(text, "Uncordoned node") {
					t.Errorf("expected uncordon confirmation, got: %s", text)
				}

				out, _ = kubectlOutput("get", "node", nodeName, "-o", "jsonpath={.spec.unschedulable}")
				if out != "" && out != "false" {
					t.Errorf("expected unschedulable to be unset, got: %s", out)
				}
			})

			t.Run("nonexistent_node_returns_error", func(t *testing.T) {
				result := callTool(t, c, "cordon_node", map[string]any{"node": "no-such-node-xyz"})
				if !result.IsError {
					t.Error("expected error for nonexistent node")
				}
			})
		})
	}
}

func TestCordonUncordon_RejectedWithoutAllowWrite(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowWrite = false
			cfg.AllowDestructive = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			_, err := callToolMayFail(t, c, "cordon_node", map[string]any{
				"node": "anything",
			})
			if err == nil {
				t.Error("expected error -- cordon_node should not be registered without --allow-write")
			}
			if err != nil && !strings.Contains(err.Error(), "not found") {
				t.Errorf("expected 'not found' error, got: %v", err)
			}

			_, err = callToolMayFail(t, c, "uncordon_node", map[string]any{
				"node": "anything",
			})
			if err == nil {
				t.Error("expected error -- uncordon_node should not be registered without --allow-write")
			}
			if err != nil && !strings.Contains(err.Error(), "not found") {
				t.Errorf("expected 'not found' error, got: %v", err)
			}
		})
	}
}
