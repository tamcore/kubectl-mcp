package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerRolloutUndo(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("rollout_undo",
		mcp.WithDescription("Undo a Deployment rollout by restoring the pod template from a previous revision. Requires --allow-write."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("kind",
			mcp.Required(),
			mcp.Description("Resource kind (currently only Deployment is supported)"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Resource name"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace of the resource"),
		),
		mcp.WithNumber("toRevision",
			mcp.Description("Revision number to roll back to. If omitted, rolls back to the previous revision."),
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

		kind, _ := req.RequireString("kind")
		name, _ := req.RequireString("name")
		namespace, _ := req.RequireString("namespace")

		if strings.ToLower(kind) != "deployment" {
			return mcp.NewToolResultError(fmt.Sprintf("kind %q does not support rollout undo (currently only Deployment is supported)", kind)), nil
		}

		toRevision := int64(req.GetFloat("toRevision", 0))

		// Find owned ReplicaSets.
		ownedRS, err := listOwnedReplicaSets(ctx, cc, namespace, name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to list ReplicaSets: %v", err)), nil
		}

		targetRS, targetRev, err := findTargetRS(ownedRS, toRevision)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Extract the pod template from the target ReplicaSet.
		template, err := extractPodTemplate(targetRS)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to extract template from revision %d: %v", targetRev, err)), nil
		}

		// Patch the Deployment's spec.template with the target template.
		patch := map[string]interface{}{
			"spec": map[string]interface{}{
				"template": template,
			},
		}
		patchData, err := json.Marshal(patch)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to build patch: %v", err)), nil
		}

		deployGVR, err := resolveGVR(cc, kind, "apps/v1")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		_, err = cc.Dynamic.Resource(deployGVR).Namespace(namespace).Patch(
			ctx, name, types.MergePatchType, patchData, metav1.PatchOptions{},
		)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to patch %s/%s: %v", kind, name, err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Rolled back %s/%s to revision %d (context: %s)", kind, name, targetRev, ctxName)), nil
	})
}

// findTargetRS selects the ReplicaSet to roll back to. If toRevision > 0, it
// finds that specific revision. Otherwise it picks the second-highest revision
// (i.e. the one before current).
func findTargetRS(rsList []unstructured.Unstructured, toRevision int64) (unstructured.Unstructured, int64, error) {
	type revRS struct {
		rev int64
		rs  unstructured.Unstructured
	}

	var revisions []revRS
	for _, rs := range rsList {
		rev := extractRevision(rs)
		if rev <= 0 {
			continue
		}
		revisions = append(revisions, revRS{rev: rev, rs: rs})
	}

	if len(revisions) == 0 {
		return unstructured.Unstructured{}, 0, fmt.Errorf("no revisions found for rollback")
	}

	// Sort by revision descending.
	sort.Slice(revisions, func(i, j int) bool {
		return revisions[i].rev > revisions[j].rev
	})

	// Specific revision.
	if toRevision > 0 {
		for _, r := range revisions {
			if r.rev == toRevision {
				return r.rs, r.rev, nil
			}
		}
		return unstructured.Unstructured{}, 0, fmt.Errorf("revision %d not found", toRevision)
	}

	// Default: previous revision (second-highest).
	if len(revisions) < 2 {
		return unstructured.Unstructured{}, 0, fmt.Errorf("only one revision exists; nothing to roll back to")
	}
	return revisions[1].rs, revisions[1].rev, nil
}

// extractPodTemplate pulls spec.template from a ReplicaSet.
func extractPodTemplate(rs unstructured.Unstructured) (map[string]interface{}, error) {
	spec, ok := rs.Object["spec"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing spec")
	}
	template, ok := spec["template"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing spec.template")
	}
	return template, nil
}
