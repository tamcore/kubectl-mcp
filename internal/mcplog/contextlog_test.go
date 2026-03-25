package mcplog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSanitizeContext(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"kind-e2e", "kind-e2e"},
		{"my-context", "my-context"},
		{"arn:aws:eks:us-east-1:123456:cluster/prod", "arn_aws_eks_us-east-1_123456_cluster_prod"},
		{"context/with/slashes", "context_with_slashes"},
		{"context\\backslash", "context_backslash"},
		{"dots.are.ok", "dots.are.ok"},
		{"under_scores", "under_scores"},
		{"spaces not ok", "spaces_not_ok"},
		{"MiXeD-CaSe", "MiXeD-CaSe"},
		{"", "_default"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeContext(tt.input)
			if got != tt.want {
				t.Fatalf("sanitizeContext(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewContextLogWriter_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	clw, err := NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter(%q) error: %v", dir, err)
	}
	defer func() { _ = clw.Close() }()

	// Should have created a PID-scoped subdirectory.
	sessionDir := clw.Dir()
	if !strings.HasPrefix(sessionDir, dir) {
		t.Fatalf("Dir() = %q, expected prefix %q", sessionDir, dir)
	}

	info, err := os.Stat(sessionDir)
	if err != nil {
		t.Fatalf("session directory does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("session directory is not a directory")
	}
}

func TestNewContextLogWriter_DirContainsPID(t *testing.T) {
	dir := t.TempDir()
	clw, err := NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	pid := fmt.Sprintf("%d", os.Getpid())
	if !strings.Contains(clw.Dir(), pid) {
		t.Fatalf("Dir() = %q, expected to contain PID %s", clw.Dir(), pid)
	}
}

func TestContextLogWriter_MainLogger(t *testing.T) {
	dir := t.TempDir()
	clw, err := NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	logger := clw.MainLogger()
	if logger == nil {
		t.Fatal("MainLogger() returned nil")
	}

	logger.Println("startup message")

	serverLog := filepath.Join(clw.Dir(), "server.log")
	content, err := os.ReadFile(serverLog)
	if err != nil {
		t.Fatalf("failed to read server.log: %v", err)
	}
	if !strings.Contains(string(content), "startup message") {
		t.Fatalf("server.log missing expected content, got: %s", string(content))
	}
}

func TestContextLogWriter_LoggerFor_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	clw, err := NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	logger := clw.LoggerFor("kind-e2e")
	if logger == nil {
		t.Fatal("LoggerFor() returned nil")
	}

	logger.Println("tool call for kind-e2e")

	logFile := filepath.Join(clw.Dir(), "kind-e2e.log")
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read context log: %v", err)
	}
	if !strings.Contains(string(content), "tool call for kind-e2e") {
		t.Fatalf("context log missing expected content, got: %s", string(content))
	}
}

func TestContextLogWriter_LoggerFor_CachesLogger(t *testing.T) {
	dir := t.TempDir()
	clw, err := NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	l1 := clw.LoggerFor("ctx-a")
	l2 := clw.LoggerFor("ctx-a")

	if l1 != l2 {
		t.Fatal("LoggerFor should return the same logger for the same context")
	}
}

func TestContextLogWriter_LoggerFor_SeparatesContexts(t *testing.T) {
	dir := t.TempDir()
	clw, err := NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	clw.LoggerFor("ctx-alpha").Println("alpha message")
	clw.LoggerFor("ctx-beta").Println("beta message")

	alphaContent, err := os.ReadFile(filepath.Join(clw.Dir(), "ctx-alpha.log"))
	if err != nil {
		t.Fatalf("failed to read ctx-alpha.log: %v", err)
	}
	betaContent, err := os.ReadFile(filepath.Join(clw.Dir(), "ctx-beta.log"))
	if err != nil {
		t.Fatalf("failed to read ctx-beta.log: %v", err)
	}

	if !strings.Contains(string(alphaContent), "alpha message") {
		t.Fatalf("ctx-alpha.log missing expected content")
	}
	if strings.Contains(string(alphaContent), "beta message") {
		t.Fatal("ctx-alpha.log should not contain beta message")
	}
	if !strings.Contains(string(betaContent), "beta message") {
		t.Fatalf("ctx-beta.log missing expected content")
	}
	if strings.Contains(string(betaContent), "alpha message") {
		t.Fatal("ctx-beta.log should not contain alpha message")
	}
}

