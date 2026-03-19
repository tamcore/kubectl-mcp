package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// unmanagedPod returns a pod with no owner references (not managed by any controller).
func unmanagedPod(name, ns, nodeName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.PodSpec{NodeName: nodeName},
	}
}

// managedPod returns a pod owned by a ReplicaSet.
func managedPod(name, ns, nodeName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "rs-1"},
			},
		},
		Spec: corev1.PodSpec{NodeName: nodeName},
	}
}

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

// TestDrainNode_ForceTrue verifies that force=true deletes unmanaged pods
// that cannot be evicted via the eviction API.
func TestDrainNode_ForceTrue(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	pods := []runtime.Object{
		unmanagedPod("unmanaged-pod", "default", "node-1"),
		managedPod("managed-pod", "default", "node-1"),
	}

	fakeCS := fake.NewClientset(pods...)
	// Eviction succeeds for managed pods, but unmanaged pods get a policy
	// violation error to simulate a PodDisruptionBudget or unmanaged rejection.
	fakeCS.PrependReactor("create", "pods/eviction", func(action clienttesting.Action) (bool, runtime.Object, error) {
		ca := action.(clienttesting.CreateAction)
		obj := ca.GetObject()
		// Use the object name to determine whether to allow eviction.
		if obj.(metav1.Object).GetName() == "unmanaged-pod" {
			// Return a 422 Unprocessable Entity style error to trigger force-delete path.
			return true, nil, &unmanagedEvictionError{}
		}
		return true, nil, nil
	})
	// Track delete calls for the unmanaged pod.
	var deletedPods []string
	fakeCS.PrependReactor("delete", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		deletedPods = append(deletedPods, action.(clienttesting.DeleteAction).GetName())
		return true, nil, nil
	})
	dynClient := newWriteFakeDynClient(testNode("node-1"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "drain_node", func(s *server.MCPServer) {
		registerDrainNode(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node":  "node-1",
		"force": true,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("unexpected error with force=true: %s", text)
	}
	// The managed pod should be evicted normally.
	if !strings.Contains(text, "managed-pod") {
		t.Errorf("expected managed-pod in output, got: %s", text)
	}
	// The unmanaged pod should be force-deleted.
	found := false
	for _, name := range deletedPods {
		if name == "unmanaged-pod" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unmanaged-pod to be force-deleted, deleted pods: %v", deletedPods)
	}
}

// unmanagedEvictionError simulates an eviction failure for unmanaged pods.
type unmanagedEvictionError struct{}

func (e *unmanagedEvictionError) Error() string {
	return "cannot evict pod as it would violate the pod's disruption budget"
}

func (e *unmanagedEvictionError) Status() metav1.Status {
	return metav1.Status{Code: 422, Reason: metav1.StatusReasonInvalid}
}

// TestDrainNode_ForceFalse_UnmanagedPodError verifies that force=false (default)
// returns an error when an unmanaged pod cannot be evicted.
func TestDrainNode_ForceFalse_UnmanagedPodError(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	pods := []runtime.Object{
		unmanagedPod("unmanaged-pod", "default", "node-1"),
	}

	fakeCS := fake.NewClientset(pods...)
	// Eviction fails for unmanaged pod.
	fakeCS.PrependReactor("create", "pods/eviction", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, &unmanagedEvictionError{}
	})
	dynClient := newWriteFakeDynClient(testNode("node-1"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "drain_node", func(s *server.MCPServer) {
		registerDrainNode(s, pool)
	})

	// force defaults to false.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node": "node-1",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	// Without force, the eviction error should be reported in the errors section.
	if !strings.Contains(text, "Errors:") {
		t.Errorf("expected Errors section when force=false and unmanaged pod present, got: %s", text)
	}
	if !strings.Contains(text, "unmanaged-pod") {
		t.Errorf("expected unmanaged-pod in errors, got: %s", text)
	}
}

// TestDrainNode_Timeout verifies that timeout>0 returns an error listing
// remaining pods when the deadline expires before all pods are processed.
//
// The drain loop checks the deadline at the top of each iteration. The test
// uses two pods and makes the first eviction block for 100ms while the
// timeout is set to 50ms, so the deadline expires before the second pod is
// attempted.
func TestDrainNode_Timeout(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	pods := []runtime.Object{
		// pod-done is processed first and its eviction blocks for 100ms.
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-done", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "node-1"},
		},
		// slow-pod is the second pod — it should appear in the timeout error.
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "slow-pod", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "node-1"},
		},
	}

	fakeCS := fake.NewClientset(pods...)
	evictCount := 0
	fakeCS.PrependReactor("create", "pods/eviction", func(action clienttesting.Action) (bool, runtime.Object, error) {
		evictCount++
		if evictCount == 1 {
			// Block long enough that the 50ms deadline expires before the
			// loop reaches the second pod.
			time.Sleep(100 * time.Millisecond)
		}
		return true, nil, nil
	})
	dynClient := newWriteFakeDynClient(testNode("node-1"))
	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "drain_node", func(s *server.MCPServer) {
		registerDrainNode(s, pool)
	})

	// timeout=0.05 → 50ms deadline; the first eviction sleeps 100ms, so the
	// deadline will have elapsed when the loop starts the second pod.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node":    "node-1",
		"timeout": float64(0.05),
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !res.IsError {
		t.Errorf("expected timeout error, got: %s", text)
	}
	if !strings.Contains(text, "timed out") {
		t.Errorf("expected 'timed out' in error, got: %s", text)
	}
	if !strings.Contains(text, "slow-pod") {
		t.Errorf("expected remaining pod in timeout error, got: %s", text)
	}
}

