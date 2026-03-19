//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestGetLogsLabelSelector(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			suffix := strings.ToLower(tc.name)
			labelKey := "e2e-logs-group"
			labelVal := "test-" + suffix

			// Create two labeled pods that print a known message.
			pod1 := "e2e-logs-lbl1-" + suffix
			pod2 := "e2e-logs-lbl2-" + suffix
			labels := map[string]string{labelKey: labelVal}

			manifest1 := labeledPodManifest(pod1, testNamespace, "busybox:1.36", labels,
				[]string{"sh", "-c", "echo hello-from-pod1; sleep 3600"})
			manifest2 := labeledPodManifest(pod2, testNamespace, "busybox:1.36", labels,
				[]string{"sh", "-c", "echo hello-from-pod2; sleep 3600"})

			callTool(t, c, "apply_resource", map[string]any{"manifest": manifest1})
			callTool(t, c, "apply_resource", map[string]any{"manifest": manifest2})
			t.Cleanup(func() {
				deleteViaKubectl(t, "pod", pod1, testNamespace)
				deleteViaKubectl(t, "pod", pod2, testNamespace)
			})
			waitForPodReady(t, pod1, testNamespace)
			waitForPodReady(t, pod2, testNamespace)

			t.Run("aggregated_logs_with_prefix", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace":     testNamespace,
					"labelSelector": labelKey + "=" + labelVal,
					"tail":          float64(10),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				// Each line should be prefixed with [podName].
				if !strings.Contains(text, "["+pod1+"]") && !strings.Contains(text, "["+pod2+"]") {
					t.Errorf("expected pod name prefixes in aggregated logs, got: %s", text)
				}
				if !strings.Contains(text, "hello-from-pod1") || !strings.Contains(text, "hello-from-pod2") {
					t.Errorf("expected logs from both pods, got: %s", text)
				}
			})

			t.Run("no_matching_pods_returns_error", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace":     testNamespace,
					"labelSelector": "no-such-label=xyz",
				})
				if !result.IsError {
					t.Error("expected error when no pods match the label selector")
				}
			})
		})
	}
}

func TestGetLogsTimestamps(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			suffix := strings.ToLower(tc.name)
			podName := "e2e-logs-ts-" + suffix

			manifest := podManifest(podName, testNamespace, "busybox:1.36",
				[]string{"sh", "-c", "echo timestamped-line; sleep 3600"})
			callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
			t.Cleanup(func() { deleteViaKubectl(t, "pod", podName, testNamespace) })
			waitForPodReady(t, podName, testNamespace)

			result := callTool(t, c, "get_logs", map[string]any{
				"namespace":  testNamespace,
				"pod":        podName,
				"timestamps": true,
				"tail":       float64(5),
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("error: %s", text)
			}
			// RFC3339 timestamps look like "2024-01-15T10:00:00.123456789Z".
			// Check that there's a date-like prefix.
			if !strings.Contains(text, "T") || !strings.Contains(text, "Z") {
				t.Errorf("expected RFC3339 timestamp in output, got: %s", text)
			}
		})
	}
}

func TestGetLogsMutualExclusion(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("since_and_sinceTime_returns_error", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace": "kube-system",
					"pod":       "kube-apiserver-e2e-control-plane",
					"since":     "1h",
					"sinceTime": "2024-01-15T10:00:00Z",
				})
				if !result.IsError {
					t.Error("expected error when both since and sinceTime are provided")
				}
				text := resultText(result)
				if !strings.Contains(text, "mutually exclusive") {
					t.Errorf("expected 'mutually exclusive' error, got: %s", text)
				}
			})

			t.Run("invalid_sinceTime_format_returns_error", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace": "kube-system",
					"pod":       "kube-apiserver-e2e-control-plane",
					"sinceTime": "not-a-timestamp",
				})
				if !result.IsError {
					t.Error("expected error for invalid sinceTime format")
				}
				text := resultText(result)
				if !strings.Contains(text, "invalid sinceTime") {
					t.Errorf("expected 'invalid sinceTime' error, got: %s", text)
				}
			})

			t.Run("pod_or_labelSelector_required", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace": testNamespace,
				})
				if !result.IsError {
					t.Error("expected error when neither pod nor labelSelector is provided")
				}
				text := resultText(result)
				if !strings.Contains(text, "pod") && !strings.Contains(text, "labelSelector") {
					t.Errorf("expected error about missing pod/labelSelector, got: %s", text)
				}
			})

			t.Run("follow_and_tail_conflict", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace": testNamespace,
					"pod":       "any-pod",
					"follow":    true,
					"tail":      float64(10),
				})
				if !result.IsError {
					t.Error("expected error when follow=true and tail are both set")
				}
				text := resultText(result)
				if !strings.Contains(text, "follow") || !strings.Contains(text, "tail") {
					t.Errorf("expected error mentioning follow and tail, got: %s", text)
				}
			})

			t.Run("follow_timeout_out_of_range_low", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace":     testNamespace,
					"pod":           "any-pod",
					"follow":        true,
					"followTimeout": float64(0),
				})
				if !result.IsError {
					t.Error("expected error when followTimeout=0")
				}
				text := resultText(result)
				if !strings.Contains(text, "followTimeout") {
					t.Errorf("expected error mentioning followTimeout, got: %s", text)
				}
			})

			t.Run("follow_timeout_out_of_range_high", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace":     testNamespace,
					"pod":           "any-pod",
					"follow":        true,
					"followTimeout": float64(200),
				})
				if !result.IsError {
					t.Error("expected error when followTimeout=200")
				}
				text := resultText(result)
				if !strings.Contains(text, "followTimeout") {
					t.Errorf("expected error mentioning followTimeout, got: %s", text)
				}
			})
		})
	}
}

func TestGetLogsFollow(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			suffix := strings.ToLower(tc.name)
			podName := "e2e-logs-follow-" + suffix

			// Pod that prints a line then exits — follow should capture it.
			manifest := podManifest(podName, testNamespace, "busybox:1.36",
				[]string{"sh", "-c", "echo follow-output-line; sleep 1"})
			callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
			t.Cleanup(func() { deleteViaKubectl(t, "pod", podName, testNamespace) })
			waitForPodReady(t, podName, testNamespace)

			t.Run("follow_returns_output", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace":     testNamespace,
					"pod":           podName,
					"follow":        true,
					"followTimeout": float64(10),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("unexpected error: %s", text)
				}
				if !strings.Contains(text, "follow-output-line") {
					t.Errorf("expected 'follow-output-line' in output, got: %s", text)
				}
			})
		})
	}
}
