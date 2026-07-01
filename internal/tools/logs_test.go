package tools

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// testRunningPod returns an unstructured Pod with the given labels and Running phase.
func testRunningPod(name, ns string, labels map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":              name,
			"namespace":         ns,
			"labels":            labels,
			"creationTimestamp": "2024-01-01T00:00:00Z",
		},
		"spec": map[string]any{
			"nodeName": "node-1",
		},
		"status": map[string]any{
			"phase": "Running",
		},
	}}
}

func TestGetLogs_RequiresPodOrLabelSelector(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	// Neither pod nor labelSelector provided.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error when neither pod nor labelSelector is provided")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "pod") || !strings.Contains(text, "labelSelector") {
		t.Errorf("expected error mentioning pod and labelSelector, got: %s", text)
	}
}

func TestGetLogs_LabelSelectorNoPods(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient() // no pods
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":     "default",
		"labelSelector": "app=nginx",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error when no pods match selector")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "no pods") {
		t.Errorf("expected 'no pods' message, got: %s", text)
	}
}

func TestGetLogs_LabelSelectorFindsPods(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient(
		testRunningPod("nginx-1", "default", map[string]any{"app": "nginx"}),
		testRunningPod("nginx-2", "default", map[string]any{"app": "nginx"}),
		testRunningPod("redis-1", "default", map[string]any{"app": "redis"}),
	)
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	// The fake clientset can't stream logs, so the handler will try and
	// fail to stream for each matched pod. But we can verify that it found
	// the right pods by checking the error mentions both nginx pods but not redis.
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":     "default",
		"labelSelector": "app=nginx",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	// Should attempt logs for nginx-1 and nginx-2 (error or success).
	if !strings.Contains(text, "nginx-1") || !strings.Contains(text, "nginx-2") {
		t.Errorf("expected both nginx pods in output, got: %s", text)
	}
	// Should NOT include redis.
	if strings.Contains(text, "redis-1") {
		t.Errorf("should not include redis pod, got: %s", text)
	}
}

func TestGetLogs_LabelSelectorContextNotAllowed(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}}
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":     "default",
		"labelSelector": "app=nginx",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for disallowed context")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected not-allowed error, got: %s", text)
	}
}

func TestGetLogs_SinceAndSinceTimeMutuallyExclusive(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"since":     "5m",
		"sinceTime": "2024-01-15T10:00:00Z",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error when both since and sinceTime are provided")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "mutually exclusive") {
		t.Errorf("expected mutual exclusion error, got: %s", text)
	}
}

func TestGetLogs_SinceTimeInvalidFormat(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"sinceTime": "not-a-date",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for invalid sinceTime format")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "RFC3339") {
		t.Errorf("expected RFC3339 format error, got: %s", text)
	}
}

func TestGetLogs_PodStillWorksAlone(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	// Pod-only path should still work (will error on streaming, but not on validation).
	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
	}))
	if err != nil {
		t.Fatal(err)
	}
	// The fake fails on streaming but should NOT fail on validation.
	text := resultText(t, res)
	if strings.Contains(text, "pod") && strings.Contains(text, "labelSelector") {
		t.Errorf("should not get validation error when pod is provided, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// Tests for follow/streaming support (issue #14)
// ---------------------------------------------------------------------------

func TestGetLogs_FollowAndTailConflict(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"follow":    true,
		"tail":      float64(10),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error when follow=true and tail are both set")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "tail") || !strings.Contains(text, "follow") {
		t.Errorf("expected error mentioning follow and tail, got: %s", text)
	}
}

func TestGetLogs_FollowTimeoutTooLow(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":     "default",
		"pod":           "my-pod",
		"follow":        true,
		"followTimeout": float64(0),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error when followTimeout=0 (out of range)")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "followTimeout") {
		t.Errorf("expected error mentioning followTimeout, got: %s", text)
	}
}

