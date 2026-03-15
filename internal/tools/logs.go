package tools

import (
	"context"
	"fmt"
	"io"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	corev1 "k8s.io/api/core/v1"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerGetLogs(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("get_logs",
		mcp.WithDescription("Get logs from a Kubernetes pod"),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace of the pod"),
		),
		mcp.WithString("pod",
			mcp.Required(),
			mcp.Description("Pod name"),
		),
		mcp.WithString("container",
			mcp.Description("Container name (required if pod has multiple containers)"),
		),
		mcp.WithNumber("tail",
			mcp.Description("Number of lines from the end to return (default 100)"),
		),
		mcp.WithString("since",
			mcp.Description("Only return logs newer than a relative duration (e.g. 5m, 1h)"),
		),
		mcp.WithBoolean("previous",
			mcp.Description("Return logs from previous terminated container"),
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

		namespace, _ := req.RequireString("namespace")
		pod, _ := req.RequireString("pod")
		container := req.GetString("container", "")
		previous := req.GetBool("previous", false)

		tailLines := int64(req.GetFloat("tail", 100))

		opts := &corev1.PodLogOptions{
			TailLines: &tailLines,
			Previous:  previous,
		}
		if container != "" {
			opts.Container = container
		}

		sinceStr := req.GetString("since", "")
		if sinceStr != "" {
			d, err := parseDuration(sinceStr)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid since duration: %v", err)), nil
			}
			secs := int64(d.Seconds())
			opts.SinceSeconds = &secs
		}

		stream, err := cc.Clientset.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get logs: %v", err)), nil
		}
		defer func() { _ = stream.Close() }()

		data, err := io.ReadAll(stream)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to read logs: %v", err)), nil
		}

		if len(data) == 0 {
			return mcp.NewToolResultText("(no logs)"), nil
		}

		return mcp.NewToolResultText(string(data)), nil
	})
}
