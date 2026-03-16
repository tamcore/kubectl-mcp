//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestListContexts(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			result := callTool(t, c, "list_contexts", nil)
			text := resultText(result)

			if text == "" {
				t.Fatal("list_contexts returned empty result")
			}
			if !strings.Contains(text, "kind-e2e") {
				t.Errorf("expected kind-e2e context, got: %s", text)
			}
		})
	}
}

func TestListNamespaces(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			result := callTool(t, c, "list_namespaces", nil)
			text := resultText(result)

			if !strings.Contains(text, "default") || !strings.Contains(text, "kube-system") {
				t.Errorf("expected default and kube-system namespaces, got: %s", text)
			}
		})
	}
}

func TestListAPIResources(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			result := callTool(t, c, "list_api_resources", nil)
			text := resultText(result)

			if result.IsError {
				t.Fatalf("error: %s", text)
			}

			var items []map[string]any
			if err := json.Unmarshal([]byte(text), &items); err != nil {
				t.Fatalf("expected JSON array: %v", err)
			}

			// Check for well-known kinds.
			kinds := make(map[string]bool)
			for _, item := range items {
				if k, ok := item["kind"].(string); ok {
					kinds[k] = true
				}
			}
			for _, expected := range []string{"Pod", "Deployment", "Service", "Namespace"} {
				if !kinds[expected] {
					t.Errorf("expected kind %q in api_resources", expected)
				}
			}
		})
	}
}

func TestDescribeResource(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("namespace", func(t *testing.T) {
				result := callTool(t, c, "describe_resource", map[string]any{
					"kind": "Namespace",
					"name": "default",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Name:         default") {
					t.Errorf("expected name in output, got: %s", text)
				}
				if !strings.Contains(text, "Kind:         Namespace") {
					t.Errorf("expected kind in output, got: %s", text)
				}
			})

			t.Run("nonexistent_returns_error", func(t *testing.T) {
				result := callTool(t, c, "describe_resource", map[string]any{
					"kind":      "Pod",
					"name":      "no-such-pod-xyz",
					"namespace": "default",
				})
				if !result.IsError {
					t.Error("expected error for nonexistent pod")
				}
			})
		})
	}
}

func TestGetLogs(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			// Find a running pod in kube-system.
			listResult := callTool(t, c, "list_resources", map[string]any{
				"kind":      "Pod",
				"namespace": "kube-system",
				"filter":    "status.phase=Running",
			})
			items := jsonArrayFromResult(t, resultText(listResult))
			if len(items) == 0 {
				t.Fatal("no running pods in kube-system")
			}
			podName := items[0]["name"].(string)

			t.Run("kube_system_pod", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace": "kube-system",
					"pod":       podName,
					"tail":      float64(5),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if text == "(no logs)" || text == "" {
					t.Log("pod has no logs (may be expected for some system pods)")
				}
			})

			t.Run("with_since", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace": "kube-system",
					"pod":       podName,
					"tail":      float64(3),
					"since":     "1h",
				})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}
			})

			t.Run("nonexistent_pod_returns_error", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace": "default",
					"pod":       "no-such-pod-xyz",
				})
				if !result.IsError {
					t.Error("expected error for nonexistent pod")
				}
			})
		})
	}
}

func TestGetEvents(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("kube_system", func(t *testing.T) {
				result := callTool(t, c, "get_events", map[string]any{
					"namespace": "kube-system",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				// Either "No events found" or valid JSON array.
				if text != "No events found" {
					var items []map[string]any
					if err := json.Unmarshal([]byte(text), &items); err != nil {
						t.Fatalf("expected JSON array: %v", err)
					}
				}
			})

			t.Run("with_limit", func(t *testing.T) {
				result := callTool(t, c, "get_events", map[string]any{
					"namespace": "kube-system",
					"limit":     float64(2),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if text != "No events found" {
					items := jsonArrayFromResult(t, text)
					if len(items) > 2 {
						t.Errorf("expected <= 2 events, got %d", len(items))
					}
				}
			})
		})
	}
}

func TestGetResource(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			result := callTool(t, c, "get_resource", map[string]any{
				"kind": "Namespace",
				"name": "default",
			})
			text := resultText(result)
			if result.IsError {
				t.Fatalf("error: %s", text)
			}

			obj := jsonObjectFromResult(t, text)
			if obj["kind"] != "Namespace" {
				t.Errorf("expected kind=Namespace, got %v", obj["kind"])
			}
		})
	}
}

func TestListResources(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("pods_in_kube_system", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":      "Pod",
					"namespace": "kube-system",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				items := jsonArrayFromResult(t, text)
				if len(items) == 0 {
					t.Error("expected at least one pod in kube-system")
				}
			})

			t.Run("with_filter", func(t *testing.T) {
				result := callTool(t, c, "list_resources", map[string]any{
					"kind":      "Pod",
					"namespace": "kube-system",
					"filter":    "status.phase=Running",
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				items := jsonArrayFromResult(t, text)
				for _, item := range items {
					if status, ok := item["status"].(string); ok && status != "Running" {
						t.Errorf("expected Running pod, got status=%s", status)
					}
				}
			})
		})
	}
}
