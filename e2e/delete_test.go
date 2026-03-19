//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestDelete(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("configmap", func(t *testing.T) {
				name := "e2e-delete-cm"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": configMapManifest(name, testNamespace, map[string]string{"k": "v"}),
				})

				result := callTool(t, c, "delete_resource", map[string]any{
					"kind":      "ConfigMap",
					"name":      name,
					"namespace": testNamespace,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Deleted ConfigMap/"+name) {
					t.Errorf("expected delete confirmation, got: %s", text)
				}

				// Verify it's gone.
				_, err := kubectlOutput("get", "configmap", name, "-n", testNamespace)
				if err == nil {
					t.Error("expected configmap to be deleted")
				}
			})

			t.Run("pod", func(t *testing.T) {
				name := "e2e-delete-pod"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": podManifest(name, testNamespace, "busybox:1.36", []string{"sleep", "10"}),
				})

				result := callTool(t, c, "delete_resource", map[string]any{
					"kind":      "Pod",
					"name":      name,
					"namespace": testNamespace,
				})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}
			})

			t.Run("nonexistent_returns_error", func(t *testing.T) {
				result := callTool(t, c, "delete_resource", map[string]any{
					"kind":      "ConfigMap",
					"name":      "no-such-cm-xyz",
					"namespace": testNamespace,
				})
				if !result.IsError {
					t.Error("expected error for nonexistent resource")
				}
			})

			t.Run("with_apiVersion", func(t *testing.T) {
				name := "e2e-delete-apiver"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": configMapManifest(name, testNamespace, map[string]string{"k": "v"}),
				})

				result := callTool(t, c, "delete_resource", map[string]any{
					"kind":       "ConfigMap",
					"name":       name,
					"namespace":  testNamespace,
					"apiVersion": "v1",
				})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}
			})
		})
	}
}

func TestDelete_ForceParam(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("force_param_accepted", func(t *testing.T) {
				// force=true with dryRun=true should succeed without actually deleting anything.
				result := callTool(t, c, "delete_resource", map[string]any{
					"kind":      "ConfigMap",
					"name":      "any-cm",
					"namespace": testNamespace,
					"force":     true,
					"dryRun":    true,
				})
				// A dry-run force delete of a non-existent resource may return an error
				// from the API server ("not found"), but the tool itself must not reject
				// the force parameter — the error must not be about an unknown parameter.
				text := resultText(result)
				if strings.Contains(text, "unknown parameter") || strings.Contains(text, "invalid parameter") {
					t.Errorf("force param should be accepted, got: %s", text)
				}
			})

			t.Run("force_and_grace_period_conflict", func(t *testing.T) {
				result := callTool(t, c, "delete_resource", map[string]any{
					"kind":               "ConfigMap",
					"name":               "any-cm",
					"namespace":          testNamespace,
					"force":              true,
					"gracePeriodSeconds": float64(30),
				})
				if !result.IsError {
					t.Error("expected error when force=true and gracePeriodSeconds>0 are both set")
				}
				text := resultText(result)
				if !strings.Contains(text, "force") || !strings.Contains(text, "gracePeriodSeconds") {
					t.Errorf("expected error to mention force and gracePeriodSeconds, got: %s", text)
				}
			})
		})
	}
}

func TestDelete_RejectedWithoutAllowDestructive(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowDestructive = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			_, err := callToolMayFail(t, c, "delete_resource", map[string]any{
				"kind":      "ConfigMap",
				"name":      "anything",
				"namespace": testNamespace,
			})
			if err == nil {
				t.Error("expected error — delete_resource should not be registered without --allow-destructive")
			}
			if !strings.Contains(err.Error(), "not found") {
				t.Errorf("expected 'not found' error, got: %v", err)
			}
		})
	}
}
