package prompts

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

func diagnosePodHandler(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	pod, ok := req.Params.Arguments["pod"]
	if !ok || pod == "" {
		return nil, fmt.Errorf("required argument 'pod' is missing")
	}
	ns, ok := req.Params.Arguments["namespace"]
	if !ok || ns == "" {
		return nil, fmt.Errorf("required argument 'namespace' is missing")
	}
	ctx := req.Params.Arguments["context"]

	ctxClause := ""
	if ctx != "" {
		ctxClause = fmt.Sprintf(`, context="%s"`, ctx)
	}

	text := fmt.Sprintf(`Diagnose pod "%s" in namespace "%s".

Follow these steps in order:

1. Call describe_resource(kind="Pod", name="%s", namespace="%s"%s) to get the full pod specification and status.
2. Call get_events(namespace="%s"%s) to check for recent warning events related to the pod.
3. Call get_logs(pod="%s", namespace="%s"%s) to inspect container logs for errors or crashes.

After each step, analyse the output for known failure patterns (CrashLoopBackOff, OOMKilled, ImagePullBackOff, Pending scheduling issues, probe failures). Summarise findings and recommend remediation steps.`,
		pod, ns,
		pod, ns, ctxClause,
		ns, ctxClause,
		pod, ns, ctxClause,
	)

	return mcp.NewGetPromptResult(
		fmt.Sprintf("Diagnose pod %s in namespace %s", pod, ns),
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text)),
		},
	), nil
}

func diagnoseDeploymentHandler(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	deployment, ok := req.Params.Arguments["deployment"]
	if !ok || deployment == "" {
		return nil, fmt.Errorf("required argument 'deployment' is missing")
	}
	ns, ok := req.Params.Arguments["namespace"]
	if !ok || ns == "" {
		return nil, fmt.Errorf("required argument 'namespace' is missing")
	}
	ctx := req.Params.Arguments["context"]

	ctxClause := ""
	if ctx != "" {
		ctxClause = fmt.Sprintf(`, context="%s"`, ctx)
	}

	text := fmt.Sprintf(`Diagnose deployment "%s" in namespace "%s".

Follow these steps in order:

1. Call rollout_status(kind="Deployment", name="%s", namespace="%s"%s) to check current rollout health.
2. Call describe_resource(kind="Deployment", name="%s", namespace="%s"%s) to inspect the deployment spec and conditions.
3. Call get_events(namespace="%s"%s) to surface recent events related to the deployment or its pods.
4. Call list_resources(kind="Pod", namespace="%s"%s) and filter for pods belonging to this deployment.
5. For each pod not in Running/Completed state, call get_logs(pod=<pod-name>, namespace="%s"%s) to check for errors.

Summarise the overall health, identify root causes, and recommend next steps.`,
		deployment, ns,
		deployment, ns, ctxClause,
		deployment, ns, ctxClause,
		ns, ctxClause,
		ns, ctxClause,
		ns, ctxClause,
	)

	return mcp.NewGetPromptResult(
		fmt.Sprintf("Diagnose deployment %s in namespace %s", deployment, ns),
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text)),
		},
	), nil
}

func investigateNodeHandler(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	node, ok := req.Params.Arguments["node"]
	if !ok || node == "" {
		return nil, fmt.Errorf("required argument 'node' is missing")
	}
	ctx := req.Params.Arguments["context"]

	ctxClause := ""
	if ctx != "" {
		ctxClause = fmt.Sprintf(`, context="%s"`, ctx)
	}

	eventsCtxClause := ""
	if ctx != "" {
		eventsCtxClause = fmt.Sprintf(`context="%s"`, ctx)
	}

	text := fmt.Sprintf(`Investigate node "%s".

Follow these steps in order:

1. Call describe_resource(kind="Node", name="%s"%s) to check node conditions, taints, and capacity.
2. Call get_events(%s) and filter for events referencing this node or its pods.
3. Call node_stats(node="%s"%s) to check current CPU and memory utilisation directly from the kubelet.
4. Call top_pods(node="%s"%s) to identify which pods are consuming the most resources on this node.

Look for NotReady conditions, disk pressure, memory pressure, PID pressure, or evicted pods. Summarise findings and recommend remediation.`,
		node,
		node, ctxClause,
		eventsCtxClause,
		node, ctxClause,
		node, ctxClause,
	)

	return mcp.NewGetPromptResult(
		fmt.Sprintf("Investigate node %s", node),
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text)),
		},
	), nil
}

func safeRollbackHandler(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	deployment, ok := req.Params.Arguments["deployment"]
	if !ok || deployment == "" {
		return nil, fmt.Errorf("required argument 'deployment' is missing")
	}
	ns, ok := req.Params.Arguments["namespace"]
	if !ok || ns == "" {
		return nil, fmt.Errorf("required argument 'namespace' is missing")
	}
	ctx := req.Params.Arguments["context"]

	ctxClause := ""
	if ctx != "" {
		ctxClause = fmt.Sprintf(`, context="%s"`, ctx)
	}

	text := fmt.Sprintf(`Safely roll back deployment "%s" in namespace "%s".

Follow these steps in order:

1. Call rollout_status(kind="Deployment", name="%s", namespace="%s"%s) to understand the current rollout state.
2. Call rollout_history(kind="Deployment", name="%s", namespace="%s"%s) to list available revisions with their change causes.
3. Present the revision history to the user. Ask which revision to roll back to and wait for explicit confirmation before proceeding.
4. Only after receiving user confirmation, call rollout_undo(kind="Deployment", name="%s", namespace="%s"%s) to execute the rollback.
5. Call rollout_status(kind="Deployment", name="%s", namespace="%s"%s) again to verify the rollback completed successfully.

Do not call rollout_undo without explicit user approval. If the user declines, stop and report the available options.`,
		deployment, ns,
		deployment, ns, ctxClause,
		deployment, ns, ctxClause,
		deployment, ns, ctxClause,
		deployment, ns, ctxClause,
	)

	return mcp.NewGetPromptResult(
		fmt.Sprintf("Safe rollback for deployment %s in namespace %s", deployment, ns),
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text)),
		},
	), nil
}
