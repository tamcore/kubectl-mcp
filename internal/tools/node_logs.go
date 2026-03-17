package tools

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerNodeLogs(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("node_logs",
		mcp.WithDescription("Get logs from a Kubernetes node via the kubelet proxy"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("node",
			mcp.Required(),
			mcp.Description("Node name"),
		),
		mcp.WithString("logPath",
			mcp.Description("Log file path under /var/log on the node (e.g. 'syslog', 'journal'). Defaults to root listing."),
		),
		mcp.WithNumber("tail",
			mcp.Description("Number of lines from the end to return (passed as query parameter)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctxName, err := pool.ResolveContext(req.GetString("context", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		cc, err := pool.ClientFor(ctxName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get client: %v", err)), nil
		}

		node, _ := req.RequireString("node")
		logPath := req.GetString("logPath", "")
		tail := int64(req.GetFloat("tail", 0))

		// Validate logPath to prevent path traversal.
		if err := validateLogPath(logPath); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		absPath := "/api/v1/nodes/" + node + "/proxy/logs/"
		if logPath != "" {
			absPath += logPath
		}

		restReq := cc.Clientset.CoreV1().RESTClient().Get().AbsPath(absPath)
		if tail > 0 {
			restReq.Param("tailLines", fmt.Sprintf("%d", tail))
		}

		stream, err := restReq.Stream(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get logs from node %q: %v", node, err)), nil
		}
		defer func() { _ = stream.Close() }()

		data, err := io.ReadAll(stream)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to read node logs: %v", err)), nil
		}

		if len(data) == 0 {
			return mcp.NewToolResultText("(no logs)"), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})
}

// validateLogPath checks for path traversal attempts in the log path.
func validateLogPath(p string) error {
	if p == "" {
		return nil
	}
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("logPath must be a relative path — path traversal is not allowed")
	}
	if strings.Contains(p, "..") {
		return fmt.Errorf("logPath must not contain '..' — path traversal is not allowed")
	}
	return nil
}