func TestContextLogWriter_LoggerFor_SanitizesName(t *testing.T) {
	dir := t.TempDir()
	clw, err := NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	clw.LoggerFor("arn:aws:eks:us-east-1:123:cluster/prod").Println("aws message")

	logFile := filepath.Join(clw.Dir(), "arn_aws_eks_us-east-1_123_cluster_prod.log")
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read sanitized log file: %v", err)
	}
	if !strings.Contains(string(content), "aws message") {
		t.Fatalf("sanitized log file missing expected content, got: %s", string(content))
	}
}

func TestContextLogWriter_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	clw, err := NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ctx := fmt.Sprintf("ctx-%d", n%5) // 5 unique contexts
			logger := clw.LoggerFor(ctx)
			logger.Printf("message from goroutine %d", n)
		}(i)
	}
	wg.Wait()

	// Verify all 5 context files were created.
	for i := 0; i < 5; i++ {
		logFile := filepath.Join(clw.Dir(), fmt.Sprintf("ctx-%d.log", i))
		if _, err := os.Stat(logFile); err != nil {
			t.Errorf("expected log file %s to exist: %v", logFile, err)
		}
	}
}

func TestContextLogWriter_Close(t *testing.T) {
	dir := t.TempDir()
	clw, err := NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}

	// Create a few loggers.
	clw.LoggerFor("ctx-a").Println("a")
	clw.LoggerFor("ctx-b").Println("b")
	clw.MainLogger().Println("server")

	if err := clw.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// Verify files exist and have content (written before close).
	for _, name := range []string{"ctx-a.log", "ctx-b.log", "server.log"} {
		content, err := os.ReadFile(filepath.Join(clw.Dir(), name))
		if err != nil {
			t.Errorf("failed to read %s after close: %v", name, err)
		}
		if len(content) == 0 {
			t.Errorf("%s is empty after close", name)
		}
	}
}

func TestNewContextLogWriter_CustomPID(t *testing.T) {
	dir := t.TempDir()
	clw, err := NewContextLogWriterWithPID(dir, 42)
	if err != nil {
		t.Fatalf("NewContextLogWriterWithPID error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	if !strings.HasSuffix(clw.Dir(), "42") {
		t.Fatalf("Dir() = %q, expected suffix '42'", clw.Dir())
	}
}

// TestContextLogWriter_MainLoggerIsNotNil ensures MainLogger is available
// immediately after construction, even before any LoggerFor calls.
func TestContextLogWriter_MainLoggerIsNotNil(t *testing.T) {
	dir := t.TempDir()
	clw, err := NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	if clw.MainLogger() == nil {
		t.Fatal("MainLogger() should never be nil")
	}
}

// TestContextLogWriter_LoggerFor_StandardInterface tests that LoggerFor and
// MainLogger return *log.Logger values.
func TestContextLogWriter_LoggerFor_StandardInterface(t *testing.T) {
	dir := t.TempDir()
	clw, err := NewContextLogWriter(dir)
	if err != nil {
		t.Fatalf("NewContextLogWriter error: %v", err)
	}
	defer func() { _ = clw.Close() }()

	l1 := clw.LoggerFor("test")
	if l1 == nil {
		t.Fatal("LoggerFor returned nil")
	}
	l2 := clw.MainLogger()
	if l2 == nil {
		t.Fatal("MainLogger returned nil")
	}
}
