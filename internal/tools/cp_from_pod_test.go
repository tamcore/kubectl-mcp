package tools

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/tamcore/kubectl-mcp/internal/config"
)

// fakeTarExecRunner implements ExecRunner, writing canned tar bytes to stdout.
type fakeTarExecRunner struct {
	tarContent []byte
	execErr    string
	captureCmd *[]string
	captureCtr *string
}

func (f *fakeTarExecRunner) Run(_ context.Context, _ kubernetes.Interface, _ *rest.Config, _, _, container string, command []string, stdout, stderr *bytes.Buffer) error {
	if f.captureCmd != nil {
		*f.captureCmd = append((*f.captureCmd)[:0], command...)
	}
	if f.captureCtr != nil {
		*f.captureCtr = container
	}
	if f.execErr != "" {
		stderr.WriteString(f.execErr)
		return fmt.Errorf("%s", f.execErr)
	}
	stdout.Write(f.tarContent)
	return nil
}

// buildTar creates an in-memory tar archive with a single named file.
func buildTar(name string, content []byte) []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(content)),
	})
	_, _ = tw.Write(content)
	_ = tw.Close()
	return buf.Bytes()
}

func TestCopyFromPod_TextFile(t *testing.T) {
	cfg := defaultCfg()
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	tarBytes := buildTar("etc/myconfig", []byte("hello world\n"))
	runner := &fakeTarExecRunner{tarContent: tarBytes}
	handler := getHandler(t, "copy_from_pod", func(s *server.MCPServer) {
		registerCopyFromPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"src_path":  "/etc/myconfig",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "hello world") {
		t.Errorf("expected file content in output, got: %s", text)
	}
	if !strings.Contains(text, "encoding: text") {
		t.Errorf("expected text encoding, got: %s", text)
	}
}

func TestCopyFromPod_BinaryFile(t *testing.T) {
	cfg := defaultCfg()
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	binaryContent := []byte{0x00, 0x01, 0x02, 0xff, 0xfe}
	tarBytes := buildTar("data/binary.bin", binaryContent)
	runner := &fakeTarExecRunner{tarContent: tarBytes}
	handler := getHandler(t, "copy_from_pod", func(s *server.MCPServer) {
		registerCopyFromPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"src_path":  "/data/binary.bin",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "encoding: base64") {
		t.Errorf("expected base64 encoding for binary file, got: %s", text)
	}
}

func TestCopyFromPod_EmptyFile(t *testing.T) {
	cfg := defaultCfg()
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	tarBytes := buildTar("empty.txt", []byte{})
	runner := &fakeTarExecRunner{tarContent: tarBytes}
	handler := getHandler(t, "copy_from_pod", func(s *server.MCPServer) {
		registerCopyFromPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"src_path":  "/empty.txt",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "0 bytes") {
		t.Errorf("expected 0-byte indication, got: %s", text)
	}
}

func TestCopyFromPod_MissingNamespace(t *testing.T) {
	cfg := defaultCfg()
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeTarExecRunner{}
	handler := getHandler(t, "copy_from_pod", func(s *server.MCPServer) {
		registerCopyFromPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"pod":      "my-pod",
		"src_path": "/etc/config",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "namespace") {
		t.Errorf("expected namespace error, got: %s", text)
	}
}

func TestCopyFromPod_MissingPod(t *testing.T) {
	cfg := defaultCfg()
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeTarExecRunner{}
	handler := getHandler(t, "copy_from_pod", func(s *server.MCPServer) {
		registerCopyFromPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"src_path":  "/etc/config",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "pod") {
		t.Errorf("expected pod error, got: %s", text)
	}
}

func TestCopyFromPod_MissingSrcPath(t *testing.T) {
	cfg := defaultCfg()
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeTarExecRunner{}
	handler := getHandler(t, "copy_from_pod", func(s *server.MCPServer) {
		registerCopyFromPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "src_path") {
		t.Errorf("expected src_path error, got: %s", text)
	}
}

func TestCopyFromPod_ContextResolutionFailure(t *testing.T) {
	cfg := &config.Config{AllowedContexts: []string{"other"}}
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeTarExecRunner{}
	handler := getHandler(t, "copy_from_pod", func(s *server.MCPServer) {
		registerCopyFromPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"src_path":  "/etc/config",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "not allowed") {
		t.Errorf("expected context error, got: %s", text)
	}
}

func TestCopyFromPod_ExecError(t *testing.T) {
	cfg := defaultCfg()
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeTarExecRunner{execErr: "no such file or directory"}
	handler := getHandler(t, "copy_from_pod", func(s *server.MCPServer) {
		registerCopyFromPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"src_path":  "/nonexistent",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "no such file") {
		t.Errorf("expected exec error in output, got: %s", text)
	}
}

func TestCopyFromPod_InvalidTar(t *testing.T) {
	cfg := defaultCfg()
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	runner := &fakeTarExecRunner{tarContent: []byte("this is not a tar file")}
	handler := getHandler(t, "copy_from_pod", func(s *server.MCPServer) {
		registerCopyFromPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"src_path":  "/etc/config",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "failed to read tar") {
		t.Errorf("expected tar error in output, got: %s", text)
	}
}

func TestCopyFromPod_TruncatedTar(t *testing.T) {
	cfg := defaultCfg()
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	// Build a valid tar for 100-byte content, then truncate it so the
	// header is present but the content is incomplete.
	full := buildTar("file.txt", bytes.Repeat([]byte("x"), 100))
	truncated := full[:512+10] // header (512) + only 10 of 100 content bytes
	runner := &fakeTarExecRunner{tarContent: truncated}
	handler := getHandler(t, "copy_from_pod", func(s *server.MCPServer) {
		registerCopyFromPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"src_path":  "/file.txt",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "failed to read tar") {
		t.Errorf("expected tar read error, got: %s", text)
	}
}

func TestCopyFromPod_WithContainer(t *testing.T) {
	cfg := defaultCfg()
	pool := buildWritePool(cfg, newWriteFakeDynClient(), fake.NewClientset())

	tarBytes := buildTar("cfg", []byte("data"))
	var capturedContainer string
	runner := &fakeTarExecRunner{tarContent: tarBytes, captureCtr: &capturedContainer}
	handler := getHandler(t, "copy_from_pod", func(s *server.MCPServer) {
		registerCopyFromPod(s, pool, runner, cfg)
	})

	_, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"src_path":  "/cfg",
		"container": "sidecar",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if capturedContainer != "sidecar" {
		t.Errorf("expected container=sidecar, got: %q", capturedContainer)
	}
}

func TestCopyFromPod_FileTooLarge(t *testing.T) {
	// Build a tar with a file that exceeds maxCopyBytes.
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	overLimit := int64(maxCopyBytes) + 1
	_ = tw.WriteHeader(&tar.Header{
		Name: "bigfile",
		Mode: 0644,
		Size: overLimit,
	})
	// Write just enough data to trigger the limit check.
	_, _ = tw.Write(make([]byte, overLimit))
	_ = tw.Close()

	runner := &fakeTarExecRunner{tarContent: tarBuf.Bytes()}
	cfg := defaultCfg()
	pool := buildPool(cfg, defaultRawConfig(), newFakeDynClient(), fake.NewClientset())
	handler := getHandler(t, "copy_from_pod", func(s *server.MCPServer) {
		registerCopyFromPod(s, pool, runner, cfg)
	})

	res, err := handler(context.Background(), callToolReq(map[string]any{
		"namespace": "default",
		"pod":       "my-pod",
		"src_path":  "/bigfile",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error for oversized file")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "exceeds") {
		t.Errorf("expected size limit error, got: %s", text)
	}
}

func TestExtractTarFile_ExactLimit(t *testing.T) {
	// A file exactly at the limit should succeed.
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	data := make([]byte, maxCopyBytes)
	_ = tw.WriteHeader(&tar.Header{Name: "file", Mode: 0644, Size: int64(len(data))})
	_, _ = tw.Write(data)
	_ = tw.Close()

	got, err := extractTarFile(&tarBuf)
	if err != nil {
		t.Fatalf("unexpected error at exact limit: %v", err)
	}
	if len(got) != maxCopyBytes {
		t.Errorf("expected %d bytes, got %d", maxCopyBytes, len(got))
	}
}
