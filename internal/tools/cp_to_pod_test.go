package tools

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// fakeCopyRunner implements CopyRunner for testing.
type fakeCopyRunner struct {
	execErr       string
	capturedStdin []byte
	captureCmd    *[]string
	captureCtr    *string
}

func (f *fakeCopyRunner) Run(_ context.Context, _ kubernetes.Interface, _ *rest.Config, _, _, container string, command []string, stdin io.Reader, stdout, stderr *bytes.Buffer) error {
	if f.captureCmd != nil {
		*f.captureCmd = append((*f.captureCmd)[:0], command...)
	}
	if f.captureCtr != nil {
		*f.captureCtr = container
	}
	if stdin != nil {
		data, _ := io.ReadAll(stdin)
		f.capturedStdin = data
	}
	if f.execErr != "" {
		stderr.WriteString(f.execErr)
		return fmt.Errorf("%s", f.execErr)
	}
	return nil
}

func TestCopyToPod_TextContent(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeCopyRunner{}
	handler := getHandler(t, "copy_to_pod", func(s *server.MCPServer) {
		registerCopyToPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"dest_path": "/etc/myconfig",
		"content":   "hello world\n",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "copied") {
		t.Errorf("expected success message, got: %s", text)
	}
	if !strings.Contains(text, "my-pod:/etc/myconfig") {
		t.Errorf("expected pod:path in output, got: %s", text)
	}
}

func TestCopyToPod_Base64Content(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	binaryData := []byte{0x00, 0x01, 0x02, 0xff}
	encoded := base64.StdEncoding.EncodeToString(binaryData)

	runner := &fakeCopyRunner{}
	handler := getHandler(t, "copy_to_pod", func(s *server.MCPServer) {
		registerCopyToPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"dest_path": "/data/binary.bin",
		"content":   encoded,
		"encoding":  "base64",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "copied") {
		t.Errorf("expected success message, got: %s", text)
	}

	// Verify the tar sent to the pod contains the decoded binary data.
	tr := tar.NewReader(bytes.NewReader(runner.capturedStdin))
	_, err = tr.Next()
	if err != nil {
		t.Fatalf("could not read tar header: %v", err)
	}
	got, _ := io.ReadAll(tr)
	if !bytes.Equal(got, binaryData) {
		t.Errorf("expected binary data in tar, got: %v", got)
	}
}

func TestCopyToPod_InvalidBase64(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeCopyRunner{}
	handler := getHandler(t, "copy_to_pod", func(s *server.MCPServer) {
		registerCopyToPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"dest_path": "/data/file",
		"content":   "not!!valid!!base64!!!",
		"encoding":  "base64",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "base64") {
		t.Errorf("expected base64 decode error, got: %s", text)
	}
}

func TestCopyToPod_MissingNamespace(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeCopyRunner{}
	handler := getHandler(t, "copy_to_pod", func(s *server.MCPServer) {
		registerCopyToPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"pod":       "my-pod",
		"dest_path": "/etc/config",
		"content":   "data",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "namespace") {
		t.Errorf("expected namespace error, got: %s", text)
	}
}

func TestCopyToPod_MissingPod(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeCopyRunner{}
	handler := getHandler(t, "copy_to_pod", func(s *server.MCPServer) {
		registerCopyToPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"dest_path": "/etc/config",
		"content":   "data",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "pod") {
		t.Errorf("expected pod error, got: %s", text)
	}
}

func TestCopyToPod_MissingDestPath(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeCopyRunner{}
	handler := getHandler(t, "copy_to_pod", func(s *server.MCPServer) {
		registerCopyToPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"content":   "data",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "dest_path") {
		t.Errorf("expected dest_path error, got: %s", text)
	}
}

func TestCopyToPod_MissingContent(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeCopyRunner{}
	handler := getHandler(t, "copy_to_pod", func(s *server.MCPServer) {
		registerCopyToPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"dest_path": "/etc/config",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "content") {
		t.Errorf("expected content error, got: %s", text)
	}
}

func TestCopyToPod_ContextResolutionFailure(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}, AllowWrite: true}
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeCopyRunner{}
	handler := getHandler(t, "copy_to_pod", func(s *server.MCPServer) {
		registerCopyToPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"dest_path": "/etc/config",
		"content":   "data",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected context error, got: %s", text)
	}
}

func TestCopyToPod_ExecError(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeCopyRunner{execErr: "permission denied"}
	handler := getHandler(t, "copy_to_pod", func(s *server.MCPServer) {
		registerCopyToPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"dest_path": "/etc/config",
		"content":   "data",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "permission denied") {
		t.Errorf("expected exec error in output, got: %s", text)
	}
}

func TestCopyToPod_SafetyDelay_Disabled(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.SafetyDelayWrite = 0
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeCopyRunner{}
	handler := getHandler(t, "copy_to_pod", func(s *server.MCPServer) {
		registerCopyToPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"dest_path": "/etc/config",
		"content":   "data",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if runner.capturedStdin == nil {
		t.Error("expected runner to be called even with delay=0")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "copied") {
		t.Errorf("expected success, got: %s", text)
	}
}

func TestCopyToPod_TarContainsCorrectPath(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeCopyRunner{}
	handler := getHandler(t, "copy_to_pod", func(s *server.MCPServer) {
		registerCopyToPod(s, pool, runner, cfg)
	})

	_, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"dest_path": "/etc/myapp/config.yaml",
		"content":   "key: value\n",
	}))
	if err != nil {
		t.Fatal(err)
	}

	tr := tar.NewReader(bytes.NewReader(runner.capturedStdin))
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("could not read tar header: %v", err)
	}
	if hdr.Name != "etc/myapp/config.yaml" {
		t.Errorf("expected tar header name=etc/myapp/config.yaml, got: %q", hdr.Name)
	}
	content, _ := io.ReadAll(tr)
	if string(content) != "key: value\n" {
		t.Errorf("expected content in tar, got: %q", string(content))
	}
}

func TestCopyToPod_SafetyDelayInterrupted(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	cfg.SafetyDelayWrite = 10 * time.Second // long delay
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeCopyRunner{}
	handler := getHandler(t, "copy_to_pod", func(s *server.MCPServer) {
		registerCopyToPod(s, pool, runner, cfg)
	})

	// Cancel the context immediately to interrupt the safety delay.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := handler(ctx, callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"dest_path": "/etc/config",
		"content":   "data",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "safety delay interrupted") {
		t.Errorf("expected safety delay interruption, got: %s", text)
	}
}

func TestCopyToPod_WithContainer(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	var capturedContainer string
	runner := &fakeCopyRunner{captureCtr: &capturedContainer}
	handler := getHandler(t, "copy_to_pod", func(s *server.MCPServer) {
		registerCopyToPod(s, pool, runner, cfg)
	})

	_, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"dest_path": "/cfg",
		"content":   "data",
		"container": "sidecar",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if capturedContainer != "sidecar" {
		t.Errorf("expected container=sidecar, got: %q", capturedContainer)
	}
}

func TestCopyToPod_PathValidation(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowWrite = true
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeCopyRunner{}
	handler := getHandler(t, "copy_to_pod", func(s *server.MCPServer) {
		registerCopyToPod(s, pool, runner, cfg)
	})

	cases := []struct {
		name       string
		destPath   string
		wantErrSub string
	}{
		{"relative path", "etc/passwd", "absolute"},
		{"relative traversal", "../etc/passwd", "absolute"},
		{"dot dot absolute", "/etc/../passwd", ".."},
		{"dot dot only", "/..", ".."},
		{"double dot in middle", "/var/../../../etc/passwd", ".."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := handler(context.Background(), callToolReq(map[string]any{
				"namespace": "default",
				"pod":       "my-pod",
				"dest_path": tc.destPath,
				"content":   "data",
			}))
			if err != nil {
				t.Fatal(err)
			}
			if !res.IsError {
				t.Errorf("expected error for dest_path=%q", tc.destPath)
			}
			text := resultText(t, res)
			if !strings.Contains(text, tc.wantErrSub) {
				t.Errorf("expected %q in error for dest_path=%q, got: %s", tc.wantErrSub, tc.destPath, text)
			}
		})
	}
}

func TestBuildTarBuffer_Success(t *testing.T) {
	reader, err := buildTarBuffer("/etc/config.yaml", []byte("key: value\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := tar.NewReader(reader)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("could not read tar header: %v", err)
	}
	if hdr.Name != "etc/config.yaml" {
		t.Errorf("expected name=etc/config.yaml, got: %q", hdr.Name)
	}
	content, _ := io.ReadAll(tr)
	if string(content) != "key: value\n" {
		t.Errorf("unexpected content: %q", string(content))
	}
}

func TestBuildTarBuffer_EmptyData(t *testing.T) {
	reader, err := buildTarBuffer("/empty", []byte{})
	if err != nil {
		t.Fatalf("unexpected error for empty data: %v", err)
	}
	tr := tar.NewReader(reader)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("could not read tar header: %v", err)
	}
	if hdr.Size != 0 {
		t.Errorf("expected size=0, got: %d", hdr.Size)
	}
}
