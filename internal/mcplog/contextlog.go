package mcplog

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ContextLogWriter manages per-kubecontext log files in a PID-scoped
// directory. Each context gets its own log file, and a server.log is
// used for lifecycle events (startup, shutdown, errors without context).
type ContextLogWriter struct {
	dir     string // PID-scoped session directory (<base>/<pid>)
	mu      sync.RWMutex
	loggers map[string]*log.Logger
	files   []*os.File
	main    *log.Logger
}

// NewContextLogWriter creates a ContextLogWriter that stores per-context
// logs under dir/<pid>/. The directory is created if it does not exist.
func NewContextLogWriter(dir string) (*ContextLogWriter, error) {
	return NewContextLogWriterWithPID(dir, os.Getpid())
}

// NewContextLogWriterWithPID is like NewContextLogWriter but accepts an
// explicit PID value (useful for testing).
func NewContextLogWriterWithPID(dir string, pid int) (*ContextLogWriter, error) {
	sessionDir := filepath.Join(dir, fmt.Sprintf("%d", pid))
	if err := os.MkdirAll(sessionDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating log directory %q: %w", sessionDir, err)
	}

	serverFile, err := openFile(filepath.Join(sessionDir, "server.log"))
	if err != nil {
		return nil, fmt.Errorf("opening server log: %w", err)
	}

	return &ContextLogWriter{
		dir:     sessionDir,
		loggers: make(map[string]*log.Logger),
		files:   []*os.File{serverFile},
		main:    log.New(serverFile, "", log.LstdFlags),
	}, nil
}

// Dir returns the PID-scoped session directory path.
func (w *ContextLogWriter) Dir() string {
	return w.dir
}

// MainLogger returns the server lifecycle logger (writes to server.log).
func (w *ContextLogWriter) MainLogger() *log.Logger {
	return w.main
}

// LoggerFor returns (or lazily creates) a logger for the given context.
// The context name is sanitized for use as a filename.
func (w *ContextLogWriter) LoggerFor(contextName string) *log.Logger {
	safe := sanitizeContext(contextName)

	w.mu.RLock()
	if l, ok := w.loggers[safe]; ok {
		w.mu.RUnlock()
		return l
	}
	w.mu.RUnlock()

	w.mu.Lock()
	defer w.mu.Unlock()

	// Double-check after acquiring write lock.
	if l, ok := w.loggers[safe]; ok {
		return l
	}

	path := filepath.Join(w.dir, safe+".log")
	f, err := openFile(path)
	if err != nil {
		// Fall back to main logger if file creation fails.
		w.main.Printf("failed to open context log %q: %v", path, err)
		w.loggers[safe] = w.main
		return w.main
	}

	w.files = append(w.files, f)
	l := log.New(f, "", log.LstdFlags)
	w.loggers[safe] = l
	return l
}

// Close closes all open log files.
func (w *ContextLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var firstErr error
	for _, f := range w.files {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	w.files = nil
	w.loggers = nil
	return firstErr
}

// SanitizeContext replaces characters unsafe for filenames with underscores.
// Only alphanumeric, dash, underscore, and dot are kept.
func SanitizeContext(name string) string {
	return sanitizeContext(name)
}

// sanitizeContext replaces characters unsafe for filenames with underscores.
func sanitizeContext(name string) string {
	if name == "" {
		return "_default"
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if isFilenameSafe(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func isFilenameSafe(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_' || r == '.'
}

func openFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
}
