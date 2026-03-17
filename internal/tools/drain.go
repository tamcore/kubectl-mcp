package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerDrainNode(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("drain_node",
		mcp.WithDescription("Drain a Kubernetes node: cordon it and evict all eligible pods. Requires --allow-destructive."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("node",
			mcp.Required(),
			mcp.Description("Node name"),
		),
		mcp.WithBoolean("ignoreDaemonSets",
			mcp.Description("Ignore DaemonSet-managed pods (default: true)"),
		),
		mcp.WithBoolean("deleteEmptyDirData",
			mcp.Description("Delete pods with emptyDir volumes (default: false)"),
		),
		mcp.WithNumber("gracePeriodSeconds",
			mcp.Description("Grace period for pod eviction in seconds (default: -1 for pod's default)"),
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
		ignoreDaemonSets := req.GetBool("ignoreDaemonSets", true)
		gracePeriod := int64(req.GetFloat("gracePeriodSeconds", -1))

		// Step 1: Cordon the node.
		cordonPatch := `{"spec":{"unschedulable":true}}`
		_, err = cc.Dynamic.Resource(nodeGVR).Patch(
			ctx, node, types.MergePatchType, []byte(cordonPatch), metav1.PatchOptions{},
		)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to cordon node %q: %v", node, err)), nil
		}

		// Step 2: List pods on the node.
		pods, err := cc.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", node),
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to list pods on node %q: %v", node, err)), nil
		}

		var evicted []string
		var skipped []string
		var errors []string

		for i := range pods.Items {
			pod := &pods.Items[i]

			// Skip mirror pods (static pods).
			if _, isMirror := pod.Annotations["kubernetes.io/config.mirror"]; isMirror {
				skipped = append(skipped, fmt.Sprintf("%s/%s (mirror pod)", pod.Namespace, pod.Name))
				continue
			}

			// Skip DaemonSet pods if requested.
			if ignoreDaemonSets && isDaemonSetPod(pod) {
				skipped = append(skipped, fmt.Sprintf("%s/%s (DaemonSet)", pod.Namespace, pod.Name))
				continue
			}

			// Step 3: Evict the pod.
			eviction := &policyv1.Eviction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pod.Name,
					Namespace: pod.Namespace,
				},
			}
			if gracePeriod >= 0 {
				eviction.DeleteOptions = &metav1.DeleteOptions{
					GracePeriodSeconds: &gracePeriod,
				}
			}

			evictErr := cc.Clientset.CoreV1().Pods(pod.Namespace).EvictV1(ctx, eviction)
			if evictErr != nil {
				errors = append(errors, fmt.Sprintf("%s/%s: %v", pod.Namespace, pod.Name, evictErr))
			} else {
				evicted = append(evicted, fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
			}
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Drained node %q (context: %s)\n\n", node, ctxName)
		fmt.Fprintf(&sb, "Evicted: %d pods\n", len(evicted))
		for _, e := range evicted {
			fmt.Fprintf(&sb, "  - %s\n", e)
		}
		if len(skipped) > 0 {
			fmt.Fprintf(&sb, "Skipped: %d pods\n", len(skipped))
			for _, s := range skipped {
				fmt.Fprintf(&sb, "  - %s\n", s)
			}
		}
		if len(errors) > 0 {
			fmt.Fprintf(&sb, "Errors: %d\n", len(errors))
			for _, e := range errors {
				fmt.Fprintf(&sb, "  - %s\n", e)
			}
		}

		return mcp.NewToolResultText(sb.String()), nil
	})
}

// isDaemonSetPod checks if a pod is owned by a DaemonSet.
func isDaemonSetPod(pod *corev1.Pod) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}
