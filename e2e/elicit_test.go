//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

// TestElicitation_GracefulDegradation verifies that destructive tools work
// without elicitation support in the client (graceful degradation).
// The SSE/HTTP clients in this test suite do not support elicitation,
// so the server should proceed without confirmation.
func TestElicitation_GracefulDegradation(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			// Create a resource to delete.
			name := "e2e-elicit-" + strings.ToLower(tc.name)
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": configMapManifest(name, testNamespace, map[string]string{"k": "v"}),
			})

			// Delete should succeed despite no elicitation support in the client.
			result := callTool(t, c, "delete_resource", map[string]any{
				"kind":      "ConfigMap",
				"name":      name,
				"namespace": testNamespace,
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("expected delete to succeed (graceful degradation), got error: %s", text)
			}
			if !strings.Contains(text, "Deleted ConfigMap/"+name) {
				t.Errorf("expected delete confirmation, got: %s", text)
			}
		})
	}
}

// TestElicitation_DryRunSkipsConfirmation verifies that dry-run deletes
// skip the elicitation confirmation.
func TestElicitation_DryRunSkipsConfirmation(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			name := "e2e-elicit-dry-" + strings.ToLower(tc.name)
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": configMapManifest(name, testNamespace, map[string]string{"k": "v"}),
			})
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
				t.Errorf("expected DRY RUN in output, got: %s", text)
			}
		})
	}
}
