//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestRolloutPause(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			name := "e2e-pause-" + strings.ToLower(tc.name)

			callTool(t, c, "apply_resource", map[string]any{
				"manifest": deploymentManifest(name, testNamespace, 1),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "deployment", name, testNamespace) })
			waitForDeploymentReady(t, name, testNamespace)

			t.Run("pause", func(t *testing.T) {
				result := callTool(t, c, "rollout_pause", map[string]any{
					"kind":      "Deployment",
					"name":      name,
					"namespace": testNamespace,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Paused Deployment/"+name) {
					t.Errorf("expected pause confirmation, got: %s", text)
				}

				// Verify the deployment is paused.
				out, _ := kubectlOutput("get", "deployment", name, "-n", testNamespace,
					"-o", "jsonpath={.spec.paused}")
				if out != "true" {
					t.Errorf("expected spec.paused=true, got: %s", out)
				}
			})

			t.Run("resume", func(t *testing.T) {
				result := callTool(t, c, "rollout_resume", map[string]any{
					"kind":      "Deployment",
					"name":      name,
					"namespace": testNamespace,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Resumed Deployment/"+name) {
					t.Errorf("expected resume confirmation, got: %s", text)
				}

				// Verify the deployment is no longer paused.
				out, _ := kubectlOutput("get", "deployment", name, "-n", testNamespace,
					"-o", "jsonpath={.spec.paused}")
				if out == "true" {
					t.Error("expected spec.paused to be removed after resume")
				}
			})

			t.Run("reject_statefulset", func(t *testing.T) {
				result := callTool(t, c, "rollout_pause", map[string]any{
					"kind":      "StatefulSet",
					"name":      "any",
					"namespace": testNamespace,
				})
				if !result.IsError {
					t.Error("expected error for StatefulSet")
				}
				text := resultText(result)
				if !strings.Contains(text, "does not support rollout pause/resume") {
					t.Errorf("expected unsupported error, got: %s", text)
				}
			})
		})
	}
}

func TestRolloutPause_RejectedWithoutAllowWrite(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowWrite = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			_, err := callToolMayFail(t, c, "rollout_pause", map[string]any{
				"kind":      "Deployment",
				"name":      "anything",
				"namespace": testNamespace,
			})
			if err == nil {
				t.Error("expected error -- rollout_pause should not be registered without --allow-write")
			}
		})
	}
}
