package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/tamcore/kubectl-mcp/internal/kube"
	"github.com/tamcore/kubectl-mcp/internal/mcplog"
	"github.com/tamcore/kubectl-mcp/internal/prompts"
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

		// Set up per-context log writer (skip when logging is off).
		var clw *mcplog.ContextLogWriter
		if logLevel != mcplog.LogLevelOff {
			clw, err = mcplog.NewContextLogWriter(cfg.LogDir)
			if err != nil {
				return fmt.Errorf("failed to create log writer in %q: %w", cfg.LogDir, err)
			}
			defer func() { _ = clw.Close() }()
			clw.MainLogger().Printf("kubectl-mcp %s started (log-level=%s, log-dir=%s)", appVersion, logLevel, clw.Dir())
		}

		hooks := newLoggingHooks(&s, logLevel, clw, pool)

		if logLevel == mcplog.LogLevelDebug && clw != nil {
			pool.SetTransportWrapper(func(rt http.RoundTripper) http.RoundTripper {
				return mcplog.NewLoggingTransport(rt, clw.MainLogger())
			})
		}

		s = server.NewMCPServer(
			"kubectl-mcp",
			appVersion,
			server.WithInstructions(serverInstructions(&cfg)),
			server.WithToolCapabilities(false),
			server.WithResourceCapabilities(false, false),
			server.WithPromptCapabilities(false),
			server.WithLogging(),
			server.WithElicitation(),
			server.WithHooks(hooks),
		)

		tools.RegisterAll(s, pool, &cfg)
		resources.RegisterAll(s, pool, &cfg)
		prompts.RegisterAll(s)

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

func newLoggingHooks(sPtr **server.MCPServer, level mcplog.LogLevel, clw *mcplog.ContextLogWriter, pool *kube.ClientPool) *server.Hooks {
	hooks := &server.Hooks{}

	if level == mcplog.LogLevelOff || clw == nil {
		return hooks
	}

	hooks.AddBeforeCallTool(func(ctx context.Context, id any, req *mcp.CallToolRequest) {
		logger := resolveLogger(clw, pool, req)
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

		logger := resolveLogger(clw, pool, req)

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
		clw.MainLogger().Printf("✗ error in %s: %v", method, err)

		if *sPtr != nil {
			_ = (*sPtr).SendLogMessageToClient(ctx, mcplog.NewNotification(
				mcp.LoggingLevelError,
				fmt.Sprintf("error in %s: %v", method, err),
			))
		}
	})

	return hooks
}

// resolveLogger extracts the "context" parameter from the tool request
// and returns the per-context logger. Falls back to the default context
// if the parameter is empty.
func resolveLogger(clw *mcplog.ContextLogWriter, pool *kube.ClientPool, req *mcp.CallToolRequest) *log.Logger {
	ctxName := ""
	if args := req.GetArguments(); args != nil {
		if v, ok := args["context"]; ok {
			if s, ok := v.(string); ok {
				ctxName = s
			}
		}
	}
	if ctxName == "" {
		ctxName = pool.DefaultContext()
	}
	return clw.LoggerFor(ctxName)
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
