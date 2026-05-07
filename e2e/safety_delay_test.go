//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// safetyDelayConfig returns a config with both safety delays set to d.
func safetyDelayConfig(d time.Duration) *config.Config {
	cfg := defaultConfig()
	cfg.SafetyDelayWrite = d
	cfg.SafetyDelayDestructive = d
	return cfg
}

func TestSafetyDelay_WriteTierDelayObserved(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, safetyDelayConfig(300*time.Millisecond))
			c := tc.clientFunc(t, base)

			name := "e2e-sd-write-" + strings.ToLower(tc.name[:3])
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": configMapManifest(name, testNamespace, map[string]string{"a": "1"}),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

			start := time.Now()
			result := callTool(t, c, "patch_resource", map[string]any{
				"kind":      "ConfigMap",
				"name":      name,
				"namespace": testNamespace,
				"patch":     `{"data":{"b":"2"}}`,
				"patchType": "merge",
			})
			elapsed := time.Since(start)

			if result.IsError {
				t.Fatalf("patch_resource error: %s", resultText(result))
			}
			if elapsed < 300*time.Millisecond {
				t.Errorf("expected safety delay >= 300ms, got %v", elapsed)
			}
		})
	}
}

func TestSafetyDelay_DryRunBypassesDelay(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, safetyDelayConfig(2*time.Second))
			c := tc.clientFunc(t, base)

			name := "e2e-sd-dry-" + strings.ToLower(tc.name[:3])
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": configMapManifest(name, testNamespace, map[string]string{"a": "1"}),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

			start := time.Now()
			result := callTool(t, c, "patch_resource", map[string]any{
				"kind":      "ConfigMap",
				"name":      name,
				"namespace": testNamespace,
				"patch":     `{"data":{"b":"2"}}`,
				"patchType": "merge",
				"dryRun":    true,
			})
			elapsed := time.Since(start)

			if result.IsError {
				t.Fatalf("dry-run patch_resource error: %s", resultText(result))
			}
			if elapsed >= 500*time.Millisecond {
				t.Errorf("dryRun should skip safety delay, elapsed %v", elapsed)
			}
			if !strings.Contains(resultText(result), "DRY RUN") {
				t.Errorf("expected DRY RUN in response, got: %s", resultText(result))
			}
		})
	}
}

func TestSafetyDelay_DestructiveTierDelayObserved(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, safetyDelayConfig(300*time.Millisecond))
			c := tc.clientFunc(t, base)

			name := "e2e-sd-dest-" + strings.ToLower(tc.name[:3])
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": configMapManifest(name, testNamespace, map[string]string{"k": "v"}),
			})

			start := time.Now()
			result := callTool(t, c, "delete_resource", map[string]any{
				"kind":      "ConfigMap",
				"name":      name,
				"namespace": testNamespace,
			})
			elapsed := time.Since(start)

			if result.IsError {
				t.Fatalf("delete_resource error: %s", resultText(result))
			}
			if elapsed < 300*time.Millisecond {
				t.Errorf("expected safety delay >= 300ms, got %v", elapsed)
			}
		})
	}
}

func TestSafetyDelay_ProgressNotificationsReceived(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, safetyDelayConfig(2*time.Second))
			c := tc.clientFunc(t, base)

			name := "e2e-sd-prog-" + strings.ToLower(tc.name[:3])
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": configMapManifest(name, testNamespace, map[string]string{"k": "v"}),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })

			progressCh := make(chan mcp.JSONRPCNotification, 10)
			c.OnNotification(func(n mcp.JSONRPCNotification) {
				if n.Method == "notifications/progress" {
					select {
					case progressCh <- n:
					default:
					}
				}
			})

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			req := mcp.CallToolRequest{}
			req.Params.Name = "patch_resource"
			req.Params.Arguments = map[string]any{
				"kind":      "ConfigMap",
				"name":      name,
				"namespace": testNamespace,
				"patch":     `{"data":{"b":"2"}}`,
				"patchType": "merge",
			}
			req.Params.Meta = &mcp.Meta{ProgressToken: "e2e-safety-delay-test"}

			result, err := c.CallTool(ctx, req)
			if err != nil {
				t.Fatalf("CallTool patch_resource: %v", err)
			}
			if result.IsError {
				t.Fatalf("patch_resource error: %s", resultText(result))
			}

			select {
			case n := <-progressCh:
				params, _ := n.Params.(map[string]any)
				msg, _ := params["message"].(string)
				if !strings.Contains(msg, "safety delay") {
					t.Errorf("expected progress message containing 'safety delay', got: %s", msg)
				}
			case <-time.After(5 * time.Second):
				t.Error("expected at least one progress notification within 5s, got none")
			}
		})
	}
}
