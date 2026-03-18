//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestScale(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("deployment_scale_up", func(t *testing.T) {
				name := "e2e-scale-up"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": deploymentManifest(name, testNamespace, 1),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "deployment", name, testNamespace) })
				// Wait for the controller to reconcile the initial state.
				time.Sleep(2 * time.Second)

				result := callTool(t, c, "scale_resource", map[string]any{
					"kind":      "Deployment",
					"name":      name,
					"namespace": testNamespace,
					"replicas":  float64(3),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Scaled Deployment/"+name) {
					t.Errorf("expected scale confirmation, got: %s", text)
				}
				if !strings.Contains(text, "to 3") {
					t.Errorf("expected 'to 3' in output, got: %s", text)
				}

				out, _ := kubectlOutput("get", "deployment", name, "-n", testNamespace,
					"-o", "jsonpath={.spec.replicas}")
				if out != "3" {
					t.Errorf("expected 3 replicas, got: %s", out)
				}
			})

			t.Run("deployment_scale_down", func(t *testing.T) {
				name := "e2e-scale-down"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": deploymentManifest(name, testNamespace, 3),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "deployment", name, testNamespace) })
				time.Sleep(2 * time.Second)

				result := callTool(t, c, "scale_resource", map[string]any{
					"kind":      "Deployment",
					"name":      name,
					"namespace": testNamespace,
					"replicas":  float64(1),
				})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}

				out, _ := kubectlOutput("get", "deployment", name, "-n", testNamespace,
					"-o", "jsonpath={.spec.replicas}")
				if out != "1" {
					t.Errorf("expected 1 replica, got: %s", out)
				}
			})

			t.Run("statefulset", func(t *testing.T) {
				name := "e2e-scale-sts"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": statefulSetManifest(name, testNamespace, 1),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "statefulset", name, testNamespace) })
				time.Sleep(2 * time.Second)

				result := callTool(t, c, "scale_resource", map[string]any{
					"kind":      "StatefulSet",
					"name":      name,
					"namespace": testNamespace,
					"replicas":  float64(2),
				})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}
				if !strings.Contains(resultText(result), "Scaled StatefulSet/"+name) {
					t.Errorf("expected scale confirmation, got: %s", resultText(result))
				}
			})
		})
	}
}

func TestScale_RejectedWithoutAllowWrite(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowWrite = false
			cfg.AllowDestructive = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			_, err := callToolMayFail(t, c, "scale_resource", map[string]any{
				"kind":      "Deployment",
				"name":      "anything",
				"namespace": testNamespace,
				"replicas":  float64(1),
			})
			if err == nil {
				t.Error("expected error -- scale_resource should not be registered without --allow-write")
			}
			if err != nil && !strings.Contains(err.Error(), "not found") {
				t.Errorf("expected 'not found' error, got: %v", err)
			}
		})
	}
}
