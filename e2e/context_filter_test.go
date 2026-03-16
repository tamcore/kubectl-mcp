//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestContextFilter(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("allowed_context_works", func(t *testing.T) {
				cfg := defaultConfig()
				cfg.AllowedContexts = []string{"kind-e2e"}

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				result := callTool(t, c, "list_contexts", nil)
				text := resultText(result)
				if !strings.Contains(text, "kind-e2e") {
					t.Errorf("expected kind-e2e in allowed contexts, got: %s", text)
				}

				// Should be able to list namespaces.
				result = callTool(t, c, "list_namespaces", nil)
				if result.IsError {
					t.Fatalf("list_namespaces failed: %s", resultText(result))
				}
			})

			t.Run("denied_context_rejected", func(t *testing.T) {
				cfg := defaultConfig()
				cfg.AllowedContexts = []string{"*"}
				cfg.DeniedContexts = []string{"kind-e2e"}

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				// list_contexts should not include kind-e2e.
				result := callTool(t, c, "list_contexts", nil)
				text := resultText(result)
				if strings.Contains(text, "kind-e2e") {
					t.Errorf("kind-e2e should not appear in denied contexts, got: %s", text)
				}

				// Explicit use should fail.
				result = callTool(t, c, "list_namespaces", map[string]any{"context": "kind-e2e"})
				if !result.IsError {
					t.Error("expected error for denied context")
				}
				if !strings.Contains(resultText(result), "not allowed") {
					t.Errorf("expected 'not allowed' error, got: %s", resultText(result))
				}
			})

			t.Run("regex_denied_pattern", func(t *testing.T) {
				cfg := defaultConfig()
				cfg.AllowedContexts = []string{"*"}
				cfg.DeniedContexts = []string{"/^kind-.*/"}

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				result := callTool(t, c, "list_contexts", nil)
				text := resultText(result)
				if strings.Contains(text, "kind-e2e") {
					t.Errorf("kind-e2e should be denied by regex, got: %s", text)
				}
			})

			t.Run("glob_allowed_pattern", func(t *testing.T) {
				cfg := defaultConfig()
				cfg.AllowedContexts = []string{"kind-*"}

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				result := callTool(t, c, "list_namespaces", nil)
				if result.IsError {
					t.Fatalf("list_namespaces failed with glob: %s", resultText(result))
				}
			})

			t.Run("deny_takes_precedence_over_allow", func(t *testing.T) {
				cfg := defaultConfig()
				cfg.AllowedContexts = []string{"kind-e2e"}
				cfg.DeniedContexts = []string{"kind-e2e"}

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				result := callTool(t, c, "list_namespaces", map[string]any{"context": "kind-e2e"})
				if !result.IsError {
					t.Error("expected error — deny should take precedence over allow")
				}
			})
		})
	}
}
