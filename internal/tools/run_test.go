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
		registerRunPod(s, pool, cfg)
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
		registerRunPod(s, pool, cfg)
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
	spec := created.Object["spec"].(map[string]any)
	containers := spec["containers"].([]any)
	container := containers[0].(map[string]any)
	command := container["command"].([]any)
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
		registerRunPod(s, pool, cfg)
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
	spec := created.Object["spec"].(map[string]any)
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
		registerRunPod(s, pool, cfg)
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
	spec := created.Object["spec"].(map[string]any)
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
		registerRunPod(s, pool, cfg)
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
		registerRunPod(s, pool, cfg)
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
	var result map[string]any
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
		registerRunPod(s, pool, cfg)
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

// ---------------------------------------------------------------------------
// ports parameter tests
// ---------------------------------------------------------------------------

func TestRunPod_WithSinglePort(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "run_pod", func(s *server.MCPServer) {
		registerRunPod(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "nginx-pod",
		"image":     "nginx:latest",
		"ports":     "80",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	created, err := dynClient.Resource(testPodGVR).Namespace("default").Get(context.Background(), "nginx-pod", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	spec := created.Object["spec"].(map[string]any)
	containers := spec["containers"].([]any)
	container := containers[0].(map[string]any)
	ports, ok := container["ports"].([]any)
	if !ok || len(ports) != 1 {
		t.Fatalf("expected 1 port, got: %v", container["ports"])
	}
	p := ports[0].(map[string]any)
	if p["containerPort"] != int64(80) {
		t.Errorf("expected containerPort=80, got: %v", p["containerPort"])
	}
	if p["protocol"] != "TCP" {
		t.Errorf("expected protocol=TCP, got: %v", p["protocol"])
	}
}

func TestRunPod_WithMultiplePorts(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "run_pod", func(s *server.MCPServer) {
		registerRunPod(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "multi-port-pod",
		"image":     "nginx:latest",
		"ports":     "80,443",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	created, err := dynClient.Resource(testPodGVR).Namespace("default").Get(context.Background(), "multi-port-pod", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	spec := created.Object["spec"].(map[string]any)
	containers := spec["containers"].([]any)
	container := containers[0].(map[string]any)
	ports := container["ports"].([]any)
	if len(ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(ports))
	}
	p0 := ports[0].(map[string]any)
	p1 := ports[1].(map[string]any)
	if p0["containerPort"] != int64(80) {
		t.Errorf("expected first port=80, got: %v", p0["containerPort"])
	}
	if p1["containerPort"] != int64(443) {
		t.Errorf("expected second port=443, got: %v", p1["containerPort"])
	}
}

func TestRunPod_WithProtocol(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "run_pod", func(s *server.MCPServer) {
		registerRunPod(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "udp-pod",
		"image":     "nginx:latest",
		"ports":     "8080/TCP,9090/UDP",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	created, err := dynClient.Resource(testPodGVR).Namespace("default").Get(context.Background(), "udp-pod", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	spec := created.Object["spec"].(map[string]any)
	containers := spec["containers"].([]any)
	container := containers[0].(map[string]any)
	ports := container["ports"].([]any)
	if len(ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(ports))
	}
	p0 := ports[0].(map[string]any)
	p1 := ports[1].(map[string]any)
	if p0["protocol"] != "TCP" {
		t.Errorf("expected protocol=TCP, got: %v", p0["protocol"])
	}
	if p1["protocol"] != "UDP" {
		t.Errorf("expected protocol=UDP, got: %v", p1["protocol"])
	}
}

func TestRunPod_InvalidPort_OutOfRange(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "run_pod", func(s *server.MCPServer) {
		registerRunPod(s, pool, cfg)
	})

	for _, badPort := range []string{"0", "65536", "99999"} {
		t.Run(badPort, func(t *testing.T) {
			res, err := handler(context.Background(), callToolReq(map[string]any{
				"namespace": "default",
				"name":      "bad-port-pod",
				"image":     "nginx:latest",
				"ports":     badPort,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if !res.IsError {
				t.Errorf("expected error for port %s, got success", badPort)
			}
			text := resultText(t, res)
			if !strings.Contains(text, "invalid port") {
				t.Errorf("expected 'invalid port' error for %s, got: %s", badPort, text)
			}
		})
	}
}

func TestRunPod_InvalidPort_NotANumber(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "run_pod", func(s *server.MCPServer) {
		registerRunPod(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "bad-port-pod",
		"image":     "nginx:latest",
		"ports":     "abc",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for non-numeric port")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "invalid port") {
		t.Errorf("expected 'invalid port' error, got: %s", text)
	}
}

func TestRunPod_InvalidProtocol(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "run_pod", func(s *server.MCPServer) {
		registerRunPod(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "bad-proto-pod",
		"image":     "nginx:latest",
		"ports":     "8080/INVALID",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for invalid protocol")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "invalid protocol") {
		t.Errorf("expected 'invalid protocol' error, got: %s", text)
	}
}

func TestRunPod_SCTPProtocol(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	handler := getHandler(t, "run_pod", func(s *server.MCPServer) {
		registerRunPod(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"name":      "sctp-pod",
		"image":     "nginx:latest",
		"ports":     "36412/SCTP",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error for SCTP port: %s", resultText(t, res))
	}
}

// ---------------------------------------------------------------------------
// parsePortsString unit tests
// ---------------------------------------------------------------------------

func TestParsePortsString_Single(t *testing.T) {
	ports, err := parsePortsString("80")
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(ports))
	}
	if ports[0]["containerPort"] != int64(80) {
		t.Errorf("expected containerPort=80, got %v", ports[0]["containerPort"])
	}
	if ports[0]["protocol"] != "TCP" {
		t.Errorf("expected protocol=TCP (default), got %v", ports[0]["protocol"])
	}
}

func TestParsePortsString_MultipleWithProtocols(t *testing.T) {
	ports, err := parsePortsString("8080/TCP,9090/UDP,36412/SCTP")
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 3 {
		t.Fatalf("expected 3 ports, got %d", len(ports))
	}
	if ports[0]["protocol"] != "TCP" {
		t.Errorf("got %v", ports[0]["protocol"])
	}
	if ports[1]["protocol"] != "UDP" {
		t.Errorf("got %v", ports[1]["protocol"])
	}
	if ports[2]["protocol"] != "SCTP" {
		t.Errorf("got %v", ports[2]["protocol"])
	}
}

func TestParsePortsString_PortZero(t *testing.T) {
	_, err := parsePortsString("0")
	if err == nil {
		t.Fatal("expected error for port 0")
	}
}

func TestParsePortsString_PortTooHigh(t *testing.T) {
	_, err := parsePortsString("65536")
	if err == nil {
		t.Fatal("expected error for port 65536")
	}
}

func TestParsePortsString_NotANumber(t *testing.T) {
	_, err := parsePortsString("http")
	if err == nil {
		t.Fatal("expected error for non-numeric port")
	}
}

func TestParsePortsString_InvalidProtocol(t *testing.T) {
	_, err := parsePortsString("8080/HTTP")
	if err == nil {
		t.Fatal("expected error for unsupported protocol")
	}
}

func TestParsePortsString_EmptyString(t *testing.T) {
	ports, err := parsePortsString("")
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 0 {
		t.Errorf("expected no ports for empty string, got %d", len(ports))
	}
}
