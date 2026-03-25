//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamcore/kubectl-mcp/internal/mcplog"
)

func TestPerContextLogFiles(t *testing.T) {
	for _, tc := range allTransports {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("creates_per_context_log_file", func(t *testing.T) {
				logDir := t.TempDir()
				cfg := defaultConfig()
				cfg.LogLevel = "info"
				cfg.LogDir = logDir

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				// Call a tool — should create a per-context log file.
				result := callTool(t, c, "list_namespaces", nil)
				if result.IsError {
					t.Fatalf("list_namespaces failed: %s", resultText(result))
				}

				// Find the PID-scoped subdirectory.
				entries, err := os.ReadDir(logDir)
				if err != nil {
					t.Fatalf("reading log dir: %v", err)
				}
				if len(entries) == 0 {
					t.Fatal("expected PID-scoped subdirectory in log dir, got none")
				}

				sessionDir := filepath.Join(logDir, entries[0].Name())

				// server.log should exist with startup message.
				serverLog := filepath.Join(sessionDir, "server.log")
				serverContent, err := os.ReadFile(serverLog)
				if err != nil {
					t.Fatalf("expected server.log to exist: %v", err)
				}
				if !strings.Contains(string(serverContent), "started") {
					t.Fatalf("server.log missing startup message, got: %s", string(serverContent))
				}

				// A context log file should exist (the default context).
				contextFiles := findLogFiles(t, sessionDir, "server.log")
				if len(contextFiles) == 0 {
					t.Fatal("expected at least one per-context log file, got none")
				}

				// At least one context file should contain the tool call.
				found := false
				for _, cf := range contextFiles {
					content, _ := os.ReadFile(filepath.Join(sessionDir, cf))
					if strings.Contains(string(content), "list_namespaces") {
						found = true
						break
					}
				}
				if !found {
					t.Fatal("no per-context log file contains 'list_namespaces'")
				}
			})

			t.Run("explicit_context_routes_to_correct_file", func(t *testing.T) {
				logDir := t.TempDir()
				cfg := defaultConfig()
				cfg.LogLevel = "info"
				cfg.LogDir = logDir

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				// Get the default context name first.
				ctxResult := callTool(t, c, "list_contexts", nil)
				if ctxResult.IsError {
					t.Fatalf("list_contexts failed: %s", resultText(ctxResult))
				}
				ctxText := resultText(ctxResult)

				// Call with the default context explicitly.
				// The context name in KinD is typically "kind-e2e".
				result := callTool(t, c, "list_namespaces", map[string]any{
					"context": extractDefaultContext(t, ctxText),
				})
				if result.IsError {
					t.Fatalf("list_namespaces failed: %s", resultText(result))
				}

				// Find session dir.
				entries, err := os.ReadDir(logDir)
				if err != nil {
					t.Fatalf("reading log dir: %v", err)
				}
				sessionDir := filepath.Join(logDir, entries[0].Name())

				// The specific context file should contain the tool call.
				ctxName := extractDefaultContext(t, ctxText)
				sanitized := mcplog.SanitizeContext(ctxName)
				contextFile := filepath.Join(sessionDir, sanitized+".log")
				content, err := os.ReadFile(contextFile)
				if err != nil {
					t.Fatalf("expected %s.log to exist: %v", sanitized, err)
				}
				if !strings.Contains(string(content), "list_namespaces") {
					t.Fatalf("context log %s.log missing tool call, got: %s", sanitized, string(content))
				}
			})

			t.Run("debug_level_includes_full_args", func(t *testing.T) {
				logDir := t.TempDir()
				cfg := defaultConfig()
				cfg.LogLevel = "debug"
				cfg.LogDir = logDir

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				result := callTool(t, c, "list_namespaces", nil)
				if result.IsError {
					t.Fatalf("list_namespaces failed: %s", resultText(result))
				}

				// Find session dir.
				entries, err := os.ReadDir(logDir)
				if err != nil {
					t.Fatalf("reading log dir: %v", err)
				}
				sessionDir := filepath.Join(logDir, entries[0].Name())

				// Context log should contain result content at debug level.
				contextFiles := findLogFiles(t, sessionDir, "server.log")
				found := false
				for _, cf := range contextFiles {
					content, _ := os.ReadFile(filepath.Join(sessionDir, cf))
					if strings.Contains(string(content), "done") && strings.Contains(string(content), "list_namespaces") {
						found = true
						break
					}
				}
				if !found {
					t.Fatal("no per-context log contains debug output for list_namespaces")
				}
			})

			t.Run("off_level_no_log_files", func(t *testing.T) {
				logDir := t.TempDir()
				cfg := defaultConfig()
				cfg.LogLevel = "off"
				cfg.LogDir = logDir

				base := tc.startFunc(t, cfg)
				c := tc.clientFunc(t, base)

				result := callTool(t, c, "list_namespaces", nil)
				if result.IsError {
					t.Fatalf("list_namespaces failed: %s", resultText(result))
				}

				// No PID subdirectory should be created.
				entries, err := os.ReadDir(logDir)
				if err != nil {
					t.Fatalf("reading log dir: %v", err)
				}
				if len(entries) > 0 {
					t.Fatalf("expected no subdirectories at off level, got %d", len(entries))
				}
			})
		})
	}
}

// findLogFiles returns .log files in dir, excluding the named file.
func findLogFiles(t *testing.T, dir, exclude string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir %s: %v", dir, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") || e.Name() == exclude {
			continue
		}
		files = append(files, e.Name())
	}
	return files
}

// extractDefaultContext parses the JSON output of list_contexts and returns
// the context marked as default.
func extractDefaultContext(t *testing.T, text string) string {
	t.Helper()
	// The output is a JSON array of {name, isDefault} objects.
	type ctxInfo struct {
		Name      string `json:"name"`
		IsDefault bool   `json:"isDefault"`
	}
	var contexts []ctxInfo
	if err := parseJSON(text, &contexts); err != nil {
		t.Fatalf("failed to parse contexts: %v", err)
	}
	for _, c := range contexts {
		if c.IsDefault {
			return c.Name
		}
	}
	// If no default found, return the first context.
	if len(contexts) > 0 {
		return contexts[0].Name
	}
	t.Fatal("no contexts found")
	return ""
}

func parseJSON(text string, v any) error {
	return json.Unmarshal([]byte(text), v)
}
