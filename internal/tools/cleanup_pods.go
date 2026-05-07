package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

var podGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

func registerCleanupPods(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	tool := mcp.NewTool("cleanup_pods",
		mcp.WithDescription("Delete pods in error states (Evicted, Failed, Succeeded) from a namespace. Requires --allow-destructive."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace to clean up pods from"),
		),
		mcp.WithBoolean("dryRun",
			mcp.Description("If true, only list pods that would be deleted without actually deleting them"),
		),
		mcp.WithString("states",
			mcp.Description("Comma-separated list of pod states to clean up (default: 'Evicted,Failed,Succeeded')"),
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
		dryRun := req.GetBool("dryRun", false)
		statesStr := req.GetString("states", "Evicted,Failed,Succeeded")

		targetStates := parseStates(statesStr)
		if len(targetStates) == 0 {
			return mcp.NewToolResultError("no valid states provided"), nil
		}

		// List all pods in the namespace.
		podList, err := cc.Dynamic.Resource(podGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to list pods in namespace %q: %v", namespace, err)), nil
		}

		// Filter pods by state.
		var matched []podCleanupEntry
		for _, pod := range podList.Items {
			state := detectPodState(pod)
			if targetStates[state] {
				matched = append(matched, podCleanupEntry{
					Name:  pod.GetName(),
					State: state,
				})
			}
		}

		if len(matched) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No pods in states [%s] found in namespace %q", statesStr, namespace)), nil
		}

		if dryRun {
			return formatDryRunCleanup(matched, namespace, ctxName), nil
		}

		if err := applySafetyDelay(ctx, req, cfg.SafetyDelayDestructive); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("safety delay interrupted: %v", err)), nil
		}

		return executeCleanup(ctx, cc, matched, namespace, ctxName, req)
	})
}

// podCleanupEntry represents a pod targeted for cleanup.
type podCleanupEntry struct {
	Name  string
	State string
}

// parseStates parses a comma-separated list of state names into a lookup set.
func parseStates(s string) map[string]bool {
	states := make(map[string]bool)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			states[part] = true
		}
	}
	return states
}

// detectPodState returns the cleanup-relevant state of a pod.
func detectPodState(pod unstructured.Unstructured) string {
	phase := getStrField(pod.Object, "status", "phase")
	reason := getStrField(pod.Object, "status", "reason")

	// Evicted: phase=Failed + reason=Evicted
	if phase == "Failed" && reason == "Evicted" {
		return "Evicted"
	}

	// Failed: phase=Failed (non-evicted)
	if phase == "Failed" {
		return "Failed"
	}

	// Succeeded: phase=Succeeded
	if phase == "Succeeded" {
		return "Succeeded"
	}

	return phase
}

// formatDryRunCleanup formats the dry-run output.
func formatDryRunCleanup(pods []podCleanupEntry, namespace, ctxName string) *mcp.CallToolResult {
	var sb strings.Builder
	fmt.Fprintf(&sb, "DRY RUN: would delete %d pods in namespace %q (context: %s)\n\n", len(pods), namespace, ctxName)
	for _, p := range pods {
		fmt.Fprintf(&sb, "  - %s (%s)\n", p.Name, p.State)
	}
	return mcp.NewToolResultText(sb.String())
}

// executeCleanup deletes the matched pods and reports the results.
func executeCleanup(ctx context.Context, cc *kube.ContextClient, pods []podCleanupEntry, namespace, ctxName string, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var deleted []string
	var errors []string

	for i, p := range pods {
		err := cc.Dynamic.Resource(podGVR).Namespace(namespace).Delete(ctx, p.Name, metav1.DeleteOptions{})
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", p.Name, err))
		} else {
			deleted = append(deleted, fmt.Sprintf("%s (%s)", p.Name, p.State))
		}
		sendProgress(ctx, req, i+1, len(pods), fmt.Sprintf("deleted %s (%s)", p.Name, p.State))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Cleaned up pods in namespace %q (context: %s)\n\n", namespace, ctxName)
	fmt.Fprintf(&sb, "Deleted: %d pods\n", len(deleted))
	for _, d := range deleted {
		fmt.Fprintf(&sb, "  - %s\n", d)
	}
	if len(errors) > 0 {
		fmt.Fprintf(&sb, "Errors: %d\n", len(errors))
		for _, e := range errors {
			fmt.Fprintf(&sb, "  - %s\n", e)
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}
