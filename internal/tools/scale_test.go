package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func newScaleFakeClientset(objs ...runtime.Object) *fake.Clientset {
	fakeCS := fake.NewClientset(objs...)

	// Add reactors for scale subresource since the default fake doesn't support it.
	fakeCS.PrependReactor("get", "deployments/scale", func(action clienttesting.Action) (bool, runtime.Object, error) {
		getAction := action.(clienttesting.GetAction)
		for _, obj := range objs {
			if d, ok := obj.(*appsv1.Deployment); ok && d.Name == getAction.GetName() && d.Namespace == getAction.GetNamespace() {
				replicas := int32(1)
				if d.Spec.Replicas != nil {
					replicas = *d.Spec.Replicas
				}
				return true, &autoscalingv1.Scale{
					ObjectMeta: metav1.ObjectMeta{Name: d.Name, Namespace: d.Namespace},
					Spec:       autoscalingv1.ScaleSpec{Replicas: replicas},
				}, nil
			}
		}
		return true, nil, fmt.Errorf("deployment %q not found", getAction.GetName())
	})
	fakeCS.PrependReactor("update", "deployments/scale", func(action clienttesting.Action) (bool, runtime.Object, error) {
		updateAction := action.(clienttesting.UpdateAction)
		scale := updateAction.GetObject().(*autoscalingv1.Scale)
		return true, scale, nil
	})
	fakeCS.PrependReactor("get", "statefulsets/scale", func(action clienttesting.Action) (bool, runtime.Object, error) {
		getAction := action.(clienttesting.GetAction)
		for _, obj := range objs {
			if s, ok := obj.(*appsv1.StatefulSet); ok && s.Name == getAction.GetName() && s.Namespace == getAction.GetNamespace() {
				replicas := int32(1)
				if s.Spec.Replicas != nil {
					replicas = *s.Spec.Replicas
				}
				return true, &autoscalingv1.Scale{
					ObjectMeta: metav1.ObjectMeta{Name: s.Name, Namespace: s.Namespace},
					Spec:       autoscalingv1.ScaleSpec{Replicas: replicas},
				}, nil
			}
		}
		return true, nil, fmt.Errorf("statefulset %q not found", getAction.GetName())
	})
	fakeCS.PrependReactor("update", "statefulsets/scale", func(action clienttesting.Action) (bool, runtime.Object, error) {
		updateAction := action.(clienttesting.UpdateAction)
		scale := updateAction.GetObject().(*autoscalingv1.Scale)
		return true, scale, nil
	})
	fakeCS.PrependReactor("get", "replicasets/scale", func(action clienttesting.Action) (bool, runtime.Object, error) {
		getAction := action.(clienttesting.GetAction)
		for _, obj := range objs {
			if r, ok := obj.(*appsv1.ReplicaSet); ok && r.Name == getAction.GetName() && r.Namespace == getAction.GetNamespace() {
				replicas := int32(1)
				if r.Spec.Replicas != nil {
					replicas = *r.Spec.Replicas
				}
				return true, &autoscalingv1.Scale{
					ObjectMeta: metav1.ObjectMeta{Name: r.Name, Namespace: r.Namespace},
					Spec:       autoscalingv1.ScaleSpec{Replicas: replicas},
				}, nil
			}
		}
		return true, nil, fmt.Errorf("replicaset %q not found", getAction.GetName())
	})
	fakeCS.PrependReactor("update", "replicasets/scale", func(action clienttesting.Action) (bool, runtime.Object, error) {
		updateAction := action.(clienttesting.UpdateAction)
		scale := updateAction.GetObject().(*autoscalingv1.Scale)
		return true, scale, nil
	})

	return fakeCS
}

func TestScaleResource_Deployment(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-deploy", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: new(int32(3))},
	}
	fakeCS := newScaleFakeClientset(deploy)
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "scale_resource", func(s *server.MCPServer) {
		registerScaleResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "my-deploy",
		"namespace": "default",
		"replicas":  float64(5),
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Scaled Deployment/my-deploy") {
		t.Errorf("expected scale confirmation, got: %s", text)
	}
	if !strings.Contains(text, "from 3 to 5") {
		t.Errorf("expected old/new replicas, got: %s", text)
	}
}

func TestScaleResource_StatefulSet(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "my-sts", Namespace: "default"},
		Spec:       appsv1.StatefulSetSpec{Replicas: new(int32(2))},
	}
	fakeCS := newScaleFakeClientset(sts)
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "scale_resource", func(s *server.MCPServer) {
		registerScaleResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "StatefulSet",
		"name":      "my-sts",
		"namespace": "default",
		"replicas":  float64(4),
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Scaled StatefulSet/my-sts") {
		t.Errorf("expected scale confirmation, got: %s", text)
	}
	if !strings.Contains(text, "from 2 to 4") {
		t.Errorf("expected old/new replicas, got: %s", text)
	}
}

func TestScaleResource_ReplicaSet(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rs", Namespace: "default"},
		Spec:       appsv1.ReplicaSetSpec{Replicas: new(int32(1))},
	}
	fakeCS := newScaleFakeClientset(rs)
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "scale_resource", func(s *server.MCPServer) {
		registerScaleResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "ReplicaSet",
		"name":      "my-rs",
		"namespace": "default",
		"replicas":  float64(3),
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Scaled ReplicaSet/my-rs") {
		t.Errorf("expected scale confirmation, got: %s", text)
	}
}

func TestScaleResource_UnsupportedKind(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "scale_resource", func(s *server.MCPServer) {
		registerScaleResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "DaemonSet",
		"name":      "test",
		"namespace": "default",
		"replicas":  float64(3),
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not scalable") {
		t.Errorf("expected unsupported kind error, got: %s", text)
	}
}

func TestScaleResource_NotFound(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "scale_resource", func(s *server.MCPServer) {
		registerScaleResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "nonexistent",
		"namespace": "default",
		"replicas":  float64(1),
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "failed to get scale") {
		t.Errorf("expected not found error, got: %s", text)
	}
}

func TestScaleResource_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}, AllowWrite: true}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "scale_resource", func(s *server.MCPServer) {
		registerScaleResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "test",
		"namespace": "default",
		"replicas":  float64(1),
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not allowed error, got: %s", text)
	}
}
