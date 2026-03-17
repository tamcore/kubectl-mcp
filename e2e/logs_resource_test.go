//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestGetLogs_Resource(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			name := "e2e-logs-res-" + strings.ToLower(tc.name)

			// Create a deployment with a container that produces logs.
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": deploymentManifestWithImage(name, testNamespace, "busybox:1.36", 1),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "deployment", name, testNamespace) })

			// Patch the deployment to use a log-producing command.
			callTool(t, c, "patch_resource", map[string]any{
				"kind":      "Deployment",
				"name":      name,
				"namespace": testNamespace,
				"patch":     `{"spec":{"template":{"spec":{"containers":[{"name":"nginx","image":"busybox:1.36","command":["sh","-c","echo hello-from-resource && sleep 3600"]}]}}}}`,
				"strategy":  "merge",
			})

			// Wait for the deployment to be ready.
			waitForDeploymentReady(t, name, testNamespace)

			t.Run("deployment_resource", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace": testNamespace,
					"resource":  "deployment/" + name,
					"tail":      float64(10),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "hello-from-resource") {
					t.Errorf("expected log output from deployment, got: %s", text)
				}
			})

			t.Run("mutually_exclusive_pod_and_resource", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace": testNamespace,
					"pod":       "any",
					"resource":  "deployment/" + name,
				})
				if !result.IsError {
					t.Error("expected error for mutually exclusive parameters")
				}
				text := resultText(result)
				if !strings.Contains(text, "mutually exclusive") {
					t.Errorf("expected mutual exclusion error, got: %s", text)
				}
			})

			t.Run("unsupported_resource_kind", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace": testNamespace,
					"resource":  "configmap/test",
				})
				if !result.IsError {
					t.Error("expected error for unsupported resource kind")
				}
				text := resultText(result)
				if !strings.Contains(text, "not supported for log resolution") {
					t.Errorf("expected unsupported error, got: %s", text)
				}
			})
		})
	}
}
