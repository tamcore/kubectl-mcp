//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestRestartRollout(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("deployment", func(t *testing.T) {
				name := "e2e-restart"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": deploymentManifest(name, testNamespace, 1),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "deployment", name, testNamespace) })

				result := callTool(t, c, "restart_rollout", map[string]any{
					"kind":      "Deployment",
					"name":      name,
					"namespace": testNamespace,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Restarted Deployment/"+name) {
					t.Errorf("expected restart confirmation, got: %s", text)
				}

				// Verify the restartedAt annotation was set.
				out, _ := kubectlOutput("get", "deployment", name, "-n", testNamespace,
					"-o", `jsonpath={.spec.template.metadata.annotations.kubectl\.kubernetes\.io/restartedAt}`)
				if out == "" {
					t.Error("expected restartedAt annotation to be set")
				}
			})
		})
	}
}

func TestRestartRollout_RejectedWithoutAllowWrite(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowWrite = false
			cfg.AllowDestructive = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			_, err := callToolMayFail(t, c, "restart_rollout", map[string]any{
				"kind":      "Deployment",
				"name":      "anything",
				"namespace": testNamespace,
			})
			if err == nil {
				t.Error("expected error -- restart_rollout should not be registered without --allow-write")
			}
			if err != nil && !strings.Contains(err.Error(), "not found") {
				t.Errorf("expected 'not found' error, got: %v", err)
			}
		})
	}
}
