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

// testReplicaSet returns an unstructured ReplicaSet with revision annotation and owner reference.
func testReplicaSet(name, ns, revision, ownerName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "ReplicaSet",
		"metadata": map[string]interface{}{
			"name":              name,
			"namespace":         ns,
			"creationTimestamp": "2024-01-01T00:00:00Z",
			"annotations": map[string]interface{}{
				"deployment.kubernetes.io/revision": revision,
			},
			"ownerReferences": []interface{}{
				map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"name":       ownerName,
					"uid":        "deploy-uid-123",
				},
			},
		},
		"spec": map[string]interface{}{
			"replicas": int64(3),
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "web",
							"image": "nginx:1." + revision,
						},
					},
				},
			},
		},
		"status": map[string]interface{}{
			"replicas":      int64(3),
			"readyReplicas": int64(3),
		},
	}}
}

func TestRolloutHistory_DeploymentAllRevisions(t *testing.T) {
	dep := testDeployment("my-deploy", "default")
	rs1 := testReplicaSet("my-deploy-rs1", "default", "1", "my-deploy")
	rs2 := testReplicaSet("my-deploy-rs2", "default", "2", "my-deploy")
	rs3 := testReplicaSet("my-deploy-rs3", "default", "3", "my-deploy")

	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(dep, rs1, rs2, rs3)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_history", func(s *server.MCPServer) {
		registerRolloutHistory(s, pool)
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

	revisions, ok := result["revisions"].([]interface{})
	if !ok {
		t.Fatalf("expected revisions array, got: %v", result["revisions"])
	}
	if len(revisions) != 3 {
		t.Errorf("expected 3 revisions, got %d", len(revisions))
	}

	// Verify sorted by revision number.
	rev1 := revisions[0].(map[string]interface{})
	if rev1["revision"].(float64) != 1 {
		t.Errorf("expected first revision=1, got %v", rev1["revision"])
	}
	rev3 := revisions[2].(map[string]interface{})
	if rev3["revision"].(float64) != 3 {
		t.Errorf("expected last revision=3, got %v", rev3["revision"])
	}
}

func TestRolloutHistory_DeploymentSpecificRevision(t *testing.T) {
	dep := testDeployment("my-deploy", "default")
	rs1 := testReplicaSet("my-deploy-rs1", "default", "1", "my-deploy")
	rs2 := testReplicaSet("my-deploy-rs2", "default", "2", "my-deploy")

	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(dep, rs1, rs2)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_history", func(s *server.MCPServer) {
		registerRolloutHistory(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "my-deploy",
		"namespace": "default",
		"revision":  float64(2),
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

	if result["revision"].(float64) != 2 {
		t.Errorf("expected revision=2, got %v", result["revision"])
	}
	if result["replicaSet"] != "my-deploy-rs2" {
		t.Errorf("expected replicaSet=my-deploy-rs2, got %v", result["replicaSet"])
	}
}

func TestRolloutHistory_RevisionNotFound(t *testing.T) {
	dep := testDeployment("my-deploy", "default")
	rs1 := testReplicaSet("my-deploy-rs1", "default", "1", "my-deploy")

	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(dep, rs1)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_history", func(s *server.MCPServer) {
		registerRolloutHistory(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "my-deploy",
		"namespace": "default",
		"revision":  float64(99),
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !res.IsError {
		t.Error("expected error for nonexistent revision")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "revision 99 not found") {
		t.Errorf("expected revision not found error, got: %s", text)
	}
}

func TestRolloutHistory_UnsupportedKind(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_history", func(s *server.MCPServer) {
		registerRolloutHistory(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "StatefulSet",
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
	if !strings.Contains(text, "does not support rollout history") {
		t.Errorf("expected unsupported error, got: %s", text)
	}
}

func TestRolloutHistory_NoReplicaSetsFound(t *testing.T) {
	dep := testDeployment("my-deploy", "default")
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(dep)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_history", func(s *server.MCPServer) {
		registerRolloutHistory(s, pool)
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

	revisions := result["revisions"].([]interface{})
	if len(revisions) != 0 {
		t.Errorf("expected 0 revisions, got %d", len(revisions))
	}
}

func TestRolloutHistory_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_history", func(s *server.MCPServer) {
		registerRolloutHistory(s, pool)
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

func TestRolloutHistory_FiltersUnrelatedReplicaSets(t *testing.T) {
	dep := testDeployment("my-deploy", "default")
	rs1 := testReplicaSet("my-deploy-rs1", "default", "1", "my-deploy")
	rsOther := testReplicaSet("other-deploy-rs1", "default", "1", "other-deploy")

	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(dep, rs1, rsOther)
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "rollout_history", func(s *server.MCPServer) {
		registerRolloutHistory(s, pool)
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

	revisions := result["revisions"].([]interface{})
	if len(revisions) != 1 {
		t.Errorf("expected 1 revision (only owned by my-deploy), got %d", len(revisions))
	}
}
