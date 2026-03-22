package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/tamcore/kubectl-mcp/internal/kube"
	"github.com/tamcore/kubectl-mcp/internal/mcplog"
	"github.com/tamcore/kubectl-mcp/internal/resources"
	"github.com/tamcore/kubectl-mcp/internal/tools"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		pool, err := kube.NewClientPool(&cfg)
		if err != nil {
			return fmt.Errorf("failed to initialize kube client pool: %w", err)
		}

		// Use a pointer so hooks can reference the server after creation.
		var s *server.MCPServer
		logLevel, _ := mcplog.ParseLogLevel(cfg.LogLevel)

		// Set up file-based logger (skip when logging is off).
		var logger *log.Logger
		if logLevel != mcplog.LogLevelOff {
			logFile, err := openLogFile(cfg.LogFile)
			if err != nil {
				return fmt.Errorf("failed to open log file %q: %w", cfg.LogFile, err)
			}
			defer func() { _ = logFile.Close() }()
			logger = log.New(logFile, "", log.LstdFlags)
			logger.Printf("kubectl-mcp %s started (log-level=%s)", appVersion, logLevel)
		}

		hooks := newLoggingHooks(&s, logLevel, logger)

		if logLevel == mcplog.LogLevelDebug {
			pool.SetTransportWrapper(func(rt http.RoundTripper) http.RoundTripper {
				return mcplog.NewLoggingTransport(rt, logger)
			})
		}

		s = server.NewMCPServer(
			"kubectl-mcp",
			appVersion,
			server.WithInstructions(serverInstructions()),
			server.WithToolCapabilities(false),
			server.WithResourceCapabilities(false, false),
			server.WithLogging(),
			server.WithElicitation(),
			server.WithHooks(hooks),
		)

		tools.RegisterAll(s, pool, &cfg)
		resources.RegisterAll(s, pool, &cfg)

		switch cfg.Transport {
		case "stdio":
			return server.ServeStdio(s)
		case "sse":
			sseServer := server.NewSSEServer(s)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Starting SSE server on %s\n", cfg.SSEAddress)
			return sseServer.Start(cfg.SSEAddress)
		case "streamable-http":
			httpServer := server.NewStreamableHTTPServer(s)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Starting streamable-HTTP server on %s\n", cfg.HTTPAddress)
			return httpServer.Start(cfg.HTTPAddress)
		default:
			return fmt.Errorf("unknown transport: %s", cfg.Transport)
		}
	},
}

func newLoggingHooks(sPtr **server.MCPServer, level mcplog.LogLevel, logger *log.Logger) *server.Hooks {
	hooks := &server.Hooks{}

	if level == mcplog.LogLevelOff {
		return hooks
	}

	hooks.AddBeforeCallTool(func(ctx context.Context, id any, req *mcp.CallToolRequest) {
		var args string
		if level == mcplog.LogLevelDebug {
			args = fullArgs(req.GetArguments())
		} else {
			args = summarizeArgs(req.GetArguments())
		}
		logger.Printf("→ %s(%s)", req.Params.Name, args)

		if level == mcplog.LogLevelDebug && *sPtr != nil {
			_ = (*sPtr).SendLogMessageToClient(ctx, mcplog.NewNotification(
				mcp.LoggingLevelDebug,
				fmt.Sprintf("→ %s(%s)", req.Params.Name, args),
			))
		}
	})

	hooks.AddAfterCallTool(func(ctx context.Context, id any, req *mcp.CallToolRequest, result any) {
		r, ok := result.(*mcp.CallToolResult)
		if !ok {
			return
		}

		if r.IsError {
			logger.Printf("✗ %s failed", req.Params.Name)
			if *sPtr != nil {
				errText := extractErrorText(r)
				_ = (*sPtr).SendLogMessageToClient(ctx, mcplog.NewNotification(
					mcp.LoggingLevelWarning,
					fmt.Sprintf("%s failed: %s", req.Params.Name, errText),
				))
			}
			return
		}

		if level == mcplog.LogLevelDebug {
			text := extractResultText(r)
			logger.Printf("✓ %s done (%dB)\n%s", req.Params.Name, len(text), text)
			if *sPtr != nil {
				_ = (*sPtr).SendLogMessageToClient(ctx, mcplog.NewNotification(
					mcp.LoggingLevelDebug,
					fmt.Sprintf("✓ %s result: %s", req.Params.Name, text),
				))
			}
		} else {
			logger.Printf("✓ %s done", req.Params.Name)
		}
	})

	hooks.AddOnError(func(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
		logger.Printf("✗ error in %s: %v", method, err)

		if *sPtr != nil {
			_ = (*sPtr).SendLogMessageToClient(ctx, mcplog.NewNotification(
				mcp.LoggingLevelError,
				fmt.Sprintf("error in %s: %v", method, err),
			))
		}
	})

	return hooks
}

func extractErrorText(r *mcp.CallToolResult) string {
	if len(r.Content) == 0 {
		return ""
	}
	if tc, ok := r.Content[0].(mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

func summarizeArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "..."
	}
	s := string(b)
	// Trim the outer braces for readability.
	if len(s) > 2 {
		s = s[1 : len(s)-1]
	}
	const maxLen = 120
	if len(s) > maxLen {
		s = s[:maxLen] + "…"
	}
	return s
}

func fullArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "..."
	}
	s := string(b)
	if len(s) > 2 {
		s = s[1 : len(s)-1]
	}
	return s
}

func extractResultText(r *mcp.CallToolResult) string {
	if len(r.Content) == 0 {
		return ""
	}
	var parts []string
	for _, c := range r.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// openLogFile creates the parent directory if needed and opens the log file
// for appending. The caller is responsible for closing the returned file.
func openLogFile(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating log directory %q: %w", dir, err)
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
}
