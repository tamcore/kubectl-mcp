//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestRunPod(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("create_and_verify", func(t *testing.T) {
				name := "e2e-run-" + strings.ToLower(tc.name)
				result := callTool(t, c, "run_pod", map[string]any{
					"namespace": testNamespace,
					"name":      name,
					"image":     "busybox:1.36",
					"command":   "sleep 3600",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Created Pod") {
					t.Errorf("expected 'Created Pod' confirmation, got: %s", text)
				}

				t.Cleanup(func() { deleteViaKubectl(t, "pod", name, testNamespace) })

				// Verify via kubectl that the pod exists.
				out, err := kubectlOutput("get", "pod", name, "-n", testNamespace,
					"-o", "jsonpath={.spec.containers[0].image}")
				if err != nil {
					t.Fatalf("kubectl get: %v", err)
				}
				if out != "busybox:1.36" {
					t.Errorf("expected busybox:1.36, got: %s", out)
				}
			})

			t.Run("with_restart_policy", func(t *testing.T) {
				name := "e2e-run-always-" + strings.ToLower(tc.name)
				result := callTool(t, c, "run_pod", map[string]any{
					"namespace":     testNamespace,
					"name":          name,
					"image":         "busybox:1.36",
					"command":       "sleep 3600",
					"restartPolicy": "Always",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}

				t.Cleanup(func() { deleteViaKubectl(t, "pod", name, testNamespace) })

				out, err := kubectlOutput("get", "pod", name, "-n", testNamespace,
					"-o", "jsonpath={.spec.restartPolicy}")
				if err != nil {
					t.Fatalf("kubectl get: %v", err)
				}
				if out != "Always" {
					t.Errorf("expected Always, got: %s", out)
				}
			})

			t.Run("invalid_restart_policy_returns_error", func(t *testing.T) {
				result := callTool(t, c, "run_pod", map[string]any{
					"namespace":     testNamespace,
					"name":          "e2e-run-bad",
					"image":         "busybox:1.36",
					"restartPolicy": "Invalid",
				})
				if !result.IsError {
					t.Error("expected error for invalid restart policy")
					t.Cleanup(func() { deleteViaKubectl(t, "pod", "e2e-run-bad", testNamespace) })
				}
				text := resultText(result)
				if !strings.Contains(text, "invalid restartPolicy") {
					t.Errorf("expected 'invalid restartPolicy' message, got: %s", text)
				}
			})
		})
	}
}

func TestRunPod_RejectedWithoutAllowWrite(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowWrite = false
			cfg.AllowDestructive = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			_, err := callToolMayFail(t, c, "run_pod", map[string]any{
				"namespace": testNamespace,
				"name":      "should-not-exist",
				"image":     "busybox:1.36",
			})
			if err == nil {
				t.Error("expected error -- run_pod should not be registered without --allow-write")
			}
			if !strings.Contains(err.Error(), "not found") {
				t.Errorf("expected 'not found' error, got: %v", err)
			}
		})
	}
}
