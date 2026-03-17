package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func testDeploymentWithStatus(name, ns string, replicas, ready, updated, available int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":              name,
			"namespace":         ns,
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"spec": map[string]interface{}{
			"replicas": replicas,
		},
		"status": map[string]interface{}{
			"replicas":          replicas,
			"readyReplicas":     ready,
			"updatedReplicas":   updated,
			"availableReplicas": available,
			"conditions": []interface{}{
				map[string]interface{}{
					"type":    "Available",
					"status":  "True",
					"reason":  "MinimumReplicasAvailable",
					"message": "Deployment has minimum availability.",
				},
				map[string]interface{}{
					"type":    "Progressing",
					"status":  "True",
					"reason":  "NewReplicaSetAvailable",
					"message": "ReplicaSet has successfully progressed.",
				},
			},
		},
	}}
}

func testStatefulSetWithStatus(name, ns string, replicas, ready, updated int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "StatefulSet",
		"metadata": map[string]interface{}{
			"name":              name,
			"namespace":         ns,
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"spec": map[string]interface{}{
			"replicas": replicas,
		},
		"status": map[string]interface{}{
			"replicas":        replicas,
			"readyReplicas":   ready,
			"updatedReplicas": updated,
		},
	}}
}

func testDaemonSetWithStatus(name, ns string, desired, ready, updated, available int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "DaemonSet",
		"metadata": map[string]interface{}{
			"name":              name,
			"namespace":         ns,
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"status": map[string]interface{}{
			"desiredNumberScheduled": desired,
			"numberReady":            ready,
			"updatedNumberScheduled": updated,
			"numberAvailable":        available,
		},
	}}
}

func TestRolloutStatus_DeploymentComplete(t *testing.T) {
	dep := testDeploymentWithStatus("my-deploy", "default", 3, 3, 3, 3)
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(dep)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_status", func(s *server.MCPServer) {
		registerRolloutStatus(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "my-deploy",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	text := resultText(t, res)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("expected JSON output, got: %s", text)
	}

	if result["complete"] != true {
		t.Errorf("expected complete=true, got %v", result["complete"])
	}
	if result["kind"] != "Deployment" {
		t.Errorf("expected kind=Deployment, got %v", result["kind"])
	}
	if result["name"] != "my-deploy" {
		t.Errorf("expected name=my-deploy, got %v", result["name"])
	}
}

func TestRolloutStatus_DeploymentInProgress(t *testing.T) {
	dep := testDeploymentWithStatus("my-deploy", "default", 3, 1, 2, 1)
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(dep)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_status", func(s *server.MCPServer) {
		registerRolloutStatus(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "my-deploy",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("expected JSON output, got: %s", text)
	}

	if result["complete"] != false {
		t.Errorf("expected complete=false, got %v", result["complete"])
	}
}

func TestRolloutStatus_StatefulSet(t *testing.T) {
	sts := testStatefulSetWithStatus("my-sts", "default", 3, 3, 3)
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(sts)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_status", func(s *server.MCPServer) {
		registerRolloutStatus(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "StatefulSet",
		"name":      "my-sts",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("expected JSON output, got: %s", text)
	}

	if result["complete"] != true {
		t.Errorf("expected complete=true, got %v", result["complete"])
	}
	if result["kind"] != "StatefulSet" {
		t.Errorf("expected kind=StatefulSet, got %v", result["kind"])
	}
}

func TestRolloutStatus_DaemonSet(t *testing.T) {
	ds := testDaemonSetWithStatus("my-ds", "default", 5, 5, 5, 5)
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(ds)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_status", func(s *server.MCPServer) {
		registerRolloutStatus(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "DaemonSet",
		"name":      "my-ds",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("expected JSON output, got: %s", text)
	}

	if result["complete"] != true {
		t.Errorf("expected complete=true, got %v", result["complete"])
	}
	if result["kind"] != "DaemonSet" {
		t.Errorf("expected kind=DaemonSet, got %v", result["kind"])
	}
}

func TestRolloutStatus_UnsupportedKind(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_status", func(s *server.MCPServer) {
		registerRolloutStatus(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "ReplicaSet",
		"name":      "test",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !res.IsError {
		t.Error("expected error for unsupported kind")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "does not support rollout status") {
		t.Errorf("expected unsupported error, got: %s", text)
	}
}

func TestRolloutStatus_NotFound(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_status", func(s *server.MCPServer) {
		registerRolloutStatus(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "nonexistent",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !res.IsError {
		t.Error("expected error for nonexistent resource")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "failed to get") {
		t.Errorf("expected not found error, got: %s", text)
	}
}

func TestRolloutStatus_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_status", func(s *server.MCPServer) {
		registerRolloutStatus(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "test",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not allowed error, got: %s", text)
	}
}
