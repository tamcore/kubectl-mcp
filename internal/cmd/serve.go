package cmd

import (
	"fmt"

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

		s := server.NewMCPServer(
			"kubectl-mcp",
			"0.1.0",
			server.WithToolCapabilities(false),
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
