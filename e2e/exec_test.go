//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestExecPod(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			// Create a busybox pod for exec tests.
			podName := "e2e-exec-" + strings.ToLower(tc.name)
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": podManifest(podName, testNamespace, "busybox:1.36", []string{"sleep", "3600"}),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "pod", podName, testNamespace) })
			waitForPodReady(t, podName, testNamespace)

			t.Run("write_and_read_file", func(t *testing.T) {
				// Write a file via exec.
				result := callTool(t, c, "exec_pod", map[string]any{
					"namespace": testNamespace,
					"pod":       podName,
					"command":   []any{"sh", "-c", "echo hello-e2e > /tmp/e2e-file"},
				})
				if result.IsError {
					t.Fatalf("write error: %s", resultText(result))
				}

				// Read it back via kubectl.
				out, err := kubectlOutput("exec", podName, "-n", testNamespace, "--", "cat", "/tmp/e2e-file")
				if err != nil {
					t.Fatalf("kubectl exec: %v", err)
				}
				if !strings.Contains(out, "hello-e2e") {
					t.Errorf("expected hello-e2e, got: %s", out)
				}
			})

			t.Run("read_command_output", func(t *testing.T) {
				result := callTool(t, c, "exec_pod", map[string]any{
					"namespace": testNamespace,
					"pod":       podName,
					"command":   []any{"echo", "test-output"},
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "test-output") {
					t.Errorf("expected test-output, got: %s", text)
				}
			})

			t.Run("string_command", func(t *testing.T) {
				result := callTool(t, c, "exec_pod", map[string]any{
					"namespace": testNamespace,
					"pod":       podName,
					"command":   "echo string-mode",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "string-mode") {
					t.Errorf("expected string-mode, got: %s", text)
				}
			})

			t.Run("nonexistent_pod_returns_error", func(t *testing.T) {
				result := callTool(t, c, "exec_pod", map[string]any{
					"namespace": testNamespace,
					"pod":       "no-such-pod-xyz",
					"command":   []any{"ls"},
				})
				if !result.IsError {
					t.Error("expected error for nonexistent pod")
				}
			})
		})
	}
}
