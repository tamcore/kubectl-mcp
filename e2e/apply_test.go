//go:build e2e

package e2e

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestApply(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			t.Run("create_configmap", func(t *testing.T) {
				manifest := configMapManifest("e2e-apply-cm", testNamespace, map[string]string{"key1": "value1"})
				result := callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Applied ConfigMap/e2e-apply-cm") {
					t.Errorf("expected apply confirmation, got: %s", text)
				}

				// Cross-validate with kubectl.
				out, err := kubectlOutput("get", "configmap", "e2e-apply-cm", "-n", testNamespace,
					"-o", "jsonpath={.data.key1}")
				if err != nil {
					t.Fatalf("kubectl get: %v", err)
				}
				if out != "value1" {
					t.Errorf("expected value1, got: %s", out)
				}

				t.Cleanup(func() { deleteViaKubectl(t, "configmap", "e2e-apply-cm", testNamespace) })
			})

			t.Run("create_deployment", func(t *testing.T) {
				manifest := deploymentManifest("e2e-apply-deploy", testNamespace, 1)
				result := callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Applied Deployment/e2e-apply-deploy") {
					t.Errorf("expected apply confirmation, got: %s", text)
				}

				out, err := kubectlOutput("get", "deployment", "e2e-apply-deploy", "-n", testNamespace,
					"-o", "jsonpath={.spec.replicas}")
				if err != nil {
					t.Fatalf("kubectl get: %v", err)
				}
				if out != "1" {
					t.Errorf("expected 1 replica, got: %s", out)
				}

				t.Cleanup(func() { deleteViaKubectl(t, "deployment", "e2e-apply-deploy", testNamespace) })
			})

			t.Run("create_statefulset", func(t *testing.T) {
				manifest := statefulSetManifest("e2e-apply-sts", testNamespace, 1)
				result := callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Applied StatefulSet/e2e-apply-sts") {
					t.Errorf("expected apply confirmation, got: %s", text)
				}

				t.Cleanup(func() { deleteViaKubectl(t, "statefulset", "e2e-apply-sts", testNamespace) })
			})

			t.Run("create_secret_redacted", func(t *testing.T) {
				secret64 := base64.StdEncoding.EncodeToString([]byte("supersecret"))
				manifest := secretManifest("e2e-apply-secret", testNamespace, map[string]string{"password": secret64})
				result := callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Applied Secret/e2e-apply-secret") {
					t.Errorf("expected apply confirmation, got: %s", text)
				}
				if !strings.Contains(text, "redacted") {
					t.Error("expected secret data to be redacted in response")
				}
				if strings.Contains(text, "supersecret") || strings.Contains(text, secret64) {
					t.Error("secret value leaked in response")
				}

				t.Cleanup(func() { deleteViaKubectl(t, "secret", "e2e-apply-secret", testNamespace) })
			})

			t.Run("create_pod", func(t *testing.T) {
				podName := "e2e-apply-pod-" + strings.ToLower(tc.name)
				manifest := podManifest(podName, testNamespace, "busybox:1.36", []string{"sleep", "3600"})
				result := callTool(t, c, "apply_resource", map[string]any{"manifest": manifest})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "Applied Pod/"+podName) {
					t.Errorf("expected apply confirmation, got: %s", text)
				}

				waitForPodReady(t, podName, testNamespace)

				t.Cleanup(func() { deleteViaKubectl(t, "pod", podName, testNamespace) })
			})

			t.Run("update_existing", func(t *testing.T) {
				name := "e2e-apply-update"
				manifest1 := configMapManifest(name, testNamespace, map[string]string{"k": "v1"})
				callTool(t, c, "apply_resource", map[string]any{"manifest": manifest1})

				manifest2 := configMapManifest(name, testNamespace, map[string]string{"k": "v2"})
				result := callTool(t, c, "apply_resource", map[string]any{"manifest": manifest2})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}

				out, err := kubectlOutput("get", "configmap", name, "-n", testNamespace,
					"-o", "jsonpath={.data.k}")
				if err != nil {
					t.Fatalf("kubectl get: %v", err)
				}
				if out != "v2" {
					t.Errorf("expected v2, got: %s", out)
				}

				t.Cleanup(func() { deleteViaKubectl(t, "configmap", name, testNamespace) })
			})

			t.Run("yaml_manifest", func(t *testing.T) {
				yamlManifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: e2e-apply-yaml
  namespace: ` + testNamespace + `
data:
  foo: bar
`
				result := callTool(t, c, "apply_resource", map[string]any{"manifest": yamlManifest})
				if result.IsError {
					t.Fatalf("error: %s", resultText(result))
				}

				out, err := kubectlOutput("get", "configmap", "e2e-apply-yaml", "-n", testNamespace,
					"-o", "jsonpath={.data.foo}")
				if err != nil {
					t.Fatalf("kubectl get: %v", err)
				}
				if out != "bar" {
					t.Errorf("expected bar, got: %s", out)
				}

				t.Cleanup(func() { deleteViaKubectl(t, "configmap", "e2e-apply-yaml", testNamespace) })
			})

			t.Run("invalid_manifest_returns_error", func(t *testing.T) {
				result := callTool(t, c, "apply_resource", map[string]any{"manifest": "not valid yaml"})
				if !result.IsError {
					t.Error("expected error for invalid manifest")
				}
			})
		})
	}
}

func TestApply_RejectedWithoutAllowWrite(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.AllowWrite = false
			cfg.AllowDestructive = false

			base := tc.startFunc(t, cfg)
			c := tc.clientFunc(t, base)

			// apply_resource should not be registered — callToolMayFail handles the SDK error.
			_, err := callToolMayFail(t, c, "apply_resource", map[string]any{
				"manifest": configMapManifest("should-not-exist", testNamespace, map[string]string{"k": "v"}),
			})
			if err == nil {
				t.Error("expected error — apply_resource should not be registered without --allow-write")
			}
			if !strings.Contains(err.Error(), "not found") {
				t.Errorf("expected 'not found' error, got: %v", err)
			}
		})
	}
}
