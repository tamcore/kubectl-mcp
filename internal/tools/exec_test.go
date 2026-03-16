package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

func TestExecPod_HappyPath(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)

	runner := &fakeExecRunner{stdout: "hello world\n"}
	handler := getHandler(t, "exec_pod", func(s *server.MCPServer) {
		registerExecPod(s, pool, runner)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"command":   []any{"echo", "hello", "world"},
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "hello world") {
		t.Errorf("expected stdout in output, got: %s", text)
	}
}

func TestExecPod_StderrIncluded(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)

	runner := &fakeExecRunner{stdout: "out\n", stderr: "err\n"}
	handler := getHandler(t, "exec_pod", func(s *server.MCPServer) {
		registerExecPod(s, pool, runner)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"command":   []any{"sh", "-c", "echo out; echo err >&2"},
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "out") {
		t.Errorf("expected stdout in output, got: %s", text)
	}
	if !strings.Contains(text, "err") {
		t.Errorf("expected stderr in output, got: %s", text)
	}
}

func TestExecPod_MissingCommand(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)

	runner := &fakeExecRunner{}
	handler := getHandler(t, "exec_pod", func(s *server.MCPServer) {
		registerExecPod(s, pool, runner)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "command") {
		t.Errorf("expected error about missing command, got: %s", text)
	}
}

func TestExecPod_ExecError(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)

	runner := &fakeExecRunner{execErr: "command not found"}
	handler := getHandler(t, "exec_pod", func(s *server.MCPServer) {
		registerExecPod(s, pool, runner)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"command":   []any{"nonexistent"},
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "command not found") {
		t.Errorf("expected exec error in output, got: %s", text)
	}
}

func TestExecPod_ContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}, AllowWrite: true}
	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient()

	pool := buildWritePool(cfg, dynClient, fakeCS)

	runner := &fakeExecRunner{}
	handler := getHandler(t, "exec_pod", func(s *server.MCPServer) {
		registerExecPod(s, pool, runner)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"command":   []any{"echo"},
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not allowed error, got: %s", text)
	}
}

func TestExecPod_CommandAsString(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)

	runner := &fakeExecRunner{stdout: "ok\n"}
	handler := getHandler(t, "exec_pod", func(s *server.MCPServer) {
		registerExecPod(s, pool, runner)
	})

	// LLM sends command as a single string instead of array.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"command":   "ls -la",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "ok") {
		t.Errorf("expected output, got: %s", text)
	}
}
