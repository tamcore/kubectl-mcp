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
		})
	}
}
