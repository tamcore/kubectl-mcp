package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// fakePortForwarder implements PortForwarder for testing.
type fakePortForwarder struct {
	localPort uint16
	err       error
	stopCh    chan struct{}
}

func (f *fakePortForwarder) Forward(_ context.Context, _ PortForwardRequest) (*PortForwardResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &PortForwardResult{
		LocalPort: f.localPort,
		StopCh:    f.stopCh,
	}, nil
}

func TestPortForward_HappyPath(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))
	pool := buildWritePool(cfg, dynClient, fakeCS)

	stopCh := make(chan struct{})
	defer close(stopCh)

	fwd := &fakePortForwarder{localPort: 12345, stopCh: stopCh}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"pod":        "my-pod",
		"remotePort": float64(8080),
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

	if result["localPort"].(float64) != 12345 {
		t.Errorf("expected localPort=12345, got %v", result["localPort"])
	}
	if result["remotePort"].(float64) != 8080 {
		t.Errorf("expected remotePort=8080, got %v", result["remotePort"])
	}
	if result["pod"] != "my-pod" {
		t.Errorf("expected pod=my-pod, got %v", result["pod"])
	}
}

func TestPortForward_WithLocalPort(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))
	pool := buildWritePool(cfg, dynClient, fakeCS)

	stopCh := make(chan struct{})
	defer close(stopCh)

	fwd := &fakePortForwarder{localPort: 9090, stopCh: stopCh}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"pod":        "my-pod",
		"remotePort": float64(8080),
		"localPort":  float64(9090),
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

	if result["localPort"].(float64) != 9090 {
		t.Errorf("expected localPort=9090, got %v", result["localPort"])
	}
}

func TestPortForward_TimeoutClamped(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))
	pool := buildWritePool(cfg, dynClient, fakeCS)

	stopCh := make(chan struct{})
	defer close(stopCh)

	fwd := &fakePortForwarder{localPort: 12345, stopCh: stopCh}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"pod":        "my-pod",
		"remotePort": float64(8080),
		"timeout":    float64(9999),
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

	timeout := result["timeout"].(float64)
	if timeout != float64(maxPortForwardTimeout/time.Second) {
		t.Errorf("expected timeout clamped to %d, got %v", maxPortForwardTimeout/time.Second, timeout)
	}
}

func TestPortForward_ForwardError(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))
	pool := buildWritePool(cfg, dynClient, fakeCS)

	fwd := &fakePortForwarder{err: context.DeadlineExceeded}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"pod":        "my-pod",
		"remotePort": float64(8080),
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !res.IsError {
		t.Error("expected error from forward failure")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "failed to start port forward") {
		t.Errorf("expected port forward error, got: %s", text)
	}
}

func TestPortForward_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}, AllowWrite: true}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	fwd := &fakePortForwarder{}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"pod":        "my-pod",
		"remotePort": float64(8080),
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not allowed error, got: %s", text)
	}
}

func TestPortForward_InvalidPort(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))
	pool := buildWritePool(cfg, dynClient, fakeCS)

	fwd := &fakePortForwarder{}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"pod":        "my-pod",
		"remotePort": float64(0),
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !res.IsError {
		t.Error("expected error for invalid port")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "remotePort") {
		t.Errorf("expected remotePort error, got: %s", text)
	}
}
