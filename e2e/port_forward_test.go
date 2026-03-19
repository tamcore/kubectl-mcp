//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestPortForward(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("remote_port_zero_returns_error", func(t *testing.T) {
				result := callTool(t, c, "port_forward", map[string]any{
					"namespace":  testNamespace,
					"resource":   "any-pod",
					"remotePort": float64(0),
				})
				if !result.IsError {
					t.Error("expected error for remotePort=0")
				}
				text := resultText(result)
				if !strings.Contains(text, "remotePort must be a valid port number") {
					t.Errorf("expected port validation error, got: %s", text)
				}
			})

			t.Run("nonexistent_pod", func(t *testing.T) {
				result := callTool(t, c, "port_forward", map[string]any{
					"namespace":  testNamespace,
					"resource":   "no-such-pod-xyz",
					"remotePort": float64(8080),
				})
				if !result.IsError {
					t.Error("expected error for nonexistent pod")
				}
			})

			t.Run("nonexistent_pod_with_localPort_and_timeout", func(t *testing.T) {
				result := callTool(t, c, "port_forward", map[string]any{
					"namespace":  testNamespace,
					"resource":   "no-such-pod-xyz",
					"remotePort": float64(8080),
					"localPort":  float64(9090),
					"timeout":    float64(5),
				})
				if !result.IsError {
					t.Error("expected error for nonexistent pod")
				}
			})

			// Backwards compatibility: bare name (no slash) treated as Pod.
			t.Run("resource_as_bare_pod_name", func(t *testing.T) {
				result := callTool(t, c, "port_forward", map[string]any{
					"namespace":  testNamespace,
					"resource":   "no-such-pod-xyz",
					"remotePort": float64(8080),
				})
				// Should attempt pod resolution (and fail), not an unknown-kind error.
				if !result.IsError {
					t.Error("expected error for nonexistent pod via bare name")
				}
				text := resultText(result)
				if strings.Contains(text, "unsupported") {
					t.Errorf("bare name should not produce unsupported-kind error, got: %s", text)
				}
			})

			// Explicit "pod/" prefix works.
			t.Run("resource_as_pod_kind", func(t *testing.T) {
				result := callTool(t, c, "port_forward", map[string]any{
					"namespace":  testNamespace,
					"resource":   "pod/no-such-pod-xyz",
					"remotePort": float64(8080),
				})
				if !result.IsError {
					t.Error("expected error for nonexistent pod")
				}
				text := resultText(result)
				if strings.Contains(text, "unsupported") {
					t.Errorf("pod/ prefix should not produce unsupported-kind error, got: %s", text)
				}
			})

			// Unknown kind returns a clear error.
			t.Run("invalid_kind_returns_error", func(t *testing.T) {
				result := callTool(t, c, "port_forward", map[string]any{
					"namespace":  testNamespace,
					"resource":   "job/my-job",
					"remotePort": float64(8080),
				})
				if !result.IsError {
					t.Error("expected error for unknown kind 'job'")
				}
				text := resultText(result)
				if !strings.Contains(text, "unsupported") {
					t.Errorf("expected unsupported-kind error, got: %s", text)
				}
			})

			// Deploy an nginx pod for real port-forward tests.
			podName := "e2e-pf-" + strings.ToLower(tc.name)
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": nginxPodManifest(podName, testNamespace),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "pod", podName, testNamespace) })
			waitForPodReady(t, podName, testNamespace)

			t.Run("forward_auto_port_and_stop", func(t *testing.T) {
				result := callTool(t, c, "port_forward", map[string]any{
					"namespace":  testNamespace,
					"resource":   podName,
					"remotePort": float64(80),
					"timeout":    float64(60),
				})
				if result.IsError {
					t.Fatalf("port_forward failed: %s", resultText(result))
				}

				text := resultText(result)
				var resp map[string]any
				if err := json.Unmarshal([]byte(text), &resp); err != nil {
					t.Fatalf("expected JSON response, got: %s", text)
				}

				localPort := resp["localPort"].(float64)
				if localPort == 0 {
					t.Fatal("expected non-zero localPort from auto-assign")
				}
				if resp["remotePort"].(float64) != 80 {
					t.Errorf("expected remotePort=80, got %v", resp["remotePort"])
				}
				if resp["pod"] != podName {
					t.Errorf("expected pod=%s, got %v", podName, resp["pod"])
				}

				// Verify we can connect to the forwarded port.
				addr := fmt.Sprintf("127.0.0.1:%d", int(localPort))
				conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
				if err != nil {
					t.Fatalf("failed to connect to forwarded port %s: %v", addr, err)
				}
				_ = conn.Close()

				// Stop the forward via stop_port_forward.
				sessionID := fmt.Sprintf("%s/%s/%d", testNamespace, podName, int(localPort))
				stopResult := callTool(t, c, "stop_port_forward", map[string]any{
					"sessionId": sessionID,
				})
				if stopResult.IsError {
					t.Fatalf("stop_port_forward failed: %s", resultText(stopResult))
				}
				stopText := resultText(stopResult)
				if !strings.Contains(stopText, "Stopped") {
					t.Errorf("expected stop confirmation, got: %s", stopText)
				}

				// Verify port is no longer listening (may take a moment to close).
				time.Sleep(500 * time.Millisecond)
				conn, err = net.DialTimeout("tcp", addr, 2*time.Second)
				if err == nil {
					_ = conn.Close()
					t.Error("expected connection to fail after stopping port forward")
				}
			})

			t.Run("forward_explicit_local_port", func(t *testing.T) {
				// Find a free port to use as explicit local port.
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatalf("failed to find free port: %v", err)
				}
				freePort := ln.Addr().(*net.TCPAddr).Port
				_ = ln.Close()

				result := callTool(t, c, "port_forward", map[string]any{
					"namespace":  testNamespace,
					"resource":   podName,
					"remotePort": float64(80),
					"localPort":  float64(freePort),
					"timeout":    float64(30),
				})
				if result.IsError {
					t.Fatalf("port_forward failed: %s", resultText(result))
				}

				text := resultText(result)
				var resp map[string]any
				if err := json.Unmarshal([]byte(text), &resp); err != nil {
					t.Fatalf("expected JSON response, got: %s", text)
				}

				if int(resp["localPort"].(float64)) != freePort {
					t.Errorf("expected localPort=%d, got %v", freePort, resp["localPort"])
				}

				// Verify connectivity.
				addr := fmt.Sprintf("127.0.0.1:%d", freePort)
				conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
				if err != nil {
					t.Fatalf("failed to connect to forwarded port %s: %v", addr, err)
				}
				_ = conn.Close()

				// Clean up.
				sessionID := fmt.Sprintf("%s/%s/%d", testNamespace, podName, freePort)
				callTool(t, c, "stop_port_forward", map[string]any{
					"sessionId": sessionID,
				})
			})
		})
	}
}

func TestPortForward_RejectedWithoutAllowWrite(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowWrite = false
			cfg.AllowDestructive = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			_, err := callToolMayFail(t, c, "port_forward", map[string]any{
				"namespace":  testNamespace,
				"resource":   "any-pod",
				"remotePort": float64(8080),
			})
			if err == nil {
				t.Error("expected error -- port_forward should not be registered without --allow-write")
			}
			if !strings.Contains(err.Error(), "not found") {
				t.Errorf("expected 'not found' error, got: %v", err)
			}
		})
	}
}

// nginxPodManifest returns a pod manifest with nginx listening on port 80.
func nginxPodManifest(name, namespace string) string {
	return fmt.Sprintf(`{
		"apiVersion": "v1",
		"kind": "Pod",
		"metadata": {"name": %q, "namespace": %q},
		"spec": {
			"containers": [{
				"name": "nginx",
				"image": "nginx:1.27-alpine",
				"ports": [{"containerPort": 80}]
			}],
			"restartPolicy": "Never"
		}
	}`, name, namespace)
}
