//go:build e2e

package e2e

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCopyFromPod(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			podName := "e2e-cp-from-" + strings.ToLower(tc.name)
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": podManifest(podName, testNamespace, "busybox:1.36", []string{"sleep", "3600"}),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "pod", podName, testNamespace) })
			waitForPodReady(t, podName, testNamespace)

			t.Run("text_file", func(t *testing.T) {
				// Write a file via exec, then copy_from_pod and verify content.
				callTool(t, c, "exec_pod", map[string]any{
					"namespace": testNamespace,
					"pod":       podName,
					"command":   []any{"sh", "-c", "echo 'hello from pod' > /tmp/cp-test.txt"},
				})

				result := callTool(t, c, "copy_from_pod", map[string]any{
					"namespace": testNamespace,
					"pod":       podName,
					"src_path":  "/tmp/cp-test.txt",
				})
				if result.IsError {
					t.Fatalf("copy_from_pod error: %s", resultText(result))
				}
				text := resultText(result)
				if !strings.Contains(text, "hello from pod") {
					t.Errorf("expected file content, got: %s", text)
				}
				if !strings.Contains(text, "encoding: text") {
					t.Errorf("expected text encoding, got: %s", text)
				}
			})

			t.Run("nonexistent_file", func(t *testing.T) {
				result := callTool(t, c, "copy_from_pod", map[string]any{
					"namespace": testNamespace,
					"pod":       podName,
					"src_path":  "/tmp/does-not-exist.txt",
				})
				if !result.IsError {
					t.Errorf("expected error for nonexistent file, got: %s", resultText(result))
				}
			})
		})
	}
}

func TestCopyToPod(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowWrite = true
			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			podName := "e2e-cp-to-" + strings.ToLower(tc.name)
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": podManifest(podName, testNamespace, "busybox:1.36", []string{"sleep", "3600"}),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "pod", podName, testNamespace) })
			waitForPodReady(t, podName, testNamespace)

			t.Run("text_content", func(t *testing.T) {
				result := callTool(t, c, "copy_to_pod", map[string]any{
					"namespace": testNamespace,
					"pod":       podName,
					"dest_path": "/tmp/injected.txt",
					"content":   "injected content\n",
				})
				if result.IsError {
					t.Fatalf("copy_to_pod error: %s", resultText(result))
				}
				if !strings.Contains(resultText(result), "copied") {
					t.Errorf("expected success message, got: %s", resultText(result))
				}

				// Verify via exec.
				out, err := kubectlOutput("exec", podName, "-n", testNamespace, "--", "cat", "/tmp/injected.txt")
				if err != nil {
					t.Fatalf("kubectl exec: %v", err)
				}
				if !strings.Contains(out, "injected content") {
					t.Errorf("expected injected content, got: %s", out)
				}
			})

			t.Run("base64_content", func(t *testing.T) {
				binaryData := []byte{0x00, 0x01, 0x02, 0x03, 0xff}
				encoded := base64.StdEncoding.EncodeToString(binaryData)

				result := callTool(t, c, "copy_to_pod", map[string]any{
					"namespace": testNamespace,
					"pod":       podName,
					"dest_path": "/tmp/binary.bin",
					"content":   encoded,
					"encoding":  "base64",
				})
				if result.IsError {
					t.Fatalf("copy_to_pod base64 error: %s", resultText(result))
				}

				// Verify byte count via exec.
				out, err := kubectlOutput("exec", podName, "-n", testNamespace, "--", "wc", "-c", "/tmp/binary.bin")
				if err != nil {
					t.Fatalf("kubectl exec wc: %v", err)
				}
				if !strings.Contains(out, "5") {
					t.Errorf("expected 5 bytes, got: %s", out)
				}
			})

			t.Run("rejected_without_write", func(t *testing.T) {
				readonlyCfg := defaultConfig()
				readonlyCfg.AllowWrite = false
				readonlyBase := tc.startFunc(t, readonlyCfg)
				readonlyClient := tc.clientFunc(t, readonlyBase)

				result := callTool(t, readonlyClient, "copy_to_pod", map[string]any{
					"namespace": testNamespace,
					"pod":       podName,
					"dest_path": "/tmp/test.txt",
					"content":   "data",
				})
				if !result.IsError {
					t.Errorf("expected copy_to_pod to be rejected without --allow-write")
				}
			})
		})
	}
}
