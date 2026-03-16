package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func TestPatchResource_MergePatch(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "patch_resource", func(s *server.MCPServer) {
		registerPatchResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Pod",
		"name":      "my-pod",
		"namespace": "default",
		"patch":     `{"metadata":{"labels":{"env":"prod"}}}`,
		"patchType": "merge",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Patched Pod/my-pod") {
		t.Errorf("expected patch confirmation, got: %s", text)
	}
	if !strings.Contains(text, "test-ctx") {
		t.Errorf("expected context in response, got: %s", text)
	}
}

func TestPatchResource_StrategicDefault(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testDeployment("my-deploy", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "patch_resource", func(s *server.MCPServer) {
		registerPatchResource(s, pool, cfg)
	})

	// Use merge patch since the fake dynamic client doesn't support strategic merge.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Deployment",
		"name":      "my-deploy",
		"namespace": "default",
		"patch":     `{"metadata":{"labels":{"version":"v2"}}}`,
		"patchType": "merge",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Patched Deployment/my-deploy") {
		t.Errorf("expected patch confirmation, got: %s", text)
	}
}

func TestPatchResource_ClusterScoped(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testNode("node-1"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "patch_resource", func(s *server.MCPServer) {
		registerPatchResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Node",
		"name":      "node-1",
		"patch":     `{"metadata":{"labels":{"role":"worker"}}}`,
		"patchType": "merge",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Patched Node/node-1") {
		t.Errorf("expected patch confirmation, got: %s", text)
	}
}

func TestPatchResource_InvalidPatchType(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "patch_resource", func(s *server.MCPServer) {
		registerPatchResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Pod",
		"name":      "test",
		"namespace": "default",
		"patch":     `{}`,
		"patchType": "invalid",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "invalid patchType") {
		t.Errorf("expected patchType error, got: %s", text)
	}
}

func TestPatchResource_NotFound(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "patch_resource", func(s *server.MCPServer) {
		registerPatchResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Pod",
		"name":      "nonexistent",
		"namespace": "default",
		"patch":     `{"metadata":{"labels":{"x":"y"}}}`,
		"patchType": "merge",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "failed to patch") {
		t.Errorf("expected patch error, got: %s", text)
	}
}

func TestPatchResource_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}, AllowWrite: true}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "patch_resource", func(s *server.MCPServer) {
		registerPatchResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Pod",
		"name":      "test",
		"namespace": "default",
		"patch":     `{}`,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not allowed error, got: %s", text)
	}
}

func TestPatchResource_PatchAsObject(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)
	handler := getHandler(t, "patch_resource", func(s *server.MCPServer) {
		registerPatchResource(s, pool, cfg)
	})

	// LLM sends patch as a JSON object instead of a string.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind":      "Pod",
		"name":      "my-pod",
		"namespace": "default",
		"patch":     map[string]any{"metadata": map[string]any{"labels": map[string]any{"env": "prod"}}},
		"patchType": "merge",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Patched Pod/my-pod") {
		t.Errorf("expected patch confirmation, got: %s", text)
	}
}

func TestParsePatchType(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"json", false},
		{"merge", false},
		{"strategic", false},
		{"", false},
		{"invalid", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := parsePatchType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parsePatchType(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
