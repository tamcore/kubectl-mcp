//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestOutputSchemaOnStructuredTools(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			result, err := c.ListTools(ctx, mcp.ListToolsRequest{})
			if err != nil {
				t.Fatalf("ListTools: %v", err)
			}

			want := []string{"get_resource", "list_resources", "describe_resource", "rollout_status"}
			byName := make(map[string]mcp.Tool, len(result.Tools))
			for _, tool := range result.Tools {
				byName[tool.Name] = tool
			}
			for _, name := range want {
				tool, ok := byName[name]
				if !ok {
					t.Errorf("tool %q not found in ListTools", name)
					continue
				}
				if tool.OutputSchema.Type == "" {
					t.Errorf("tool %q: expected outputSchema.type to be set, got empty", name)
				}
			}
		})
	}
}

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

				// Verify structuredContent is an object envelope (not array).
				envelope, ok := result.StructuredContent.(map[string]interface{})
				if !ok {
					t.Fatalf("expected StructuredContent to be map, got %T", result.StructuredContent)
				}
				if _, ok := envelope["items"]; !ok {
					t.Error("expected 'items' key in structured content envelope")
				}
				if _, ok := envelope["count"]; !ok {
					t.Error("expected 'count' key in structured content envelope")
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
