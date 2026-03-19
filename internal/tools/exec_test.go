package tools

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
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

func TestExecPod_QuotedStringCommand(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)

	var captured []string
	runner := &fakeExecRunner{
		stdout:     "ok\n",
		captureCmd: &captured,
	}
	handler := getHandler(t, "exec_pod", func(s *server.MCPServer) {
		registerExecPod(s, pool, runner)
	})

	// LLM sends sh -c with a quoted argument — quotes must be preserved.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"command":   `sh -c "echo hello world"`,
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "ok") {
		t.Errorf("expected output, got: %s", text)
	}

	if len(captured) != 3 {
		t.Fatalf("expected 3 command parts, got %d: %v", len(captured), captured)
	}
	if captured[0] != "sh" || captured[1] != "-c" || captured[2] != "echo hello world" {
		t.Errorf("unexpected command split: %v", captured)
	}
}

func TestExecPod_ErrorIncludesStderr(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)

	runner := &fakeExecRunner{
		execErr: "command terminated with exit code 2",
		stderr:  "sh: wget: not found\n",
	}
	handler := getHandler(t, "exec_pod", func(s *server.MCPServer) {
		registerExecPod(s, pool, runner)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"command":   []any{"wget", "-qO-", "http://localhost"},
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "exit code 2") {
		t.Errorf("expected exit code in output, got: %s", text)
	}
	if !strings.Contains(text, "wget: not found") {
		t.Errorf("expected stderr in error output, got: %s", text)
	}
}

func TestFormatExecError_Timeout(t *testing.T) {
	var stderr bytes.Buffer
	stderr.WriteString("partial output\n")

	msg := formatExecError(context.DeadlineExceeded, 30*time.Second, &stderr)

	if !strings.Contains(msg, "timed out after 30s") {
		t.Errorf("expected timeout message, got: %s", msg)
	}
	if !strings.Contains(msg, "timeout parameter") {
		t.Errorf("expected suggestion to increase timeout, got: %s", msg)
	}
	if !strings.Contains(msg, "partial output") {
		t.Errorf("expected stderr in timeout message, got: %s", msg)
	}
}

func TestFormatExecError_RegularError(t *testing.T) {
	var stderr bytes.Buffer
	stderr.WriteString("permission denied\n")

	err := &execTestErr{msg: "command terminated with exit code 1"}
	msg := formatExecError(err, 30*time.Second, &stderr)

	if !strings.Contains(msg, "exit code 1") {
		t.Errorf("expected error message, got: %s", msg)
	}
	if !strings.Contains(msg, "permission denied") {
		t.Errorf("expected stderr, got: %s", msg)
	}
	if strings.Contains(msg, "timed out") {
		t.Errorf("should not mention timeout for non-deadline error, got: %s", msg)
	}
}

func TestFormatExecError_NoStderr(t *testing.T) {
	var stderr bytes.Buffer
	err := &execTestErr{msg: "something failed"}
	msg := formatExecError(err, 30*time.Second, &stderr)

	if strings.Contains(msg, "STDERR") {
		t.Errorf("should not include STDERR section when empty, got: %s", msg)
	}
}

type execTestErr struct{ msg string }

func (e *execTestErr) Error() string { return e.msg }

// ---------------------------------------------------------------------------
// requireCommand — unit tests for all error branches
// ---------------------------------------------------------------------------

func TestRequireCommand_EmptyString(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"command": ""}
	_, err := requireCommand(req)
	if err == nil {
		t.Fatal("expected error for empty string command, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error, got: %v", err)
	}
}

func TestRequireCommand_WhitespaceOnlyString(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"command": "   "}
	_, err := requireCommand(req)
	if err == nil {
		t.Fatal("expected error for whitespace-only command, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error, got: %v", err)
	}
}

func TestRequireCommand_InvalidQuotedString(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"command": `sh -c "unterminated`}
	_, err := requireCommand(req)
	if err == nil {
		t.Fatal("expected error for unterminated quote, got nil")
	}
	if !strings.Contains(err.Error(), "invalid command string") {
		t.Errorf("expected 'invalid command string' in error, got: %v", err)
	}
}

func TestRequireCommand_EmptyArray(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"command": []any{}}
	_, err := requireCommand(req)
	if err == nil {
		t.Fatal("expected error for empty command array, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error, got: %v", err)
	}
}

