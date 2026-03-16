//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestRateLimit(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("read_limit_enforced", func(t *testing.T) {
				cfg := defaultConfig()
				cfg.RateLimitRead = 2 // Very low: 2/min, burst=1

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				rateLimited := false
				for i := 0; i < 10; i++ {
					result := callTool(t, c, "list_contexts", nil)
					if result.IsError && strings.Contains(resultText(result), "Rate limited") {
						rateLimited = true
						break
					}
				}
				if !rateLimited {
					t.Error("expected at least one rate-limited response")
				}
			})

			t.Run("write_limit_enforced", func(t *testing.T) {
				cfg := defaultConfig()
				cfg.RateLimitWrite = 2 // Very low: 2/min, burst=1

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				// Create a resource to patch.
				callTool(t, c, "apply_resource", map[string]any{
					"manifest": configMapManifest("e2e-rl-write", testNamespace, map[string]string{"k": "v"}),
				})
				t.Cleanup(func() { deleteViaKubectl(t, "configmap", "e2e-rl-write", testNamespace) })

				rateLimited := false
				for i := 0; i < 10; i++ {
					result := callTool(t, c, "patch_resource", map[string]any{
						"kind":      "ConfigMap",
						"name":      "e2e-rl-write",
						"namespace": testNamespace,
						"patch":     `{"data":{"k":"v"}}`,
						"patchType": "merge",
					})
					if result.IsError && strings.Contains(resultText(result), "Rate limited") {
						rateLimited = true
						break
					}
				}
				if !rateLimited {
					t.Error("expected at least one write rate-limited response")
				}
			})

			t.Run("unlimited_when_zero", func(t *testing.T) {
				cfg := defaultConfig()
				cfg.RateLimitRead = 0 // unlimited

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				for i := 0; i < 20; i++ {
					result := callTool(t, c, "list_contexts", nil)
					if result.IsError && strings.Contains(resultText(result), "Rate limited") {
						t.Fatalf("unexpected rate limit at iteration %d", i)
					}
				}
			})
		})
	}
}