// TestDrainNode_TimeoutZero_NoTimeout verifies that timeout=0 (default) does
// not impose any timeout — eviction errors are reported but no timeout error.
func TestDrainNode_TimeoutZero_NoTimeout(t *testing.T) {
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
		"node":    "node-1",
		"timeout": float64(0), // explicit 0 = no timeout
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("unexpected error with timeout=0: %s", text)
	}
	if strings.Contains(text, "timed out") {
		t.Errorf("should not have timeout error when timeout=0, got: %s", text)
	}
	if !strings.Contains(text, "Evicted: 1 pods") {
		t.Errorf("expected 1 evicted pod, got: %s", text)
	}
}

// TestDrainNode_ForceAndTimeout verifies that force=true and timeout>0 work
// together — force-deletes unmanaged pods, times out if evictions stall.
func TestDrainNode_ForceAndTimeout(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	pods := []runtime.Object{
		unmanagedPod("unmanaged-pod", "default", "node-1"),
	}

	fakeCS := fake.NewClientset(pods...)
	// Eviction fails — triggers force-delete path.
	fakeCS.PrependReactor("create", "pods/eviction", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, &unmanagedEvictionError{}
	})
	var deletedPods []string
	fakeCS.PrependReactor("delete", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		deletedPods = append(deletedPods, action.(clienttesting.DeleteAction).GetName())
		return true, nil, nil
	})
	dynClient := newWriteFakeDynClient(testNode("node-1"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "drain_node", func(s *server.MCPServer) {
		registerDrainNode(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node":    "node-1",
		"force":   true,
		"timeout": float64(30),
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("unexpected error with force=true and timeout=30: %s", text)
	}
	// Unmanaged pod should have been force-deleted.
	found := false
	for _, name := range deletedPods {
		if name == "unmanaged-pod" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unmanaged-pod to be force-deleted; deleted: %v", deletedPods)
	}
}

// TestDrainNode_ForceAndDryRun verifies that force=true with dryRun=true
// does not actually force-delete pods and reports DRY RUN.
func TestDrainNode_ForceAndDryRun(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	pods := []runtime.Object{
		unmanagedPod("unmanaged-pod", "default", "node-1"),
	}

	fakeCS := fake.NewClientset(pods...)
	fakeCS.PrependReactor("create", "pods/eviction", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil // dry-run evictions succeed
	})
	var deletedPods []string
	fakeCS.PrependReactor("delete", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		deletedPods = append(deletedPods, action.(clienttesting.DeleteAction).GetName())
		return true, nil, nil
	})
	dynClient := newWriteFakeDynClient(testNode("node-1"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "drain_node", func(s *server.MCPServer) {
		registerDrainNode(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"node":   "node-1",
		"force":  true,
		"dryRun": true,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("unexpected error: %s", text)
	}
	if !strings.Contains(text, "DRY RUN") {
		t.Errorf("expected DRY RUN in output, got: %s", text)
	}
	// No actual deletes should happen during dry-run.
	if len(deletedPods) > 0 {
		t.Errorf("expected no actual deletes during dry-run, got: %v", deletedPods)
	}
}

// TestDrainNode_ToolDescription_MentionsForceRisk verifies that the tool
// description or elicitation text mentions the risk associated with force.
func TestDrainNode_ToolDescription_MentionsForceRisk(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.AllowDestructive = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()
	pool := buildWritePool(cfg, dynClient, fakeCS)

	s := server.NewMCPServer("test", "0.1.0", server.WithToolCapabilities(false))
	registerDrainNode(s, pool)

	tool := s.GetTool("drain_node")
	if tool == nil {
		t.Fatal("drain_node tool not found")
	}

	desc := tool.Tool.Description
	descLower := strings.ToLower(desc)
	if !strings.Contains(descLower, "force") && !strings.Contains(descLower, "unmanaged") && !strings.Contains(descLower, "lost") && !strings.Contains(descLower, "deleted") {
		t.Errorf("tool description should mention force risk (unmanaged/lost/deleted pods), got: %s", desc)
	}
}
