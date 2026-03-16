//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestAnnotations(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			// Create an annotated pod.
			podName := "e2e-annotated-" + strings.ToLower(tc.name)
			manifest := annotatedPodManifest(podName, testNamespace, map[string]string{
				"app.kubernetes.io/name":    "myapp",
				"app.kubernetes.io/version": "1.0",
				"custom.io/owner":           "team-a",
				"internal.io/debug":         "true",
			})
			callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
			t.Cleanup(func() { deleteViaKubectl(t, "pod", podName, testNamespace) })
			waitForPodReady(t, podName, testNamespace)

			t.Run("default_excludes_last_applied_config", func(t *testing.T) {
				result := callTool(t, c, "get_resource", map[string]any{
					"kind":      "Pod",
					"name":      podName,
					"namespace": testNamespace,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if strings.Contains(text, "kubectl.kubernetes.io/last-applied-configuration") {
					t.Error("last-applied-configuration should be excluded by default")
				}
				// Other annotations should be present.
				if !strings.Contains(text, "app.kubernetes.io/name") {
					t.Error("expected app.kubernetes.io/name annotation to be present")
				}
			})

			t.Run("include_pattern", func(t *testing.T) {
				result := callTool(t, c, "get_resource", map[string]any{
					"kind":                "Pod",
					"name":                podName,
					"namespace":           testNamespace,
					"include_annotations": "app.kubernetes.io/*",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "app.kubernetes.io/name") {
					t.Error("expected app.kubernetes.io/name to be included")
				}
				if strings.Contains(text, "custom.io/owner") {
					t.Error("custom.io/owner should NOT be included")
				}
				if strings.Contains(text, "internal.io/debug") {
					t.Error("internal.io/debug should NOT be included")
				}
			})

			t.Run("exclude_pattern", func(t *testing.T) {
				result := callTool(t, c, "get_resource", map[string]any{
					"kind":                "Pod",
					"name":                podName,
					"namespace":           testNamespace,
					"exclude_annotations": "internal.io/*",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if strings.Contains(text, "internal.io/debug") {
					t.Error("internal.io/debug should be excluded")
				}
				if !strings.Contains(text, "app.kubernetes.io/name") {
					t.Error("app.kubernetes.io/name should still be present")
				}
			})

			t.Run("include_and_exclude_combined", func(t *testing.T) {
				result := callTool(t, c, "get_resource", map[string]any{
					"kind":                "Pod",
					"name":                podName,
					"namespace":           testNamespace,
					"include_annotations": "app.kubernetes.io/*,custom.io/*",
					"exclude_annotations": "custom.io/owner",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "app.kubernetes.io/name") {
					t.Error("app.kubernetes.io/name should be included")
				}
				if strings.Contains(text, "custom.io/owner") {
					t.Error("custom.io/owner should be excluded")
				}
				if strings.Contains(text, "internal.io/debug") {
					t.Error("internal.io/debug should not be included")
				}
			})

			t.Run("describe_resource_filters", func(t *testing.T) {
				result := callTool(t, c, "describe_resource", map[string]any{
					"kind":                "Pod",
					"name":                podName,
					"namespace":           testNamespace,
					"exclude_annotations": "internal.io/*",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if strings.Contains(text, "internal.io/debug") {
					t.Error("internal.io/debug should be excluded from describe output")
				}
			})

			t.Run("list_resources_filters", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":                "Pod",
					"namespace":           testNamespace,
					"include_annotations": "app.kubernetes.io/*",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				// The list response may not include annotations directly
				// (it's a summary format), but it should not error.
			})
		})
	}
}
