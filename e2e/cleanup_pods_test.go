//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestCleanupPods(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			suffix := strings.ToLower(tc.name)

			t.Run("dry_run_succeeded", func(t *testing.T) {
				// Create a pod that will succeed (exit 0).
				name := "e2e-cleanup-ok-" + suffix
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": podManifest(name, testNamespace, "busybox:1.36", []string{"sh", "-c", "exit 0"}),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "pod", name, testNamespace) })

				waitForPodPhase(t, name, testNamespace, "Succeeded")

				result := callTool(t, c, "cleanup_pods", map[string]any{
					"namespace": testNamespace,
					"dryRun":    true,
					"states":    "Succeeded",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "DRY RUN") {
					t.Errorf("expected DRY RUN header, got: %s", text)
				}
			})

			t.Run("execute_cleanup", func(t *testing.T) {
				// Create a pod that will fail (exit 1).
				name := "e2e-cleanup-fail-" + suffix
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": podManifest(name, testNamespace, "busybox:1.36", []string{"sh", "-c", "exit 1"}),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "pod", name, testNamespace) })

				waitForPodPhase(t, name, testNamespace, "Failed")

				result := callTool(t, c, "cleanup_pods", map[string]any{
					"namespace": testNamespace,
					"states":    "Failed",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Cleaned up pods") {
					t.Errorf("expected cleanup report, got: %s", text)
				}
			})

			t.Run("no_matching_pods", func(t *testing.T) {
				// Use a state that likely has no matching pods.
				result := callTool(t, c, "cleanup_pods", map[string]any{
					"namespace": testNamespace,
					"states":    "Evicted",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "No pods in states") {
					t.Errorf("expected no-match message, got: %s", text)
				}
			})
		})
	}
}

func TestCleanupPods_RejectedWithoutAllowDestructive(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowDestructive = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			_, err := callToolMayFail(t, c, "cleanup_pods", map[string]any{
				"namespace": testNamespace,
			})
			if err == nil {
				t.Error("expected error -- cleanup_pods should not be registered without --allow-destructive")
			}
		})
	}
}
