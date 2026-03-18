//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestRolloutStatus(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			name := "e2e-rollst-" + strings.ToLower(tc.name)
			manifest := deploymentManifest(name, testNamespace, 1)
			callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
			t.Cleanup(func() { deleteViaKubectl(t, "deployment", name, testNamespace) })
			waitForDeploymentReady(t, name, testNamespace)

			t.Run("shows_complete", func(t *testing.T) {
				result := callTool(t, c, "rollout_status", map[string]any{
					"kind":      "Deployment",
					"name":      name,
					"namespace": testNamespace,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				obj := jsonObjectFromResult(t, text)
				if obj["complete"] != true {
					t.Errorf("expected complete=true, got: %v", obj["complete"])
				}
				if obj["kind"] != "Deployment" {
					t.Errorf("expected kind=Deployment, got: %v", obj["kind"])
				}
			})

			t.Run("nonexistent_returns_error", func(t *testing.T) {
				result := callTool(t, c, "rollout_status", map[string]any{
					"kind":      "Deployment",
					"name":      "no-such-deploy-xyz",
					"namespace": testNamespace,
				})
				if !result.IsError {
					t.Error("expected error for nonexistent deployment")
				}
			})

			t.Run("unsupported_kind_returns_error", func(t *testing.T) {
				result := callTool(t, c, "rollout_status", map[string]any{
					"kind":      "ConfigMap",
					"name":      "anything",
					"namespace": testNamespace,
				})
				if !result.IsError {
					t.Error("expected error for unsupported kind")
				}
				text := resultText(result)
				if !strings.Contains(text, "does not support rollout status") {
					t.Errorf("expected 'does not support rollout status' message, got: %s", text)
				}
			})
		})
	}
}

func TestRolloutHistory(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			name := "e2e-rollhist-" + strings.ToLower(tc.name)
			manifest := deploymentManifest(name, testNamespace, 1)
			callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
			t.Cleanup(func() { deleteViaKubectl(t, "deployment", name, testNamespace) })
			waitForDeploymentReady(t, name, testNamespace)

			t.Run("lists_revision_1", func(t *testing.T) {
				result := callTool(t, c, "rollout_history", map[string]any{
					"kind":      "Deployment",
					"name":      name,
					"namespace": testNamespace,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				obj := jsonObjectFromResult(t, text)
				revisions, ok := obj["revisions"].([]any)
				if !ok || len(revisions) == 0 {
					t.Fatalf("expected at least one revision, got: %v", obj["revisions"])
				}
				firstRev := revisions[0].(map[string]any)
				if firstRev["revision"].(float64) != 1 {
					t.Errorf("expected revision 1, got: %v", firstRev["revision"])
				}
			})

			t.Run("specific_revision", func(t *testing.T) {
				result := callTool(t, c, "rollout_history", map[string]any{
					"kind":      "Deployment",
					"name":      name,
					"namespace": testNamespace,
					"revision":  float64(1),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				obj := jsonObjectFromResult(t, text)
				if obj["revision"].(float64) != 1 {
					t.Errorf("expected revision=1, got: %v", obj["revision"])
				}
			})

			t.Run("nonexistent_revision_returns_error", func(t *testing.T) {
				result := callTool(t, c, "rollout_history", map[string]any{
					"kind":      "Deployment",
					"name":      name,
					"namespace": testNamespace,
					"revision":  float64(999),
				})
				if !result.IsError {
					t.Error("expected error for nonexistent revision")
				}
			})

			t.Run("unsupported_kind_returns_error", func(t *testing.T) {
				result := callTool(t, c, "rollout_history", map[string]any{
					"kind":      "StatefulSet",
					"name":      "anything",
					"namespace": testNamespace,
				})
				if !result.IsError {
					t.Error("expected error for unsupported kind")
				}
				text := resultText(result)
				if !strings.Contains(text, "does not support rollout history") {
					t.Errorf("expected 'does not support rollout history' message, got: %s", text)
				}
			})
		})
	}
}

func TestRolloutUndo(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			name := "e2e-rollundo-" + strings.ToLower(tc.name)

			// Create deployment with initial image.
			manifest1 := deploymentManifestWithImage(name, testNamespace, "nginx:1.27-alpine", 1)
			callTool(t, c, "apply_resource", map[string]any{"manifest": manifest1})
			t.Cleanup(func() { deleteViaKubectl(t, "deployment", name, testNamespace) })
			waitForDeploymentReady(t, name, testNamespace)

			// Update to a different image to create revision 2.
			manifest2 := deploymentManifestWithImage(name, testNamespace, "nginx:1.26-alpine", 1)
			callTool(t, c, "apply_resource", map[string]any{"manifest": manifest2})
			waitForDeploymentReady(t, name, testNamespace)

			t.Run("undo_to_previous", func(t *testing.T) {
				result := callTool(t, c, "rollout_undo", map[string]any{
					"kind":      "Deployment",
					"name":      name,
					"namespace": testNamespace,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Rolled back") {
					t.Errorf("expected 'Rolled back' confirmation, got: %s", text)
				}

				// Verify the image was restored to the original.
				waitForDeploymentReady(t, name, testNamespace)
				out, err := kubectlOutput("get", "deployment", name, "-n", testNamespace,
					"-o", "jsonpath={.spec.template.spec.containers[0].image}")
				if err != nil {
					t.Fatalf("kubectl get: %v", err)
				}
				if !strings.Contains(out, "1.27") {
					t.Errorf("expected original image (1.27), got: %s", out)
				}
			})

			t.Run("undo_to_specific_revision", func(t *testing.T) {
				// Undo to revision 1 explicitly using toRevision.
				result := callTool(t, c, "rollout_undo", map[string]any{
					"kind":       "Deployment",
					"name":       name,
					"namespace":  testNamespace,
					"toRevision": float64(1),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Rolled back") {
					t.Errorf("expected 'Rolled back' confirmation, got: %s", text)
				}
			})

			t.Run("unsupported_kind_returns_error", func(t *testing.T) {
				result := callTool(t, c, "rollout_undo", map[string]any{
					"kind":      "ConfigMap",
					"name":      "anything",
					"namespace": testNamespace,
				})
				if !result.IsError {
					t.Error("expected error for unsupported kind")
				}
				text := resultText(result)
				if !strings.Contains(text, "does not support rollout undo") {
					t.Errorf("expected 'does not support rollout undo' message, got: %s", text)
				}
			})
		})
	}
}

func TestRolloutUndo_RejectedWithoutAllowWrite(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowWrite = false
			cfg.AllowDestructive = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			_, err := callToolMayFail(t, c, "rollout_undo", map[string]any{
				"kind":      "Deployment",
				"name":      "anything",
				"namespace": testNamespace,
			})
			if err == nil {
				t.Error("expected error -- rollout_undo should not be registered without --allow-write")
			}
			if !strings.Contains(err.Error(), "not found") {
				t.Errorf("expected 'not found' error, got: %v", err)
			}
		})
	}
}
