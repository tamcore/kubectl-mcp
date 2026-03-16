//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestFuzzyKindMatching(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("short_name_deploy", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":      "deploy",
					"namespace": "kube-system",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("expected short name 'deploy' to resolve, got error: %s", text)
				}
			})

			t.Run("short_name_svc", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":      "svc",
					"namespace": "default",
				})
				if result.IsError {
					t.Fatalf("expected short name 'svc' to resolve, got error: %s", resultText(result))
				}
			})

			t.Run("short_name_cm", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":      "cm",
					"namespace": "kube-system",
				})
				if result.IsError {
					t.Fatalf("expected short name 'cm' to resolve, got error: %s", resultText(result))
				}
			})

			t.Run("short_name_ns", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind": "ns",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("expected short name 'ns' to resolve, got error: %s", text)
				}
				if !strings.Contains(text, "default") {
					t.Errorf("expected 'default' namespace in results, got: %s", text)
				}
			})

			t.Run("typo_suggests_correction", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":      "Deploymnt",
					"namespace": "default",
				})
				text := resultText(result)
				if !result.IsError {
					t.Fatal("expected error for typo 'Deploymnt'")
				}
				if !strings.Contains(text, "did you mean") {
					t.Errorf("expected 'did you mean' suggestion, got: %s", text)
				}
				if !strings.Contains(text, "Deployment") {
					t.Errorf("expected 'Deployment' suggestion, got: %s", text)
				}
			})

			t.Run("unknown_kind_no_suggestion", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":      "CompletelyFakeResource",
					"namespace": "default",
				})
				if !result.IsError {
					t.Fatal("expected error for unknown kind")
				}
				text := resultText(result)
				if strings.Contains(text, "did you mean") {
					t.Errorf("should not suggest for completely unknown kind, got: %s", text)
				}
			})
		})
	}
}
