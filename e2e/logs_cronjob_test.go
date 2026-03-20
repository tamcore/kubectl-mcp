//go:build e2e

package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func cronJobManifest(name, namespace string) string {
	return fmt.Sprintf(`{
		"apiVersion": "batch/v1",
		"kind": "CronJob",
		"metadata": {"name": %q, "namespace": %q},
		"spec": {
			"schedule": "* * * * *",
			"successfulJobsHistoryLimit": 1,
			"failedJobsHistoryLimit": 1,
			"jobTemplate": {
				"spec": {
					"backoffLimit": 0,
					"template": {
						"spec": {
							"containers": [{
								"name": "main",
								"image": "busybox:1.36",
								"command": ["sh", "-c", "echo hello-from-cronjob && sleep 5"]
							}],
							"restartPolicy": "Never"
						}
					}
				}
			}
		}
	}`, name, namespace)
}

// waitForCronJobRun waits until at least one Job owned by the CronJob exists
// and has a running or succeeded pod.
func waitForCronJobRun(t *testing.T, name, namespace string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		out, err := kubectlOutput(
			"get", "jobs",
			"-n", namespace,
			"-l", fmt.Sprintf("batch.kubernetes.io/controller-uid"),
			"--sort-by=.metadata.creationTimestamp",
			"-o", "jsonpath={.items[?(@.metadata.ownerReferences[0].name==\""+name+"\")].metadata.name}",
		)
		if err == nil && strings.TrimSpace(out) != "" {
			// At least one job exists. Wait for a pod from it to be running.
			jobName := lastWord(out)
			podOut, _ := kubectlOutput(
				"get", "pods",
				"-n", namespace,
				"-l", fmt.Sprintf("job-name=%s", jobName),
				"-o", "jsonpath={.items[0].status.phase}",
			)
			if podOut == "Running" || podOut == "Succeeded" {
				return
			}
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("timed out waiting for CronJob %s/%s to spawn a running job", namespace, name)
}

func lastWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func TestGetLogs_CronJobResource(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			base := tc.startFunc(t, defaultConfig())
			c := tc.clientFunc(t, base)

			name := "e2e-logs-cj-" + strings.ToLower(tc.name)

			// Create the CronJob.
			callTool(t, c, "apply_resource", map[string]any{
				"manifest": cronJobManifest(name, testNamespace),
			})
			t.Cleanup(func() { deleteViaKubectl(t, "cronjob", name, testNamespace) })

			// Wait for at least one Job run to produce a pod.
			waitForCronJobRun(t, name, testNamespace)

			t.Run("cronjob_resource", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace": testNamespace,
					"resource":  "cronjob/" + name,
					"tail":      float64(50),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "hello-from-cronjob") {
					t.Errorf("expected log output from cronjob, got: %s", text)
				}
			})

			t.Run("cronjob_short_name", func(t *testing.T) {
				result := callTool(t, c, "get_logs", map[string]any{
					"namespace": testNamespace,
					"resource":  "cj/" + name,
					"tail":      float64(50),
				})
				text := resultText(result)
				if result.IsError {
					t.Fatalf("error: %s", text)
				}
				if !strings.Contains(text, "hello-from-cronjob") {
					t.Errorf("expected log output via short name, got: %s", text)
				}
			})
		})
	}
}
