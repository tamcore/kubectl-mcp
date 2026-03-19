//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestDrain(t *testing.T) {
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

			// Always uncordon in cleanup — critical for other tests.
			t.Cleanup(func() {
				callTool(t, c, "uncordon_node", map[string]any{"node": nodeName})
			})

			t.Run("dry_run", func(t *testing.T) {
				result := callTool(t, c, "drain_node", map[string]any{
					"node":             nodeName,
					"ignoreDaemonSets": true,
					"dryRun":           true,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("drain dry-run error: %s", text)
				}
				if !strings.Contains(text, "DRY RUN") {
					t.Errorf("expected DRY RUN in output, got: %s", text)
				}

				// Verify node is NOT cordoned after dry-run.
				out, _ := kubectlOutput("get", "node", nodeName, "-o", "jsonpath={.spec.unschedulable}")
				if out == "true" {
					t.Error("node should NOT be cordoned after dry-run")
				}
			})

			t.Run("drain_and_uncordon", func(t *testing.T) {
				// Create a standalone pod that will be evicted.
				podName := "e2e-drain-pod"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": podManifest(podName, testNamespace, "busybox:1.36", []string{"sleep", "3600"}),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "pod", podName, testNamespace) })
				waitForPodReady(t, podName, testNamespace)

				result := callTool(t, c, "drain_node", map[string]any{
					"node":               nodeName,
					"ignoreDaemonSets":   true,
					"deleteEmptyDirData": true,
					"gracePeriodSeconds": float64(10),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("drain error: %s", text)
				}
				if !strings.Contains(text, "Drained node") {
					t.Errorf("expected drain confirmation, got: %s", text)
				}

				// Verify node is cordoned.
				out, _ := kubectlOutput("get", "node", nodeName, "-o", "jsonpath={.spec.unschedulable}")
				if out != "true" {
					t.Errorf("expected node to be cordoned after drain, got: %s", out)
				}

				// Uncordon for other tests.
				uncordonResult := callTool(t, c, "uncordon_node", map[string]any{"node": nodeName})
				if uncordonResult.IsError {
					t.Fatalf("uncordon error: %s", resultText(uncordonResult))
				}
			})
		})
	}
}

// TestDrain_ForceAndTimeoutParams verifies that the new force and timeout
// parameters are accepted by the tool without parameter-level errors.
// These are smoke tests — they use dryRun=true so no actual drain happens.
func TestDrain_ForceAndTimeoutParams(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			listResult := callTool(t, c, "list_resources", map[string]any{"kind": "Node"})
			nodes := jsonArrayFromResult(t, resultText(listResult))
			if len(nodes) == 0 {
				t.Fatal("no nodes found")
			}
			nodeName := nodes[0]["name"].(string)

			t.Run("force_param_accepted", func(t *testing.T) {
				result := callTool(t, c, "drain_node", map[string]any{
					"node":             nodeName,
					"ignoreDaemonSets": true,
					"dryRun":           true,
					"force":            true,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("force param caused unexpected error: %s", text)
				}
				if !strings.Contains(text, "DRY RUN") {
					t.Errorf("expected DRY RUN in output, got: %s", text)
				}
			})

			t.Run("timeout_param_accepted", func(t *testing.T) {
				result := callTool(t, c, "drain_node", map[string]any{
					"node":             nodeName,
					"ignoreDaemonSets": true,
					"dryRun":           true,
					"timeout":          float64(30),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("timeout param caused unexpected error: %s", text)
				}
				if !strings.Contains(text, "DRY RUN") {
					t.Errorf("expected DRY RUN in output, got: %s", text)
				}
			})

			t.Run("force_and_timeout_together", func(t *testing.T) {
				result := callTool(t, c, "drain_node", map[string]any{
					"node":             nodeName,
					"ignoreDaemonSets": true,
					"dryRun":           true,
					"force":            true,
					"timeout":          float64(60),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("force+timeout params caused unexpected error: %s", text)
				}
				if !strings.Contains(text, "DRY RUN") {
					t.Errorf("expected DRY RUN in output, got: %s", text)
				}
			})
		})
	}
}

func TestDrain_RejectedWithoutAllowDestructive(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowDestructive = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			_, err := callToolMayFail(t, c, "drain_node", map[string]any{
				"node": "anything",
			})
			if err == nil {
				t.Error("expected error -- drain_node should not be registered without --allow-destructive")
			}
			if err != nil && !strings.Contains(err.Error(), "not found") {
				t.Errorf("expected 'not found' error, got: %v", err)
			}
		})
	}
}
