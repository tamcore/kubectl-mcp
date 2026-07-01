package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

const revisionAnnotation = "deployment.kubernetes.io/revision"

var replicaSetGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}

// revisionSummary holds a short description of a single rollout revision.
type revisionSummary struct {
	Revision   int64  `json:"revision"`
	ReplicaSet string `json:"replicaSet"`
	Replicas   int64  `json:"replicas,omitempty"`
	Ready      int64  `json:"ready,omitempty"`
	Image      string `json:"image,omitempty"`
}

// revisionDetail holds full detail for a single revision lookup.
type revisionDetail struct {
	Revision   int64          `json:"revision"`
	ReplicaSet string         `json:"replicaSet"`
	Template   map[string]any `json:"template,omitempty"`
}

func registerRolloutHistory(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("rollout_history",
		mcp.WithDescription("Show the rollout history of a Deployment. Lists revisions via owned ReplicaSets."),
		mcp.WithReadOnlyHintAnnotation(true),
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
		mcp.WithNumber("revision",
			mcp.Description("Specific revision number to inspect. If omitted, all revisions are listed."),
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
			return mcp.NewToolResultError(fmt.Sprintf("kind %q does not support rollout history (currently only Deployment is supported)", kind)), nil
		}

		revision := int64(req.GetFloat("revision", 0))

		ownedRS, err := listOwnedReplicaSets(ctx, cc, namespace, name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to list ReplicaSets: %v", err)), nil
		}

		// Specific revision lookup.
		if revision > 0 {
			return lookupRevision(ownedRS, revision)
		}

		// All revisions.
		return listRevisions(name, ownedRS)
	})
}

// listOwnedReplicaSets returns ReplicaSets in the given namespace that are
// owned by the named Deployment.
func listOwnedReplicaSets(ctx context.Context, cc *kube.ContextClient, namespace, deployName string) ([]unstructured.Unstructured, error) {
	rsList, err := cc.Dynamic.Resource(replicaSetGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var owned []unstructured.Unstructured
	for _, rs := range rsList.Items {
		if isOwnedBy(rs, deployName) {
			owned = append(owned, rs)
		}
	}
	return owned, nil
}

// isOwnedBy checks whether the ReplicaSet has an ownerReference matching the
// given Deployment name.
func isOwnedBy(rs unstructured.Unstructured, ownerName string) bool {
	refs, ok := rs.Object["metadata"].(map[string]any)["ownerReferences"].([]any)
	if !ok {
		return false
	}
	for _, ref := range refs {
		m, ok := ref.(map[string]any)
		if !ok {
			continue
		}
		if toString(m, "kind") == "Deployment" && toString(m, "name") == ownerName {
			return true
		}
	}
	return false
}

// listRevisions builds a sorted list of revision summaries.
func listRevisions(deployName string, rsList []unstructured.Unstructured) (*mcp.CallToolResult, error) {
	summaries := make([]revisionSummary, 0, len(rsList))
	for _, rs := range rsList {
		rev := extractRevision(rs)
		if rev <= 0 {
			continue
		}
		summaries = append(summaries, revisionSummary{
			Revision:   rev,
			ReplicaSet: rs.GetName(),
			Replicas:   toInt64(statusMap(rs), "replicas"),
			Ready:      toInt64(statusMap(rs), "readyReplicas"),
			Image:      firstContainerImage(rs),
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Revision < summaries[j].Revision
	})

	result := map[string]any{
		"deployment": deployName,
		"revisions":  summaries,
	}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

// lookupRevision returns the detail for a specific revision number.
func lookupRevision(rsList []unstructured.Unstructured, targetRevision int64) (*mcp.CallToolResult, error) {
	for _, rs := range rsList {
		rev := extractRevision(rs)
		if rev == targetRevision {
			spec, _ := rs.Object["spec"].(map[string]any)
			template, _ := spec["template"].(map[string]any)

			detail := revisionDetail{
				Revision:   rev,
				ReplicaSet: rs.GetName(),
				Template:   template,
			}

			out, err := json.MarshalIndent(detail, "", "  ")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
			}
			return mcp.NewToolResultText(string(out)), nil
		}
	}

	return mcp.NewToolResultError(fmt.Sprintf("revision %d not found", targetRevision)), nil
}

// extractRevision reads the deployment.kubernetes.io/revision annotation.
func extractRevision(rs unstructured.Unstructured) int64 {
	annotations := rs.GetAnnotations()
	if annotations == nil {
		return 0
	}
	revStr, ok := annotations[revisionAnnotation]
	if !ok {
		return 0
	}
	rev, err := strconv.ParseInt(revStr, 10, 64)
	if err != nil {
		return 0
	}
	return rev
}

// statusMap returns the status sub-object of an unstructured resource.
func statusMap(obj unstructured.Unstructured) map[string]any {
	m, _ := obj.Object["status"].(map[string]any)
	return m
}

// firstContainerImage extracts the image from the first container in the pod template.
func firstContainerImage(rs unstructured.Unstructured) string {
	spec, _ := rs.Object["spec"].(map[string]any)
	tmpl, _ := spec["template"].(map[string]any)
	podSpec, _ := tmpl["spec"].(map[string]any)
	containers, _ := podSpec["containers"].([]any)
	if len(containers) == 0 {
		return ""
	}
	container, _ := containers[0].(map[string]any)
	return toString(container, "image")
}
