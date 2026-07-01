package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func TestExplainResource_SimpleKind(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "explain_resource", func(s *server.MCPServer) {
		registerExplainResource(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"resource": "Pod",
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

	if result["kind"] != "Pod" {
		t.Errorf("expected kind=Pod, got %v", result["kind"])
	}
	if result["apiVersion"] != "v1" {
		t.Errorf("expected apiVersion=v1, got %v", result["apiVersion"])
	}
}

func TestExplainResource_WithApiVersion(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "explain_resource", func(s *server.MCPServer) {
		registerExplainResource(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"resource":   "Deployment",
		"apiVersion": "apps/v1",
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

	if result["kind"] != "Deployment" {
		t.Errorf("expected kind=Deployment, got %v", result["kind"])
	}
	if result["apiVersion"] != "apps/v1" {
		t.Errorf("expected apiVersion=apps/v1, got %v", result["apiVersion"])
	}
}

func TestExplainResource_DottedPath(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "explain_resource", func(s *server.MCPServer) {
		registerExplainResource(s, pool)
	})

	// "Pod.spec" - should resolve the kind and include path info.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"resource": "Pod.spec",
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

	if result["kind"] != "Pod" {
		t.Errorf("expected kind=Pod, got %v", result["kind"])
	}
	if result["fieldPath"] != "spec" {
		t.Errorf("expected fieldPath=spec, got %v", result["fieldPath"])
	}
}

func TestExplainResource_UnknownKind(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "explain_resource", func(s *server.MCPServer) {
		registerExplainResource(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"resource": "BogusKind",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !res.IsError {
		t.Error("expected error for unknown kind")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "could not resolve") {
		t.Errorf("expected resolve error, got: %s", text)
	}
}

func TestExplainResource_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}}
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "explain_resource", func(s *server.MCPServer) {
		registerExplainResource(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"resource": "Pod",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not allowed error, got: %s", text)
	}
}

func TestExplainResource_ContainsVerbs(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "explain_resource", func(s *server.MCPServer) {
		registerExplainResource(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"resource": "Pod",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("expected JSON output, got: %s", text)
	}

	verbs, ok := result["verbs"].([]any)
	if !ok {
		t.Fatal("expected verbs array")
	}
	if len(verbs) == 0 {
		t.Error("expected at least one verb")
	}

	namespaced, ok := result["namespaced"].(bool)
	if !ok {
		t.Fatal("expected namespaced field")
	}
	if !namespaced {
		t.Error("Pod should be namespaced")
	}
}