func TestGetLogs_FollowTimeoutTooHigh(t *testing.T) {
	cfg := defaultCfg()
	fakeCS := fake.NewClientset()
	dynClient := newFakeDynClient()
	pool := buildPool(cfg, defaultRawConfig(), dynClient, fakeCS)

	handler := getHandler(t, "get_logs", func(s *server.MCPServer) {
		registerGetLogs(s, pool)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace":     "default",
		"pod":           "my-pod",
		"follow":        true,
		"followTimeout": float64(200),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error when followTimeout=200 (out of range)")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "followTimeout") {
		t.Errorf("expected error mentioning followTimeout, got: %s", text)
	}
}

// TestGetLogs_ReadFollowStream_Basic verifies that readFollowStream reads lines
// from a stream that closes naturally.
func TestGetLogs_ReadFollowStream_Basic(t *testing.T) {
	logBody := "line-one\nline-two\nline-three\n"
	pr, pw := io.Pipe()
	go func() {
		_, _ = fmt.Fprint(pw, logBody)
		_ = pw.Close()
	}()

	lines, truncated, err := readFollowStream(pr, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if truncated {
		t.Error("expected no truncation for small stream")
	}
	result := strings.Join(lines, "\n")
	if !strings.Contains(result, "line-one") || !strings.Contains(result, "line-two") {
		t.Errorf("expected all lines in output, got: %s", result)
	}
}

// TestGetLogs_ReadFollowStream_Timeout verifies readFollowStream returns after
// the timeout even if the stream doesn't close.
func TestGetLogs_ReadFollowStream_Timeout(t *testing.T) {
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()

	// Write a couple of lines then block (don't close).
	go func() {
		_, _ = fmt.Fprintln(pw, "line-before-timeout")
	}()

	start := time.Now()
	lines, _, _ := readFollowStream(pr, 200*time.Millisecond)
	elapsed := time.Since(start)

	// Should return in roughly 200ms, not hang.
	if elapsed > 2*time.Second {
		t.Errorf("readFollowStream took too long: %v", elapsed)
	}
	result := strings.Join(lines, "\n")
	if !strings.Contains(result, "line-before-timeout") {
		t.Errorf("expected accumulated lines before timeout, got: %q", result)
	}
}

// TestGetLogs_ReadFollowStream_BufferCapLines verifies that >10000 lines triggers truncation.
func TestGetLogs_ReadFollowStream_BufferCapLines(t *testing.T) {
	pr, pw := io.Pipe()
	go func() {
		defer func() { _ = pw.Close() }()
		for i := range 10100 {
			_, _ = fmt.Fprintf(pw, "log line %d\n", i)
		}
	}()

	lines, truncated, _ := readFollowStream(pr, 5*time.Second)
	if !truncated {
		t.Error("expected truncation flag when > 10000 lines")
	}
	if len(lines) > 10000 {
		t.Errorf("expected at most 10000 lines, got %d", len(lines))
	}
}

// TestGetLogs_ReadFollowStream_BufferCapBytes verifies that >= 1MB triggers truncation.
func TestGetLogs_ReadFollowStream_BufferCapBytes(t *testing.T) {
	pr, pw := io.Pipe()
	go func() {
		defer func() { _ = pw.Close() }()
		// Write lines that total > 1MB.
		line := strings.Repeat("x", 1000) + "\n"
		for range 1100 {
			_, _ = fmt.Fprint(pw, line)
		}
	}()

	_, truncated, _ := readFollowStream(pr, 5*time.Second)
	if !truncated {
		t.Error("expected truncation flag when > 1MB read")
	}
}

// repeatReader is an infinite, non-blocking io.Reader that endlessly repeats
// data. It never returns EOF and never blocks, so the scanning goroutine in
// readFollowStream is only ever scanning or parked on a channel send — exactly
// the condition under which the timeout/cap return path must not leak it.
type repeatReader struct {
	data []byte
	off  int
}

func (r *repeatReader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		if r.off >= len(r.data) {
			r.off = 0
		}
		c := copy(p[n:], r.data[r.off:])
		n += c
		r.off += c
	}
	return n, nil
}

// TestGetLogs_ReadFollowStream_NoGoroutineLeak verifies that the scanning
// goroutine exits after readFollowStream returns, even when the source never
// closes and the goroutine is parked on a channel send at return time.
func TestGetLogs_ReadFollowStream_NoGoroutineLeak(t *testing.T) {
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	r := &repeatReader{data: []byte("a line of log output\n")}
	lines, _, _ := readFollowStream(r, 50*time.Millisecond)
	if len(lines) == 0 {
		t.Fatal("expected to accumulate at least one line")
	}

	// The internal scanning goroutine must terminate shortly after the call
	// returns. With the leak it stays blocked on the channel send forever.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if runtime.NumGoroutine() <= baseline {
			return // goroutine cleaned up
		}
		if time.Now().After(deadline) {
			t.Fatalf("scanning goroutine leaked: goroutines=%d, baseline=%d",
				runtime.NumGoroutine(), baseline)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
