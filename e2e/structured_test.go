//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestStructuredContent(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			name := "e2e-struct-" + strings.ToLower(tc.name)

			// Create a configmap so we have something to get/list/describe.
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": configMapManifest(name, testNamespace, map[string]string{"key": "val"}),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

			t.Run("get_resource", func(t *testing.T) {
				result := callTool(t, c, "get_resource", map[string]any{
					"kind":      "ConfigMap",
					"name":      name,
					"namespace": testNamespace,
				})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}

				// Verify structured content is returned.
				if result.StructuredContent == nil {
					t.Error("expected StructuredContent to be populated for get_resource")
				}

				// Verify text fallback is present.
				text := resultText(result)
				if !strings.Contains(text, name) {
					t.Errorf("expected resource name in text fallback, got: %s", text)
				}
			})

			t.Run("list_resources", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":      "ConfigMap",
					"namespace": testNamespace,
				})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}

				if result.StructuredContent == nil {
					t.Error("expected StructuredContent to be populated for list_resources")
				}
			})

			t.Run("describe_resource", func(t *testing.T) {
				result := callTool(t, c, "describe_resource", map[string]any{
					"kind":      "ConfigMap",
					"name":      name,
					"namespace": testNamespace,
				})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}

				if result.StructuredContent == nil {
					t.Error("expected StructuredContent to be populated for describe_resource")
				}

				text := resultText(result)
				if !strings.Contains(text, "Name:") {
					t.Errorf("expected describe-style output, got: %s", text)
				}
			})
		})
	}
}
