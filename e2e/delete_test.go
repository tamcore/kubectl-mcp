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

func TestDelete_RejectedWithoutAllowDestructive(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowDestructive = false
			// AllowWrite stays true.

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			result := callTool(t, c, "delete_resource", map[string]any{
				"kind":      "ConfigMap",
				"name":      "anything",
				"namespace": testNamespace,
			})
			// Tool should not be registered.
			if !result.IsError {
				t.Error("expected error — delete_resource should not be registered without --allow-destructive")
			}
		})
	}
}
