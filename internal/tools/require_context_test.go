package tools

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// ---------------------------------------------------------------------------
// applyRequireContext tests
// ---------------------------------------------------------------------------

func TestApplyRequireContext_MarksContextRequired(t *testing.T) {
	cfg := defaultCfg()
	cfg.RequireContext = true
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fake.NewClientset())
	s := server.NewMCPServer("test", "0.1.0", server.WithToolCapabilities(false))
	RegisterAll(s, pool, cfg)

	toolsWithContext := []string{
		"get_resource", "list_resources", "list_namespaces",
		"get_logs", "get_events", "describe_resource",
	}

	tools := s.ListTools()
	for _, name := range toolsWithContext {
		st, ok := tools[name]
		if !ok {
			t.Fatalf("tool %q not registered", name)
		}
		found := slices.Contains(st.Tool.InputSchema.Required, "context")
		if !found {
			t.Errorf("tool %q should have context in Required, got %v", name, st.Tool.InputSchema.Required)
		}
	}
}

func TestApplyRequireContext_SkipsToolsWithoutContext(t *testing.T) {
	cfg := defaultCfg()
	cfg.RequireContext = true
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fake.NewClientset())
	s := server.NewMCPServer("test", "0.1.0", server.WithToolCapabilities(false))
	RegisterAll(s, pool, cfg)

	exempt := []string{"list_contexts", "stop_port_forward"}
	tools := s.ListTools()
	for _, name := range exempt {
		st, ok := tools[name]
		if !ok {
			t.Fatalf("tool %q not registered", name)
		}
		for _, r := range st.Tool.InputSchema.Required {
			if r == "context" {
				t.Errorf("tool %q should NOT have context in Required", name)
			}
		}
	}
}

func TestApplyRequireContext_UpdatesDescription(t *testing.T) {
	cfg := defaultCfg()
	cfg.RequireContext = true
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fake.NewClientset())
	s := server.NewMCPServer("test", "0.1.0", server.WithToolCapabilities(false))
	RegisterAll(s, pool, cfg)

	tools := s.ListTools()
	st := tools["get_resource"]

	prop, ok := st.Tool.InputSchema.Properties["context"]
	if !ok {
		t.Fatal("get_resource should have context property")
	}
	propMap, ok := prop.(map[string]any)
	if !ok {
		t.Fatalf("context property should be a map, got %T", prop)
	}
	desc, _ := propMap["description"].(string)
	if strings.Contains(desc, "defaults to current context") {
		t.Errorf("context description should not mention default fallback, got: %s", desc)
	}
	if !strings.Contains(desc, "Kubernetes context to use") {
		t.Errorf("context description should contain 'Kubernetes context to use', got: %s", desc)
	}
}

func TestApplyRequireContext_NotAppliedWhenFlagOff(t *testing.T) {
	cfg := defaultCfg()
	cfg.RequireContext = false
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fake.NewClientset())
	s := server.NewMCPServer("test", "0.1.0", server.WithToolCapabilities(false))
	RegisterAll(s, pool, cfg)

	tools := s.ListTools()
	st := tools["get_resource"]

	for _, r := range st.Tool.InputSchema.Required {
		if r == "context" {
			t.Error("context should NOT be in Required when RequireContext is false")
		}
	}
}

// ---------------------------------------------------------------------------
// Handler-level integration tests
// ---------------------------------------------------------------------------

func TestRequireContext_HandlerRejectsEmpty(t *testing.T) {
	cfg := &config.Config{
		AllowedContexts: []string{"*"},
		RequireContext:  true,
	}
	rawCfg := clientcmdapi.Config{
		CurrentContext: "test-ctx",
		Contexts:       map[string]*clientcmdapi.Context{"test-ctx": {}},
	}
	dynClient := newFakeDynClient(testPod("nginx", "default"))
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, rawCfg, dynClient, fakeCS)

	handler := getHandler(t, "get_resource", func(s *server.MCPServer) {
		registerGetResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"kind": "Pod",
		"name": "nginx",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result when context is not provided with RequireContext")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "require-context") {
		t.Errorf("expected require-context in error message, got: %s", text)
	}
}

func TestRequireContext_HandlerAllowsExplicit(t *testing.T) {
	cfg := &config.Config{
		AllowedContexts: []string{"*"},
		RequireContext:  true,
	}
	rawCfg := clientcmdapi.Config{
		CurrentContext: "test-ctx",
		Contexts:       map[string]*clientcmdapi.Context{"test-ctx": {}},
	}
	dynClient := newFakeDynClient(testPod("nginx", "default"))
	fakeCS := fake.NewClientset()
	pool := buildPool(cfg, rawCfg, dynClient, fakeCS)

	handler := getHandler(t, "get_resource", func(s *server.MCPServer) {
		registerGetResource(s, pool, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"context":   "test-ctx",
		"kind":      "Pod",
		"name":      "nginx",
		"namespace": "default",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, res))
	}
}
