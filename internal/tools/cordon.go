package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

var nodeGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}

func registerCordonNode(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	tool := mcp.NewTool("cordon_node",
		mcp.WithDescription("Mark a Kubernetes node as unschedulable (cordon). Requires --allow-write."),
		mcp.WithReadOnlyHintAnnotation(false),
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
	)

	s.AddTool(tool, cordonHandler(pool, cfg, true))
}

func registerUncordonNode(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	tool := mcp.NewTool("uncordon_node",
		mcp.WithDescription("Mark a Kubernetes node as schedulable (uncordon). Requires --allow-write."),
		mcp.WithReadOnlyHintAnnotation(false),
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
	)

	s.AddTool(tool, cordonHandler(pool, cfg, false))
}

func cordonHandler(pool *kube.ClientPool, cfg *config.Config, cordon bool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctxName, err := pool.ResolveContext(req.GetString("context", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		cc, err := pool.ClientFor(ctxName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get client: %v", err)), nil
		}

		node, _ := req.RequireString("node")

		if err := applySafetyDelay(ctx, req, cfg.SafetyDelayWrite); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("safety delay interrupted: %v", err)), nil
		}

		patch := fmt.Sprintf(`{"spec":{"unschedulable":%t}}`, cordon)
		_, err = cc.Dynamic.Resource(nodeGVR).Patch(
			ctx, node, types.MergePatchType, []byte(patch), metav1.PatchOptions{},
		)
		if err != nil {
			action := "cordon"
			if !cordon {
				action = "uncordon"
			}
			return mcp.NewToolResultError(fmt.Sprintf("failed to %s node %q: %v", action, node, err)), nil
		}

		action := "Cordoned"
		if !cordon {
			action = "Uncordoned"
		}
		return mcp.NewToolResultText(fmt.Sprintf("%s node %q (context: %s)", action, node, ctxName)), nil
	}
}
