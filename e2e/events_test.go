//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestGetEventsAllNamespaces(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("all_namespaces", func(t *testing.T) {
				// kube-system typically has events from system components.
				result := callTool(t, c, "get_events", map[string]any{
					"allNamespaces": true,
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				// Result should be a JSON array.
				if !strings.Contains(text, "[") {
					t.Errorf("expected JSON array in response, got: %s", text)
				}
			})

			t.Run("all_namespaces_with_namespace_errors", func(t *testing.T) {
				result := callTool(t, c, "get_events", map[string]any{
					"allNamespaces": true,
					"namespace":     testNamespace,
				})
				if !result.IsError {
					t.Error("expected error when allNamespaces=true and namespace is set")
				}
			})
		})
	}
}
