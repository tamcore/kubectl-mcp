package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

var testPodGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

func TestRunPod_HappyPath(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "run_pod", func(s *server.MCPServer) {
		registerRunPod(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "debug-pod",
		"image":     "busybox:latest",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Created Pod") {
		t.Errorf("expected creation confirmation, got: %s", text)
	}
	if !strings.Contains(text, "debug-pod") {
		t.Errorf("expected pod name, got: %s", text)
	}
}

func TestRunPod_WithCommand(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "run_pod", func(s *server.MCPServer) {
		registerRunPod(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "debug-pod",
		"image":     "busybox:latest",
		"command":   "sh -c 'echo hello'",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Created Pod") {
		t.Errorf("expected creation confirmation, got: %s", text)
	}

	// Verify the created Pod has the correct command.
	created, err := dynClient.Resource(testPodGVR).Namespace("default").Get(context.Background(), "debug-pod", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	spec := created.Object["spec"].(map[string]interface{})
	containers := spec["containers"].([]interface{})
	container := containers[0].(map[string]interface{})
	command := container["command"].([]interface{})
	if len(command) != 3 {
		t.Errorf("expected 3 command parts, got %d: %v", len(command), command)
	}
}

func TestRunPod_WithRestartPolicy(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "run_pod", func(s *server.MCPServer) {
		registerRunPod(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":     "default",
		"name":          "debug-pod",
		"image":         "busybox:latest",
		"restartPolicy": "OnFailure",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	// Check the actual pod object.
	created, err := dynClient.Resource(testPodGVR).Namespace("default").Get(context.Background(), "debug-pod", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	spec := created.Object["spec"].(map[string]interface{})
	if spec["restartPolicy"] != "OnFailure" {
		t.Errorf("expected restartPolicy=OnFailure, got %v", spec["restartPolicy"])
	}
}

func TestRunPod_DefaultRestartPolicyNever(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "run_pod", func(s *server.MCPServer) {
		registerRunPod(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "debug-pod",
		"image":     "busybox:latest",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	created, err := dynClient.Resource(testPodGVR).Namespace("default").Get(context.Background(), "debug-pod", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	spec := created.Object["spec"].(map[string]interface{})
	if spec["restartPolicy"] != "Never" {
		t.Errorf("expected default restartPolicy=Never, got %v", spec["restartPolicy"])
	}
}

func TestRunPod_InvalidRestartPolicy(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "run_pod", func(s *server.MCPServer) {
		registerRunPod(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":     "default",
		"name":          "debug-pod",
		"image":         "busybox:latest",
		"restartPolicy": "Invalid",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !res.IsError {
		t.Error("expected error for invalid restart policy")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "restartPolicy") {
		t.Errorf("expected restartPolicy error, got: %s", text)
	}
}

func TestRunPod_OutputContainsPodSpec(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "run_pod", func(s *server.MCPServer) {
		registerRunPod(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "debug-pod",
		"image":     "busybox:latest",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	// Output should contain JSON of the created pod.
	var result map[string]interface{}
	// Find the JSON part after the "Created Pod" message.
	idx := strings.Index(text, "{")
	if idx < 0 {
		t.Fatalf("expected JSON in output, got: %s", text)
	}
	if err := json.Unmarshal([]byte(text[idx:]), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s", text[idx:])
	}
	if result["kind"] != "Pod" {
		t.Errorf("expected kind=Pod, got %v", result["kind"])
	}
}

func TestRunPod_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}, AllowWrite: true}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "run_pod", func(s *server.MCPServer) {
		registerRunPod(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "debug-pod",
		"image":     "busybox:latest",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not allowed error, got: %s", text)
	}
}
