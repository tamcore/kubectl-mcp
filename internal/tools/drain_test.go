package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func TestDrainNode_Basic(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	pods := []runtime.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "node-1"},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "node-1"},
		},
	}

	fakeCS := fake.NewClientset(pods...)
	// The fake client doesn't support Eviction subresource natively.
	// Add a reactor that handles eviction by returning success.
	fakeCS.PrependReactor("create", "pods/eviction", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	dynClient := newWriteFakeDynClient(testNode("node-1"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "drain_node", func(s *server.MCPServer) {
		registerDrainNode(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node": "node-1",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Drained node") {
		t.Errorf("expected drain confirmation, got: %s", text)
	}
	if !strings.Contains(text, "Evicted: 2 pods") {
		t.Errorf("expected 2 evicted pods, got: %s", text)
	}
	if !strings.Contains(text, "test-ctx") {
		t.Errorf("expected context in response, got: %s", text)
	}
}

func TestDrainNode_SkipDaemonSet(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	pods := []runtime.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "regular-pod",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{NodeName: "node-1"},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ds-pod",
				Namespace: "kube-system",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "DaemonSet", Name: "my-ds"},
				},
			},
			Spec: corev1.PodSpec{NodeName: "node-1"},
		},
	}

	fakeCS := fake.NewClientset(pods...)
	fakeCS.PrependReactor("create", "pods/eviction", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	dynClient := newWriteFakeDynClient(testNode("node-1"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "drain_node", func(s *server.MCPServer) {
		registerDrainNode(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node":             "node-1",
		"ignoreDaemonSets": true,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Evicted: 1 pods") {
		t.Errorf("expected 1 evicted pod, got: %s", text)
	}
	if !strings.Contains(text, "Skipped: 1 pods") {
		t.Errorf("expected 1 skipped pod, got: %s", text)
	}
	if !strings.Contains(text, "DaemonSet") {
		t.Errorf("expected DaemonSet in skipped reason, got: %s", text)
	}
}

func TestDrainNode_SkipMirrorPod(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	pods := []runtime.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mirror-pod",
				Namespace: "kube-system",
				Annotations: map[string]string{
					"kubernetes.io/config.mirror": "abc123",
				},
			},
			Spec: corev1.PodSpec{NodeName: "node-1"},
		},
	}

	fakeCS := fake.NewClientset(pods...)
	fakeCS.PrependReactor("create", "pods/eviction", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	dynClient := newWriteFakeDynClient(testNode("node-1"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "drain_node", func(s *server.MCPServer) {
		registerDrainNode(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node": "node-1",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Evicted: 0 pods") {
		t.Errorf("expected 0 evicted pods, got: %s", text)
	}
	if !strings.Contains(text, "mirror pod") {
		t.Errorf("expected mirror pod in skipped, got: %s", text)
	}
}

func TestDrainNode_NodeNotFound(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "drain_node", func(s *server.MCPServer) {
		registerDrainNode(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node": "nonexistent",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "failed to cordon") {
		t.Errorf("expected cordon error, got: %s", text)
	}
}

func TestDrainNode_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}, AllowWrite: true, AllowDestructive: true}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "drain_node", func(s *server.MCPServer) {
		registerDrainNode(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node": "test",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not allowed error, got: %s", text)
	}
}

func TestDrainNode_DryRun(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	pods := []runtime.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "node-1"},
		},
	}

	fakeCS := fake.NewClientset(pods...)
	fakeCS.PrependReactor("create", "pods/eviction", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	dynClient := newWriteFakeDynClient(testNode("node-1"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "drain_node", func(s *server.MCPServer) {
		registerDrainNode(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node":   "node-1",
		"dryRun": true,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "DRY RUN") {
		t.Errorf("expected DRY RUN in output, got: %s", text)
	}
	if !strings.Contains(text, "Would evict") {
		t.Errorf("expected 'Would evict' in output, got: %s", text)
	}
}

func TestDrainNode_NoPods(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testNode("empty-node"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "drain_node", func(s *server.MCPServer) {
		registerDrainNode(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node": "empty-node",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Evicted: 0 pods") {
		t.Errorf("expected 0 evicted pods, got: %s", text)
	}
}
