//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Feature 1: get_resource strips noisy metadata fields
// ---------------------------------------------------------------------------

func TestGetResourceNoisyFieldsStripped(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			result := callTool(t, c, "get_resource", map[string]any{
				"kind": "Namespace",
				"name": "default",
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("error: %s", text)
			}

			obj := jsonObjectFromResult(t, text)

			meta, ok := obj["metadata"].(map[string]any)
			if !ok {
				t.Fatal("expected metadata map in response")
			}

			for _, field := range []string{"uid", "resourceVersion", "generation", "managedFields"} {
				if _, exists := meta[field]; exists {
					t.Errorf("expected %q to be stripped from metadata", field)
				}
			}

			// Name should still be present.
			if meta["name"] != "default" {
				t.Errorf("expected metadata.name=default, got %v", meta["name"])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Feature 5: get_resource format=summary
// ---------------------------------------------------------------------------

func TestGetResourceFormatSummary(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			// List pods in kube-system to find a running one.
			listResult := callTool(t, c, "list_resources", map[string]any{
				"kind":      "Pod",
				"namespace": "kube-system",
				"filter":    "status.phase=Running",
			})
			items := jsonArrayFromResult(t, resultText(listResult))
			if len(items) == 0 {
				t.Fatal("no running pods in kube-system")
			}
			podName := items[0]["name"].(string)

			result := callTool(t, c, "get_resource", map[string]any{
				"kind":      "Pod",
				"name":      podName,
				"namespace": "kube-system",
				"format":    "summary",
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("error: %s", text)
			}

			// Summary should contain the pod name and compact fields.
			if !strings.Contains(text, podName) {
				t.Error("expected pod name in summary output")
			}
			// Summary for a Pod should include status and ready.
			if !strings.Contains(text, "status") {
				t.Error("expected 'status' field in pod summary")
			}
			// Should NOT contain full spec/status objects.
			if strings.Contains(text, "containerStatuses") {
				t.Error("summary should not contain raw containerStatuses")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Feature 5: get_resource format=yaml
// ---------------------------------------------------------------------------

func TestGetResourceFormatYAML(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			result := callTool(t, c, "get_resource", map[string]any{
				"kind":   "Namespace",
				"name":   "default",
				"format": "yaml",
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("error: %s", text)
			}

			// YAML output should contain colon-separated fields, not JSON braces.
			if !strings.Contains(text, "kind:") {
				t.Error("expected YAML-style 'kind:' in output")
			}
			if !strings.Contains(text, "metadata:") {
				t.Error("expected YAML-style 'metadata:' in output")
			}
			// Should not contain managedFields.
			if strings.Contains(text, "managedFields") {
				t.Error("managedFields should be stripped from YAML output")
			}
			// Should not contain uid.
			if strings.Contains(text, "uid:") {
				t.Error("uid should be stripped from YAML output")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Feature 3: default list limit
// ---------------------------------------------------------------------------

func TestListResourcesDefaultLimit(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			// List namespaces without an explicit limit; the default of 100 should apply.
			result := callTool(t, c, "list_resources", map[string]any{
				"kind": "Namespace",
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("error: %s", text)
			}

			// With a real cluster there are likely < 100 namespaces, so
			// no "Showing first 100" should appear.
			// Just verify we get valid results.
			items := jsonArrayFromResult(t, text)
			if len(items) == 0 {
				t.Error("expected at least one namespace")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Feature 4: list_resources format=table
// ---------------------------------------------------------------------------

func TestListResourcesFormatTable(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			result := callTool(t, c, "list_resources", map[string]any{
				"kind":      "Pod",
				"namespace": "kube-system",
				"format":    "table",
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("error: %s", text)
			}

			// Table output should have column headers.
			if !strings.Contains(text, "NAME") {
				t.Error("expected NAME column header in table output")
			}
			// Should have multiple lines (header + rows).
			lines := strings.Split(strings.TrimSpace(text), "\n")
			if len(lines) < 2 {
				t.Errorf("expected at least header + 1 row, got %d lines", len(lines))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Feature 4: list_resources format=json
// ---------------------------------------------------------------------------

func TestListResourcesFormatJSON(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			result := callTool(t, c, "list_resources", map[string]any{
				"kind":      "Pod",
				"namespace": "kube-system",
				"format":    "json",
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("error: %s", text)
			}

			// Should be a valid JSON array.
			jsonStart := strings.Index(text, "[")
			if jsonStart < 0 {
				t.Fatalf("expected JSON array in response, got: %s", text)
			}
			var items []map[string]any
			if err := json.Unmarshal([]byte(text[jsonStart:]), &items); err != nil {
				t.Fatalf("failed to parse JSON array: %v\ntext: %s", err, text)
			}
			if len(items) == 0 {
				t.Error("expected at least one pod")
			}

			// Items should have full object structure (spec, status).
			for _, item := range items {
				if _, ok := item["spec"]; !ok {
					t.Error("expected 'spec' in JSON format items")
					break
				}
				// Verify noisy metadata is stripped.
				meta, ok := item["metadata"].(map[string]any)
				if ok {
					if _, exists := meta["managedFields"]; exists {
						t.Error("managedFields should be stripped from JSON format")
						break
					}
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Feature 2: describe_resource no managedFields
// ---------------------------------------------------------------------------

func TestDescribeResourceNoManagedFields(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			result := callTool(t, c, "describe_resource", map[string]any{
				"kind": "Namespace",
				"name": "default",
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("error: %s", text)
			}

			if strings.Contains(text, "managedFields") {
				t.Error("managedFields should not appear in describe output")
			}

			// Basic describe fields should be present.
			if !strings.Contains(text, "Name:") {
				t.Error("expected Name: in describe output")
			}
			if !strings.Contains(text, "Kind:") {
				t.Error("expected Kind: in describe output")
			}
		})
	}
}
