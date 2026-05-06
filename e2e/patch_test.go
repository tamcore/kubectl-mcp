//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestPatch(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("merge_patch", func(t *testing.T) {
				name := "e2e-patch-merge"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": configMapManifest(name, testNamespace, map[string]string{"a": "1"}),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

				result := callTool(t, c, "patch_resource", map[string]any{
					"kind":      "ConfigMap",
					"name":      name,
					"namespace": testNamespace,
					"patch":     `{"data":{"b":"2"}}`,
					"patchType": "merge",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Patched ConfigMap/"+name) {
					t.Errorf("expected patch confirmation, got: %s", text)
				}

				out, err := kubectlOutput("get", "configmap", name, "-n", testNamespace,
					"-o", "jsonpath={.data.a},{.data.b}")
				if err != nil {
					t.Fatalf("kubectl get: %v", err)
				}
				if out != "1,2" {
					t.Errorf("expected 1,2 got: %s", out)
				}
			})

			t.Run("strategic_merge_patch", func(t *testing.T) {
				name := "e2e-patch-strategic"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": configMapManifest(name, testNamespace, map[string]string{"x": "1"}),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

				result := callTool(t, c, "patch_resource", map[string]any{
					"kind":      "ConfigMap",
					"name":      name,
					"namespace": testNamespace,
					"patch":     `{"data":{"y":"2"}}`,
					"patchType": "strategic",
				})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}

				out, _ := kubectlOutput("get", "configmap", name, "-n", testNamespace,
					"-o", "jsonpath={.data.x},{.data.y}")
				if out != "1,2" {
					t.Errorf("expected 1,2 got: %s", out)
				}
			})

			t.Run("json_patch", func(t *testing.T) {
				name := "e2e-patch-json"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": configMapManifest(name, testNamespace, map[string]string{"val": "old"}),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

				result := callTool(t, c, "patch_resource", map[string]any{
					"kind":      "ConfigMap",
					"name":      name,
					"namespace": testNamespace,
					"patch":     `[{"op":"replace","path":"/data/val","value":"new"}]`,
					"patchType": "json",
				})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}

				out, _ := kubectlOutput("get", "configmap", name, "-n", testNamespace,
					"-o", "jsonpath={.data.val}")
				if out != "new" {
					t.Errorf("expected new, got: %s", out)
				}
			})

			t.Run("patch_as_object", func(t *testing.T) {
				name := "e2e-patch-obj"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": configMapManifest(name, testNamespace, map[string]string{"a": "1"}),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

				// LLM sends patch as JSON object, not string.
				result := callTool(t, c, "patch_resource", map[string]any{
					"kind":      "ConfigMap",
					"name":      name,
					"namespace": testNamespace,
					"patch":     map[string]any{"data": map[string]any{"c": "3"}},
					"patchType": "merge",
				})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}

				out, _ := kubectlOutput("get", "configmap", name, "-n", testNamespace,
					"-o", "jsonpath={.data.c}")
				if out != "3" {
					t.Errorf("expected 3, got: %s", out)
				}
			})

			t.Run("with_dryRun", func(t *testing.T) {
				name := "e2e-patch-dry"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": configMapManifest(name, testNamespace, map[string]string{"a": "1"}),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

				result := callTool(t, c, "patch_resource", map[string]any{
					"kind":      "ConfigMap",
					"name":      name,
					"namespace": testNamespace,
					"patch":     `{"data":{"dry":"run"}}`,
					"patchType": "merge",
					"dryRun":    true,
				})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}
				// Verify the dry-run didn't actually apply.
				out, _ := kubectlOutput("get", "configmap", name, "-n", testNamespace,
					"-o", "jsonpath={.data.dry}")
				if out == "run" {
					t.Error("dry-run should not have persisted the change")
				}
			})

			t.Run("with_apiVersion", func(t *testing.T) {
				name := "e2e-patch-apiv"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": configMapManifest(name, testNamespace, map[string]string{"x": "1"}),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

				result := callTool(t, c, "patch_resource", map[string]any{
					"kind":       "ConfigMap",
					"name":       name,
					"namespace":  testNamespace,
					"apiVersion": "v1",
					"patch":      `{"data":{"y":"2"}}`,
					"patchType":  "merge",
				})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}
			})

			t.Run("invalid_patch_type_returns_error", func(t *testing.T) {
				result := callTool(t, c, "patch_resource", map[string]any{
					"kind":      "ConfigMap",
					"name":      "anything",
					"namespace": testNamespace,
					"patch":     `{}`,
					"patchType": "invalid",
				})
				if !result.IsError {
					t.Error("expected error for invalid patch type")
				}
			})

			t.Run("with_subresource_status", func(t *testing.T) {
				if err := kubectlApplyStdin(crdWithStatusManifest()); err != nil {
					t.Fatalf("apply CRD: %v", err)
				}
				t.Cleanup(func() {
					_ = kubectl("delete", "crd", "widgete2es.e2e.kubectl-mcp.dev", "--ignore-not-found", "--wait=false")
				})
				if err := kubectl("wait", "--for=condition=Established",
					"crd/widgete2es.e2e.kubectl-mcp.dev", "--timeout=30s"); err != nil {
					t.Fatalf("CRD not established: %v", err)
				}

				name := "e2e-widget-status"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": widgetCRManifest(name, testNamespace),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "widgete2e", name, testNamespace) })

				result := callTool(t, c, "patch_resource", map[string]any{
					"kind":        "WidgetE2E",
					"name":        name,
					"namespace":   testNamespace,
					"apiVersion":  "e2e.kubectl-mcp.dev/v1alpha1",
					"patch":       `{"status":{"phase":"Ready"}}`,
					"patchType":   "merge",
					"subresource": "status",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("expected success, got: %s", text)
				}
				if !strings.Contains(text, "(subresource: status)") {
					t.Errorf("expected subresource in response message, got: %s", text)
				}

				phase, err := kubectlOutput("get", "widgete2e", name, "-n", testNamespace,
					"-o", "jsonpath={.status.phase}")
				if err != nil {
					t.Fatalf("kubectl get status: %v", err)
				}
				if phase != "Ready" {
					t.Errorf("expected status.phase=Ready, got: %q", phase)
				}

				size, err := kubectlOutput("get", "widgete2e", name, "-n", testNamespace,
					"-o", "jsonpath={.spec.size}")
				if err != nil {
					t.Fatalf("kubectl get spec: %v", err)
				}
				if size != "1" {
					t.Errorf("expected spec.size=1 unchanged, got: %q", size)
				}
			})

			t.Run("with_subresource_invalid_returns_error", func(t *testing.T) {
				result := callTool(t, c, "patch_resource", map[string]any{
					"kind":        "ConfigMap",
					"name":        "anything",
					"namespace":   testNamespace,
					"patch":       `{}`,
					"patchType":   "merge",
					"subresource": "metadata",
				})
				if !result.IsError {
					t.Error("expected error for invalid subresource")
				}
				if !strings.Contains(resultText(result), "invalid subresource") {
					t.Errorf("expected 'invalid subresource' in error, got: %s", resultText(result))
				}
			})

			t.Run("with_subresource_status_no_spec_mutation", func(t *testing.T) {
				if err := kubectlApplyStdin(crdWithStatusManifest()); err != nil {
					t.Fatalf("apply CRD: %v", err)
				}
				t.Cleanup(func() {
					_ = kubectl("delete", "crd", "widgete2es.e2e.kubectl-mcp.dev", "--ignore-not-found", "--wait=false")
				})
				if err := kubectl("wait", "--for=condition=Established",
					"crd/widgete2es.e2e.kubectl-mcp.dev", "--timeout=30s"); err != nil {
					t.Fatalf("CRD not established: %v", err)
				}

				name := "e2e-widget-nospec"
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": widgetCRManifest(name, testNamespace),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "widgete2e", name, testNamespace) })

				// Patch via status subresource with a body that also sets spec.size=99.
				// The API server must discard the spec change and only apply the status.
				result := callTool(t, c, "patch_resource", map[string]any{
					"kind":        "WidgetE2E",
					"name":        name,
					"namespace":   testNamespace,
					"apiVersion":  "e2e.kubectl-mcp.dev/v1alpha1",
					"patch":       `{"spec":{"size":99},"status":{"phase":"Done"}}`,
					"patchType":   "merge",
					"subresource": "status",
				})
				if result.IsError {
					t.Fatalf("expected success, got: %s", resultText(result))
				}

				phase, _ := kubectlOutput("get", "widgete2e", name, "-n", testNamespace,
					"-o", "jsonpath={.status.phase}")
				if phase != "Done" {
					t.Errorf("expected status.phase=Done, got: %q", phase)
				}

				size, _ := kubectlOutput("get", "widgete2e", name, "-n", testNamespace,
					"-o", "jsonpath={.spec.size}")
				if size != "1" {
					t.Errorf("expected spec.size=1 (unchanged by status patch), got: %q", size)
				}
			})
		})
	}
}

func TestPatch_RejectedWithoutAllowWrite(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowWrite = false
			cfg.AllowDestructive = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			_, err := callToolMayFail(t, c, "patch_resource", map[string]any{
				"kind":      "ConfigMap",
				"name":      "anything",
				"namespace": testNamespace,
				"patch":     `{}`,
				"patchType": "merge",
			})
			if err == nil {
				t.Error("expected error -- patch_resource should not be registered without --allow-write")
			}
			if err != nil && !strings.Contains(err.Error(), "not found") {
				t.Errorf("expected 'not found' error, got: %v", err)
			}
		})
	}
}