func TestRequireCommand_ArrayWithNonStringElement(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"command": []any{"ls", 42}}
	_, err := requireCommand(req)
	if err == nil {
		t.Fatal("expected error for non-string array element, got nil")
	}
	if !strings.Contains(err.Error(), "must be strings") {
		t.Errorf("expected 'must be strings' in error, got: %v", err)
	}
}

func TestRequireCommand_StringifiedJSONArray(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"command": `["sh", "-c", "hostname && uname -a"]`}
	cmd, err := requireCommand(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"sh", "-c", "hostname && uname -a"}
	if len(cmd) != len(want) {
		t.Fatalf("got %d tokens %v, want %d tokens %v", len(cmd), cmd, len(want), want)
	}
	for i := range cmd {
		if cmd[i] != want[i] {
			t.Errorf("token[%d] = %q, want %q", i, cmd[i], want[i])
		}
	}
}

func TestRequireCommand_StringifiedJSONArraySimple(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"command": `["ls", "-la"]`}
	cmd, err := requireCommand(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"ls", "-la"}
	if len(cmd) != len(want) {
		t.Fatalf("got %d tokens %v, want %d tokens %v", len(cmd), cmd, len(want), want)
	}
	for i := range cmd {
		if cmd[i] != want[i] {
			t.Errorf("token[%d] = %q, want %q", i, cmd[i], want[i])
		}
	}
}

func TestRequireCommand_BracketStringNotJSON(t *testing.T) {
	// A string starting with [ that is NOT valid JSON should still be shell-split.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"command": `[not json at all`}
	_, err := requireCommand(req)
	// This will hit shellSplit, which should return an error for unterminated bracket
	// or produce tokens — either way, it should not panic.
	_ = err
}

func TestRequireCommand_WrongType(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"command": 42}
	_, err := requireCommand(req)
	if err == nil {
		t.Fatal("expected error for integer command, got nil")
	}
	if !strings.Contains(err.Error(), "string or array") {
		t.Errorf("expected 'string or array' in error, got: %v", err)
	}
}

func TestRequireCommand_MissingKey(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	_, err := requireCommand(req)
	if err == nil {
		t.Fatal("expected error for missing command key, got nil")
	}
	if !strings.Contains(err.Error(), "command") {
		t.Errorf("expected 'command' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// registerExecPod — uncovered handler branches
// ---------------------------------------------------------------------------

func TestExecPod_NoOutput(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)

	// Runner produces neither stdout nor stderr.
	runner := &fakeExecRunner{}
	handler := getHandler(t, "exec_pod", func(s *server.MCPServer) {
		registerExecPod(s, pool, runner)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"command":   []any{"true"},
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if text != "(no output)" {
		t.Errorf("expected '(no output)', got: %s", text)
	}
}

func TestExecPod_TimeoutCapped(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)

	runner := &fakeExecRunner{stdout: "ok\n"}
	handler := getHandler(t, "exec_pod", func(s *server.MCPServer) {
		registerExecPod(s, pool, runner)
	})

	// Request timeout larger than maxExecTimeout (300s); should be silently capped.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"command":   []any{"echo", "ok"},
		"timeout":   float64(9999),
	}))
	if err != nil {
		t.Fatal(err)
	}

	// The request should succeed — the cap is transparent to the caller.
	text := resultText(t, res)
	if !strings.Contains(text, "ok") {
		t.Errorf("expected output after timeout cap, got: %s", text)
	}
}

func TestExecPod_TimeoutZeroUsesDefault(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	fakeCS := fake.NewClientset()
	dynClient := newWriteFakeDynClient(testPod("my-pod", "default"))

	pool := buildWritePool(cfg, dynClient, fakeCS)

	runner := &fakeExecRunner{stdout: "ok\n"}
	handler := getHandler(t, "exec_pod", func(s *server.MCPServer) {
		registerExecPod(s, pool, runner)
	})

	// Explicit timeout of 0 should fall back to the 30 s default.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"command":   []any{"echo", "ok"},
		"timeout":   float64(0),
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "ok") {
		t.Errorf("expected output after zero-timeout fallback, got: %s", text)
	}
}

func TestExecPod_ClientForError(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true

	// Build a pool whose rawConfig has the "test-ctx" context entry so
	// ResolveContext succeeds, but supply NO pre-loaded clients so that
	// ClientFor must attempt lazy construction from a bare kubeconfig and
	// will fail (there is no cluster/user/TLS config to use).
	pool := kube.NewClientPoolForTest(cfg, defaultRawConfig(), nil)

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
	if !strings.Contains(text, "failed to get client") {
		t.Errorf("expected 'failed to get client' error, got: %s", text)
	}
}
