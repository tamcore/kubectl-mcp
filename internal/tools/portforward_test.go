package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// fakePortForwarder implements PortForwarder for testing.
type fakePortForwarder struct {
	localPort uint16
	err       error
	stopCh    chan struct{}
}

func (f *fakePortForwarder) Forward(_ context.Context, _ kubernetes.Interface, _ *rest.Config, _ PortForwardRequest) (*PortForwardResult, error) {
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
		registerPortForward(s, pool, fwd, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"resource":   "my-pod",
		"remotePort": float64(8080),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	text := resultText(t, res)
	var result map[string]any
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
		registerPortForward(s, pool, fwd, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"resource":   "my-pod",
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
	var result map[string]any
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
		registerPortForward(s, pool, fwd, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"resource":   "my-pod",
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
	var result map[string]any
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
		registerPortForward(s, pool, fwd, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"resource":   "my-pod",
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
		registerPortForward(s, pool, fwd, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"resource":   "my-pod",
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
		registerPortForward(s, pool, fwd, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"resource":   "my-pod",
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

func TestPortForward_PortValidation(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))
	pool := buildWritePool(cfg, dynClient, fakeCS)
	fwd := &fakePortForwarder{}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd, cfg)
	})

	cases := []struct {
		name       string
		remotePort float64
		localPort  float64
		wantErrSub string
	}{
		{"remotePort zero", 0, 0, "remotePort"},
		{"remotePort negative", -1, 0, "remotePort"},
		{"remotePort overflow 70000", 70000, 0, "remotePort"},
		{"remotePort 65536", 65536, 0, "remotePort"},
		{"localPort negative", 8080, -1, "localPort"},
		{"localPort overflow 70000", 8080, 70000, "localPort"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := handler(context.Background(), callToolReq(map[string]any{
				"namespace":  "default",
				"resource":   "my-pod",
				"remotePort": tc.remotePort,
				"localPort":  tc.localPort,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if !res.IsError {
				t.Errorf("expected error for %s", tc.name)
			}
			text := resultText(t, res)
			if !strings.Contains(text, tc.wantErrSub) {
				t.Errorf("expected %q in error, got: %s", tc.wantErrSub, text)
			}
		})
	}
}

// --- resource parameter tests ---

func TestParseResource(t *testing.T) {
	tests := []struct {
		input    string
		wantKind string
		wantName string
	}{
		{"my-pod", "pod", "my-pod"},
		{"pod/my-pod", "pod", "my-pod"},
		{"Pod/my-pod", "pod", "my-pod"},
		{"svc/my-svc", "service", "my-svc"},
		{"service/my-svc", "service", "my-svc"},
		{"Service/my-svc", "service", "my-svc"},
		{"deploy/my-deploy", "deployment", "my-deploy"},
		{"deployment/my-deploy", "deployment", "my-deploy"},
		{"Deployment/my-deploy", "deployment", "my-deploy"},
		{"sts/my-sts", "statefulset", "my-sts"},
		{"statefulset/my-sts", "statefulset", "my-sts"},
		{"StatefulSet/my-sts", "statefulset", "my-sts"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			kind, name := parseResource(tt.input)
			if kind != tt.wantKind {
				t.Errorf("parseResource(%q) kind = %q, want %q", tt.input, kind, tt.wantKind)
			}
			if name != tt.wantName {
				t.Errorf("parseResource(%q) name = %q, want %q", tt.input, name, tt.wantName)
			}
		})
	}
}

func TestParseResource_UnknownKind(t *testing.T) {
	kind, name := parseResource("job/my-job")
	if kind != "job" {
		t.Errorf("expected kind=job, got %q", kind)
	}
	if name != "my-job" {
		t.Errorf("expected name=my-job, got %q", name)
	}
}

func TestPortForward_ResourceBareName(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))
	pool := buildWritePool(cfg, dynClient, fakeCS)

	stopCh := make(chan struct{})
	defer close(stopCh)
	fwd := &fakePortForwarder{localPort: 12345, stopCh: stopCh}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"resource":   "my-pod",
		"remotePort": float64(8080),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	text := resultText(t, res)
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("expected JSON output, got: %s", text)
	}
	if result["pod"] != "my-pod" {
		t.Errorf("expected pod=my-pod, got %v", result["pod"])
	}
}

func TestPortForward_ResourceExplicitPod(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))
	pool := buildWritePool(cfg, dynClient, fakeCS)

	stopCh := make(chan struct{})
	defer close(stopCh)
	fwd := &fakePortForwarder{localPort: 12345, stopCh: stopCh}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"resource":   "pod/my-pod",
		"remotePort": float64(8080),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	text := resultText(t, res)
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("expected JSON output, got: %s", text)
	}
	if result["pod"] != "my-pod" {
		t.Errorf("expected pod=my-pod, got %v", result["pod"])
	}
}

