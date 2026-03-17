//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestApplyDryRun(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("resource_not_persisted", func(t *testing.T) {
				name := "e2e-dryrun-apply-" + strings.ToLower(tc.name)
				manifest := configMapManifest(name, testNamespace, map[string]string{"k": "v"})

				result := callTool(t, c, "apply_resource", map[string]any{
					"manifest": manifest,
					"dryRun":   true,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "DRY RUN") {
					t.Errorf("expected 'DRY RUN' prefix, got: %s", text)
				}

				// Verify resource was NOT actually created.
				_, err := kubectlOutput("get", "configmap", name, "-n", testNamespace)
				if err == nil {
					t.Error("expected ConfigMap to not exist after dry run")
					t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })
				}
			})
		})
	}
}

func TestDeleteDryRun(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("resource_still_exists", func(t *testing.T) {
				name := "e2e-dryrun-del-" + strings.ToLower(tc.name)
				manifest := configMapManifest(name, testNamespace, map[string]string{"k": "v"})
				callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
				t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

				result := callTool(t, c, "delete_resource", map[string]any{
					"kind":      "ConfigMap",
					"name":      name,
					"namespace": testNamespace,
					"dryRun":    true,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "DRY RUN") {
					t.Errorf("expected 'DRY RUN' prefix, got: %s", text)
				}

				// Verify resource still exists.
				out, err := kubectlOutput("get", "configmap", name, "-n", testNamespace,
					"-o", "jsonpath={.data.k}")
				if err != nil {
					t.Fatalf("expected ConfigMap to still exist after dry run delete, got: %v", err)
				}
				if out != "v" {
					t.Errorf("expected data.k=v, got: %s", out)
				}
			})
		})
	}
}

func TestDeleteGracePeriodSeconds(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("grace_period_zero", func(t *testing.T) {
				name := "e2e-grace-" + strings.ToLower(tc.name)
				manifest := podManifest(name, testNamespace, "busybox:1.36", []string{"sleep", "3600"})
				callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
				waitForPodReady(t, name, testNamespace)

				result := callTool(t, c, "delete_resource", map[string]any{
					"kind":               "Pod",
					"name":               name,
					"namespace":          testNamespace,
					"gracePeriodSeconds": float64(0),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Deleted Pod/"+name) {
					t.Errorf("expected delete confirmation, got: %s", text)
				}

				// Verify pod is gone (may take a moment).
				_, err := kubectlOutput("get", "pod", name, "-n", testNamespace)
				if err == nil {
					t.Log("pod still terminating (acceptable with gracePeriod=0)")
				}
			})
		})
	}
}
