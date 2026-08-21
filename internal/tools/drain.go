package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerDrainNode(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	mcpServer := s
	tool := mcp.NewTool("drain_node",
		mcp.WithDescription("Drain a Kubernetes node: cordon it and evict all eligible pods. Requires --allow-destructive. "+
			"WARNING: force=true will delete unmanaged pods (not controlled by a ReplicaSet, Job, DaemonSet, or StatefulSet); those pods will be permanently lost."),
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
		mcp.WithBoolean("dryRun",
			mcp.Description("If true, validate the drain without actually cordoning or evicting (server-side dry run)"),
		),
		mcp.WithBoolean("force",
			mcp.Description("Continue even if there are pods not managed by a ReplicaSet, Job, DaemonSet, or StatefulSet. "+
				"WARNING: these unmanaged pods will be deleted and permanently lost (default: false)"),
		),
		mcp.WithNumber("timeout",
			mcp.Description("Maximum seconds to spend issuing eviction requests for the node's pods. 0 means no limit (default: 0). "+
				"Eviction is asynchronous: this bounds the time to REQUEST eviction of all eligible pods, not the time for them to "+
				"actually terminate. If the timeout is reached before every pod has had an eviction requested, the tool returns an "+
				"error listing the pods that were not yet processed."),
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
		dryRun := req.GetBool("dryRun", false)

		// Skip elicitation for dry-run since no real action is taken.
		if !dryRun {
			confirmed, confirmErr := confirmDestructiveAction(ctx, mcpServer,
				fmt.Sprintf("Are you sure you want to drain node %q? This will cordon the node and evict all eligible pods.", node))
			if confirmErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("elicitation error: %v", confirmErr)), nil
			}
			if !confirmed {
				return mcp.NewToolResultText("Drain cancelled by user"), nil
			}
			if err := applySafetyDelay(ctx, req, cfg.SafetyDelayDestructive); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("safety delay interrupted: %v", err)), nil
			}
		}

		ignoreDaemonSets := req.GetBool("ignoreDaemonSets", true)
		gracePeriod := int64(req.GetFloat("gracePeriodSeconds", -1))
		force := req.GetBool("force", false)
		timeoutFloat := req.GetFloat("timeout", 0)

		// Build an optional deadline for the drain operation.
		// timeout is in seconds and may be fractional (e.g. 0.5 = 500ms).
		var deadline time.Time
		if timeoutFloat > 0 {
			deadline = time.Now().Add(time.Duration(float64(time.Second) * timeoutFloat))
		}

		// Step 1: Cordon the node.
		cordonPatch := `{"spec":{"unschedulable":true}}`
		_, err = cc.Dynamic.Resource(nodeGVR).Patch(
			ctx, node, types.MergePatchType, []byte(cordonPatch), metav1.PatchOptions{
				DryRun: dryRunOption(dryRun),
			},
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
		var forceDeleted []string
		var skipped []string
		var errors []string

		for i := range pods.Items {
			// Check timeout before processing each pod. The timeout bounds the
			// time spent ISSUING eviction requests, not the time for pods to
			// terminate (eviction is asynchronous).
			if !deadline.IsZero() && time.Now().After(deadline) {
				// Collect the pods that had no eviction requested yet.
				var remaining []string
				for j := i; j < len(pods.Items); j++ {
					p := &pods.Items[j]
					if _, isMirror := p.Annotations["kubernetes.io/config.mirror"]; isMirror {
						continue
					}
					if ignoreDaemonSets && isDaemonSetPod(p) {
						continue
					}
					remaining = append(remaining, fmt.Sprintf("%s/%s", p.Namespace, p.Name))
				}
				return mcp.NewToolResultError(fmt.Sprintf(
					"drain timed out after %.3gs while issuing eviction requests; pods not yet processed: %s",
					timeoutFloat, strings.Join(remaining, ", "),
				)), nil
			}

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
				Name:      pod.Name,
				Namespace: pod.Namespace,
				DeleteOptions: &metav1.DeleteOptions{
					DryRun: dryRunOption(dryRun),
				},
			}
			if gracePeriod >= 0 {
				eviction.DeleteOptions.GracePeriodSeconds = &gracePeriod
			}

			evictErr := cc.Clientset.CoreV1().Pods(pod.Namespace).EvictV1(ctx, eviction)
			if evictErr != nil {
				if force && !dryRun {
					// Force-delete the pod directly when eviction fails.
					deleteOpts := metav1.DeleteOptions{}
					if gracePeriod >= 0 {
						gp := gracePeriod
						deleteOpts.GracePeriodSeconds = &gp
					}
					delErr := cc.Clientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, deleteOpts)
					if delErr != nil {
						errors = append(errors, fmt.Sprintf("%s/%s: force-delete failed: %v", pod.Namespace, pod.Name, delErr))
					} else {
						forceDeleted = append(forceDeleted, fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
					}
				} else {
					errors = append(errors, fmt.Sprintf("%s/%s: %v", pod.Namespace, pod.Name, evictErr))
				}
			} else {
				evicted = append(evicted, fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
			}
			sendProgress(ctx, req, i+1, len(pods.Items), fmt.Sprintf("processed %s/%s", pod.Namespace, pod.Name))
		}

		var sb strings.Builder
		if dryRun {
			fmt.Fprintf(&sb, "DRY RUN: would drain node %q (context: %s)\n\n", node, ctxName)
			fmt.Fprintf(&sb, "Would evict: %d pods\n", len(evicted))
		} else {
			fmt.Fprintf(&sb, "Drained node %q (context: %s)\n\n", node, ctxName)
			fmt.Fprintf(&sb, "Evicted: %d pods\n", len(evicted))
		}
		for _, e := range evicted {
			fmt.Fprintf(&sb, "  - %s\n", e)
		}
		if len(forceDeleted) > 0 {
			fmt.Fprintf(&sb, "Force-deleted: %d pods\n", len(forceDeleted))
			for _, e := range forceDeleted {
				fmt.Fprintf(&sb, "  - %s\n", e)
			}
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
