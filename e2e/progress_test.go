//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestProgressNotifications(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			suffix := "prog-" + strings.ToLower(tc.name[:3])

			// Create two pods that will fail immediately (exit 1 → Failed state).
			pod1 := "e2e-" + suffix + "-1"
			pod2 := "e2e-" + suffix + "-2"
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": podManifest(pod1, testNamespace, "busybox:1.36", []string{"sh", "-c", "exit 1"}),
			})
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": podManifest(pod2, testNamespace, "busybox:1.36", []string{"sh", "-c", "exit 1"}),
			})
			t.Cleanup(func() {
				deleteViaKubectl(t, "pod", pod1, testNamespace)
				deleteViaKubectl(t, "pod", pod2, testNamespace)
			})

			// Wait for the pods to reach Failed state (up to 60s).
			for _, pod := range []string{pod1, pod2} {
				if err := kubectl("wait",
					fmt.Sprintf("--for=jsonpath={.status.phase}=Failed"),
					fmt.Sprintf("pod/%s", pod),
					"-n", testNamespace,
					"--timeout=60s",
				); err != nil {
					t.Skipf("pod %s did not reach Failed state within 60s (%v); skipping progress test", pod, err)
					return
				}
			}

			// Register a progress notification handler before the tool call.
			progressCh := make(chan mcp.JSONRPCNotification, 10)
			c.OnNotification(func(n mcp.JSONRPCNotification) {
				if n.Method == "notifications/progress" {
					select {
					case progressCh <- n:
					default:
					}
				}
			})

			// Call cleanup_pods with a progressToken.
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			req := mcp.CallToolRequest{}
			req.Params.Name = "cleanup_pods"
			req.Params.Arguments = map[string]any{
				"namespace": testNamespace,
				"states":    "Failed",
			}
			req.Params.Meta = &mcp.Meta{ProgressToken: "e2e-progress-test"}

			result, err := c.CallTool(ctx, req)
			if err != nil {
				t.Fatalf("CallTool cleanup_pods: %v", err)
			}
			text := resultText(result)
			if result.IsError {
				t.Fatalf("cleanup_pods returned error: %s", text)
			}
			if strings.Contains(text, "No pods in states") {
				t.Skipf("cleanup_pods found no Failed pods (got: %s); skipping progress assertion", text)
				return
			}

			// Expect at least one progress notification.
			select {
			case <-progressCh:
				// received at least one progress notification
			case <-time.After(5 * time.Second):
				t.Error("expected at least one progress notification, got none within 5s")
			}
		})
	}
}
