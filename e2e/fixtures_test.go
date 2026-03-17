//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const testNamespace = "e2e-test"

func TestMain(m *testing.M) {
	// Create test namespace.
	_ = kubectl("create", "namespace", testNamespace)

	code := m.Run()

	// Cleanup: delete test namespace (best-effort).
	_ = kubectl("delete", "namespace", testNamespace, "--wait=false", "--ignore-not-found")

	os.Exit(code)
}

func kubectl(args ...string) error {
	cmd := exec.Command("kubectl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func kubectlOutput(args ...string) (string, error) {
	cmd := exec.Command("kubectl", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func waitForPodReady(t *testing.T, name, namespace string) {
	t.Helper()
	err := kubectl("wait", "--for=condition=ready", fmt.Sprintf("pod/%s", name),
		"-n", namespace, "--timeout=120s")
	if err != nil {
		t.Fatalf("waiting for pod %s/%s ready: %v", namespace, name, err)
	}
}

func deleteViaKubectl(t *testing.T, kind, name, namespace string) {
	t.Helper()
	args := []string{"delete", kind, name, "--ignore-not-found", "--wait=false"}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	_ = kubectl(args...)
}

// jsonArrayFromResult extracts a JSON array from a tool result, handling
// the "Matched N of M" header that filter results include.
func jsonArrayFromResult(t *testing.T, text string) []map[string]any {
	t.Helper()
	jsonStart := strings.Index(text, "[")
	if jsonStart < 0 {
		t.Fatalf("expected JSON array in response, got: %s", text)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(text[jsonStart:]), &items); err != nil {
		t.Fatalf("failed to parse JSON array: %v\ntext: %s", err, text)
	}
	return items
}

func jsonObjectFromResult(t *testing.T, text string) map[string]any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		t.Fatalf("failed to parse JSON object: %v\ntext: %s", err, text)
	}
	return obj
}

// ---------------------------------------------------------------------------
// Manifest templates
// ---------------------------------------------------------------------------

func configMapManifest(name, namespace string, data map[string]string) string {
	dataJSON, _ := json.Marshal(data)
	return fmt.Sprintf(`{
		"apiVersion": "v1",
		"kind": "ConfigMap",
		"metadata": {"name": %q, "namespace": %q},
		"data": %s
	}`, name, namespace, string(dataJSON))
}

func secretManifest(name, namespace string, data map[string]string) string {
	dataJSON, _ := json.Marshal(data)
	return fmt.Sprintf(`{
		"apiVersion": "v1",
		"kind": "Secret",
		"metadata": {"name": %q, "namespace": %q},
		"type": "Opaque",
		"data": %s
	}`, name, namespace, string(dataJSON))
}

func podManifest(name, namespace, image string, command []string) string {
	cmdJSON, _ := json.Marshal(command)
	return fmt.Sprintf(`{
		"apiVersion": "v1",
		"kind": "Pod",
		"metadata": {"name": %q, "namespace": %q},
		"spec": {
			"containers": [{
				"name": "main",
				"image": %q,
				"command": %s
			}],
			"restartPolicy": "Never"
		}
	}`, name, namespace, image, string(cmdJSON))
}

func annotatedPodManifest(name, namespace string, annotations map[string]string) string {
	annJSON, _ := json.Marshal(annotations)
	return fmt.Sprintf(`{
		"apiVersion": "v1",
		"kind": "Pod",
		"metadata": {
			"name": %q,
			"namespace": %q,
			"annotations": %s
		},
		"spec": {
			"containers": [{
				"name": "main",
				"image": "busybox:1.36",
				"command": ["sleep", "3600"]
			}],
			"restartPolicy": "Never"
		}
	}`, name, namespace, string(annJSON))
}

func deploymentManifest(name, namespace string, replicas int) string {
	return fmt.Sprintf(`{
		"apiVersion": "apps/v1",
		"kind": "Deployment",
		"metadata": {"name": %q, "namespace": %q},
		"spec": {
			"replicas": %d,
			"selector": {"matchLabels": {"app": %q}},
			"template": {
				"metadata": {"labels": {"app": %q}},
				"spec": {
					"containers": [{
						"name": "nginx",
						"image": "nginx:1.27-alpine",
						"ports": [{"containerPort": 80}]
					}]
				}
			}
		}
	}`, name, namespace, replicas, name, name)
}

func statefulSetManifest(name, namespace string, replicas int) string {
	return fmt.Sprintf(`{
		"apiVersion": "apps/v1",
		"kind": "StatefulSet",
		"metadata": {"name": %q, "namespace": %q},
		"spec": {
			"replicas": %d,
			"serviceName": %q,
			"selector": {"matchLabels": {"app": %q}},
			"template": {
				"metadata": {"labels": {"app": %q}},
				"spec": {
					"containers": [{
						"name": "nginx",
						"image": "nginx:1.27-alpine"
					}]
				}
			}
		}
	}`, name, namespace, replicas, name, name, name)
}

func labeledPodManifest(name, namespace, image string, labels map[string]string, command []string) string {
	cmdJSON, _ := json.Marshal(command)
	labelsJSON, _ := json.Marshal(labels)
	return fmt.Sprintf(`{
		"apiVersion": "v1",
		"kind": "Pod",
		"metadata": {"name": %q, "namespace": %q, "labels": %s},
		"spec": {
			"containers": [{
				"name": "main",
				"image": %q,
				"command": %s
			}],
			"restartPolicy": "Never"
		}
	}`, name, namespace, string(labelsJSON), image, string(cmdJSON))
}

func deploymentManifestWithImage(name, namespace, image string, replicas int) string {
	return fmt.Sprintf(`{
		"apiVersion": "apps/v1",
		"kind": "Deployment",
		"metadata": {"name": %q, "namespace": %q},
		"spec": {
			"replicas": %d,
			"selector": {"matchLabels": {"app": %q}},
			"template": {
				"metadata": {"labels": {"app": %q}},
				"spec": {
					"containers": [{
						"name": "nginx",
						"image": %q,
						"ports": [{"containerPort": 80}]
					}]
				}
			}
		}
	}`, name, namespace, replicas, name, name, image)
}

func waitForDeploymentReady(t *testing.T, name, namespace string) {
	t.Helper()
	err := kubectl("rollout", "status", fmt.Sprintf("deployment/%s", name),
		"-n", namespace, "--timeout=120s")
	if err != nil {
		t.Fatalf("waiting for deployment %s/%s rollout: %v", namespace, name, err)
	}
}

