//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestCreate(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("create_new_configmap", func(t *testing.T) {
				manifest := configMapManifest("e2e-create-cm", testNamespace, map[string]string{"key": "value"})
				result := callTool(t, c, "create_resource", map[string]any{"manifest": manifest})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Created ConfigMap/e2e-create-cm") {
					t.Errorf("expected create confirmation, got: %s", text)
				}

				out, err := kubectlOutput("get", "configmap", "e2e-create-cm", "-n", testNamespace,
					"-o", "jsonpath={.data.key}")
				if err != nil {
					t.Fatalf("kubectl get: %v", err)
				}
				if out != "value" {
					t.Errorf("expected value, got: %s", out)
				}

				t.Cleanup(func() { deleteViaKubectl(t, "configmap", "e2e-create-cm", testNamespace) })
			})

			t.Run("conflict_on_existing_resource", func(t *testing.T) {
				name := "e2e-create-conflict"
				manifest := configMapManifest(name, testNamespace, map[string]string{"k": "v"})

				// First create should succeed.
				result := callTool(t, c, "create_resource", map[string]any{"manifest": manifest})
				if result.IsError {
					t.Fatalf("first create failed: %s", resultText(result))
				}

				// Second create must fail with conflict.
				result = callTool(t, c, "create_resource", map[string]any{"manifest": manifest})
				text := resultText(result)
				if !result.IsError {
					t.Fatalf("expected conflict error on second create, got success: %s", text)
				}
				if !strings.Contains(text, "already exists") {
					t.Errorf("expected 'already exists' in error, got: %s", text)
				}

				t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })
			})

			t.Run("dry_run_does_not_persist", func(t *testing.T) {
				name := "e2e-create-dryrun"
				manifest := configMapManifest(name, testNamespace, map[string]string{"k": "v"})
				result := callTool(t, c, "create_resource", map[string]any{
					"manifest": manifest,
					"dryRun":   true,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("dry run failed: %s", text)
				}
				if !strings.Contains(text, "DRY RUN") {
					t.Errorf("expected DRY RUN prefix, got: %s", text)
				}

				// Verify the resource was NOT actually created.
				out, _ := kubectlOutput("get", "configmap", name, "-n", testNamespace,
					"--ignore-not-found", "-o", "name")
				if out != "" {
					t.Errorf("dry-run should not have created resource, but found: %s", out)
					t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })
				}
			})

			t.Run("multi_document_yaml", func(t *testing.T) {
				manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: e2e-create-multi-1
  namespace: ` + testNamespace + `
data:
  k: v1
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: e2e-create-multi-2
  namespace: ` + testNamespace + `
data:
  k: v2`

				result := callTool(t, c, "create_resource", map[string]any{"manifest": manifest})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("multi-doc create failed: %s", text)
				}
				if !strings.Contains(text, "Created ConfigMap/e2e-create-multi-1") {
					t.Errorf("expected first doc confirmation, got: %s", text)
				}
				if !strings.Contains(text, "Created ConfigMap/e2e-create-multi-2") {
					t.Errorf("expected second doc confirmation, got: %s", text)
				}

				t.Cleanup(func() {
					deleteViaKubectl(t, "configmap", "e2e-create-multi-1", testNamespace)
					deleteViaKubectl(t, "configmap", "e2e-create-multi-2", testNamespace)
				})
			})

			t.Run("invalid_manifest_returns_error", func(t *testing.T) {
				result := callTool(t, c, "create_resource", map[string]any{"manifest": "not valid yaml"})
				if !result.IsError {
					t.Error("expected error for invalid manifest")
				}
			})
		})
	}
}

func TestCreate_RejectedWithoutAllowWrite(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowWrite = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			_, err := callToolMayFail(t, c, "create_resource", map[string]any{
				"manifest": configMapManifest("should-not-exist", testNamespace, map[string]string{"k": "v"}),
			})
			if err == nil {
				t.Error("expected error — create_resource should not be registered without --allow-write")
			}
			if !strings.Contains(err.Error(), "not found") {
				t.Errorf("expected 'not found' error, got: %v", err)
			}
		})
	}
}