func TestPortForward_ResourceService(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	// Create a ready pod matching the service selector.
	pod := &corev1.Pod{
		Name:      "svc-pod",
		Namespace: "default",
		Labels:    map[string]string{"app": "my-svc"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
	// Create a Service with a selector and named port.
	svc := &corev1.Service{
		Name:      "my-svc",
		Namespace: "default",
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "my-svc"},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt32(8080),
				},
			},
		},
	}
	fakeCS := fake.NewClientset(pod, svc)
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	stopCh := make(chan struct{})
	defer close(stopCh)
	fwd := &fakePortForwarder{localPort: 12345, stopCh: stopCh}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"resource":   "svc/my-svc",
		"remotePort": float64(8080),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	text := resultText(t, res)
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("expected JSON output, got: %s", text)
	}
	if result["pod"] != "svc-pod" {
		t.Errorf("expected pod=svc-pod (resolved from service), got %v", result["pod"])
	}
}

func TestPortForward_ResourceServiceNamedPort(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	pod := &corev1.Pod{
		Name:      "svc-pod",
		Namespace: "default",
		Labels:    map[string]string{"app": "my-svc"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
	svc := &corev1.Service{
		Name:      "my-svc",
		Namespace: "default",
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "my-svc"},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt32(8080),
				},
			},
		},
	}
	fakeCS := fake.NewClientset(pod, svc)
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	stopCh := make(chan struct{})
	defer close(stopCh)

	var capturedReq PortForwardRequest
	fwd := &capturingPortForwarder{localPort: 12345, stopCh: stopCh, captured: &capturedReq}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd, cfg)
	})

	// remotePort=80 matches the named port "http" whose targetPort=8080.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"resource":   "svc/my-svc",
		"remotePort": float64(80),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	// The port forwarder should have been called with targetPort=8080.
	if capturedReq.RemotePort != 8080 {
		t.Errorf("expected resolved targetPort=8080, got %d", capturedReq.RemotePort)
	}
}

func TestPortForward_ResourceDeployment(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	pod := &corev1.Pod{
		Name:      "deploy-pod",
		Namespace: "default",
		Labels:    map[string]string{"app": "my-deploy"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	deploy := &appsv1.Deployment{
		Name:      "my-deploy",
		Namespace: "default",
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "my-deploy"},
			},
		},
	}
	fakeCS := fake.NewClientset(pod, deploy)
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	stopCh := make(chan struct{})
	defer close(stopCh)
	fwd := &fakePortForwarder{localPort: 12345, stopCh: stopCh}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"resource":   "deploy/my-deploy",
		"remotePort": float64(8080),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	text := resultText(t, res)
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("expected JSON output, got: %s", text)
	}
	if result["pod"] != "deploy-pod" {
		t.Errorf("expected pod=deploy-pod (resolved from deployment), got %v", result["pod"])
	}
}

func TestPortForward_ResourceStatefulSet(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	// StatefulSet prefers pod-0.
	pod0 := &corev1.Pod{
		Name:      "my-sts-0",
		Namespace: "default",
		Labels:    map[string]string{"app": "my-sts"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
	pod1 := &corev1.Pod{
		Name:      "my-sts-1",
		Namespace: "default",
		Labels:    map[string]string{"app": "my-sts"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	sts := &appsv1.StatefulSet{
		Name:      "my-sts",
		Namespace: "default",
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "my-sts"},
			},
		},
	}
	fakeCS := fake.NewClientset(pod0, pod1, sts)
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	stopCh := make(chan struct{})
	defer close(stopCh)
	fwd := &fakePortForwarder{localPort: 12345, stopCh: stopCh}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"resource":   "sts/my-sts",
		"remotePort": float64(8080),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}

	text := resultText(t, res)
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("expected JSON output, got: %s", text)
	}
	// Should prefer my-sts-0 over my-sts-1.
	if result["pod"] != "my-sts-0" {
		t.Errorf("expected pod=my-sts-0 (preferred pod-0 for StatefulSet), got %v", result["pod"])
	}
}

func TestPortForward_ResourceUnknownKind(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	fwd := &fakePortForwarder{}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"resource":   "job/my-job",
		"remotePort": float64(8080),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for unknown kind")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "unsupported") {
		t.Errorf("expected 'unsupported' error, got: %s", text)
	}
}

func TestPortForward_NoReadyPods(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	// Pod exists but is not ready.
	notReadyPod := &corev1.Pod{
		Name:      "pending-pod",
		Namespace: "default",
		Labels:    map[string]string{"app": "my-svc"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
		},
	}
	svc := &corev1.Service{
		Name:      "my-svc",
		Namespace: "default",
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "my-svc"},
			Ports: []corev1.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt32(8080)},
			},
		},
	}
	fakeCS := fake.NewClientset(notReadyPod, svc)
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	fwd := &fakePortForwarder{}

	handler := getHandler(t, "port_forward", func(s *server.MCPServer) {
		registerPortForward(s, pool, fwd, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":  "default",
		"resource":   "svc/my-svc",
		"remotePort": float64(8080),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error when no ready pods found")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "no ready pod") {
		t.Errorf("expected 'no ready pod' error, got: %s", text)
	}
}

// capturingPortForwarder records the PortForwardRequest it was called with.
type capturingPortForwarder struct {
	localPort uint16
	stopCh    chan struct{}
	captured  *PortForwardRequest
}

func (c *capturingPortForwarder) Forward(_ context.Context, _ kubernetes.Interface, _ *rest.Config, req PortForwardRequest) (*PortForwardResult, error) {
	*c.captured = req
	return &PortForwardResult{
		LocalPort: c.localPort,
		StopCh:    c.stopCh,
	}, nil
}
