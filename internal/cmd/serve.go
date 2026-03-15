package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/tamcore/kubectl-mcp/internal/kube"
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

		hooks := newLoggingHooks()

		s := server.NewMCPServer(
			"kubectl-mcp",
			"0.1.0",
			server.WithToolCapabilities(false),
			server.WithHooks(hooks),
		)

		tools.RegisterAll(s, pool, &cfg)

		switch cfg.Transport {
		case "stdio":
			return server.ServeStdio(s)
		case "sse":
			sseServer := server.NewSSEServer(s)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Starting SSE server on %s\n", cfg.SSEAddress)
			return sseServer.Start(cfg.SSEAddress)
		default:
			return fmt.Errorf("unknown transport: %s", cfg.Transport)
		}
	},
}

func newLoggingHooks() *server.Hooks {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	hooks := &server.Hooks{}

	hooks.AddBeforeCallTool(func(ctx context.Context, id any, req *mcp.CallToolRequest) {
		args := summarizeArgs(req.GetArguments())
		logger.Printf("→ %s(%s)", req.Params.Name, args)
	})

	hooks.AddAfterCallTool(func(ctx context.Context, id any, req *mcp.CallToolRequest, result any) {
		if r, ok := result.(*mcp.CallToolResult); ok && r.IsError {
			logger.Printf("✗ %s failed", req.Params.Name)
		} else {
			logger.Printf("✓ %s done", req.Params.Name)
		}
	})

	hooks.AddOnError(func(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
		logger.Printf("✗ error in %s: %v", method, err)
	})

	return hooks
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
