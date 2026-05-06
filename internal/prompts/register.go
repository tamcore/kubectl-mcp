package prompts

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterAll registers all MCP prompts for kubectl diagnostic workflows.
func RegisterAll(s *server.MCPServer) {
	s.AddPrompt(mcp.NewPrompt("diagnose-pod",
		mcp.WithPromptDescription("Diagnose a Kubernetes pod: describe, events, and logs in sequence."),
		mcp.WithArgument("pod", mcp.RequiredArgument(), mcp.ArgumentDescription("Pod name")),
		mcp.WithArgument("namespace", mcp.RequiredArgument(), mcp.ArgumentDescription("Namespace")),
		mcp.WithArgument("context", mcp.ArgumentDescription("kubeconfig context (optional)")),
	), diagnosePodHandler)

	s.AddPrompt(mcp.NewPrompt("diagnose-deployment",
		mcp.WithPromptDescription("Diagnose a Kubernetes deployment: rollout status, describe, events, pod logs."),
		mcp.WithArgument("deployment", mcp.RequiredArgument(), mcp.ArgumentDescription("Deployment name")),
		mcp.WithArgument("namespace", mcp.RequiredArgument(), mcp.ArgumentDescription("Namespace")),
		mcp.WithArgument("context", mcp.ArgumentDescription("kubeconfig context (optional)")),
	), diagnoseDeploymentHandler)

	s.AddPrompt(mcp.NewPrompt("investigate-node",
		mcp.WithPromptDescription("Investigate a Kubernetes node: describe, events, node stats, top pods."),
		mcp.WithArgument("node", mcp.RequiredArgument(), mcp.ArgumentDescription("Node name")),
		mcp.WithArgument("context", mcp.ArgumentDescription("kubeconfig context (optional)")),
	), investigateNodeHandler)

	s.AddPrompt(mcp.NewPrompt("safe-rollback",
		mcp.WithPromptDescription("Safely roll back a deployment: inspect history, confirm with user, then undo."),
		mcp.WithArgument("deployment", mcp.RequiredArgument(), mcp.ArgumentDescription("Deployment name")),
		mcp.WithArgument("namespace", mcp.RequiredArgument(), mcp.ArgumentDescription("Namespace")),
		mcp.WithArgument("context", mcp.ArgumentDescription("kubeconfig context (optional)")),
	), safeRollbackHandler)
}
