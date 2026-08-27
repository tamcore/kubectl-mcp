package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// rolloutStatusResult holds the structured rollout status for a workload.
type rolloutStatusResult struct {
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Complete   bool              `json:"complete"`
	Replicas   map[string]int64  `json:"replicas"`
	Conditions []statusCondition `json:"conditions,omitempty"`
}

// statusCondition is a simplified representation of a workload condition.
type statusCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

func registerRolloutStatus(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("rollout_status",
		mcp.WithDescription("Get the rollout status of a Deployment, StatefulSet, or DaemonSet"),
		mcp.WithOutputSchema[rolloutStatusResult](),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("kind",
			mcp.Required(),
			mcp.Description("Resource kind: Deployment, StatefulSet, or DaemonSet"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Resource name"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace of the resource"),
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

		lowerKind := strings.ToLower(kind)
		if lowerKind != "deployment" && lowerKind != "statefulset" && lowerKind != "daemonset" {
			return mcp.NewToolResultError(fmt.Sprintf("kind %q does not support rollout status (supported: Deployment, StatefulSet, DaemonSet)", kind)), nil
		}

		gvr, err := resolveGVR(cc, kind, "apps/v1")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		obj, err := cc.Dynamic.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get %s/%s: %v", kind, name, err)), nil
		}

		result := buildRolloutStatus(kind, name, namespace, obj.Object)

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
		}

		return mcp.NewToolResultStructured(result, string(out)), nil
	})
}

// buildRolloutStatus extracts status fields from the raw object and determines
// whether the rollout is complete.
func buildRolloutStatus(kind, name, namespace string, obj map[string]any) rolloutStatusResult {
	status, _ := obj["status"].(map[string]any)
	if status == nil {
		status = map[string]any{}
	}

	result := rolloutStatusResult{
		Kind:       kind,
		Name:       name,
		Namespace:  namespace,
		Replicas:   make(map[string]int64),
		Conditions: extractConditions(status),
	}

	lowerKind := strings.ToLower(kind)
	switch lowerKind {
	case "deployment":
		result.Complete = deploymentComplete(obj, status, result.Replicas)
	case "statefulset":
		result.Complete = statefulSetComplete(obj, status, result.Replicas)
	case "daemonset":
		result.Complete = daemonSetComplete(status, result.Replicas)
	}

	return result
}

func deploymentComplete(obj, status map[string]any, replicas map[string]int64) bool {
	spec, _ := obj["spec"].(map[string]any)
	desired := toInt64(spec, "replicas")
	ready := toInt64(status, "readyReplicas")
	updated := toInt64(status, "updatedReplicas")
	available := toInt64(status, "availableReplicas")

	replicas["desired"] = desired
	replicas["ready"] = ready
	replicas["updated"] = updated
	replicas["available"] = available

	return desired > 0 && ready == desired && updated == desired && available == desired
}

func statefulSetComplete(obj, status map[string]any, replicas map[string]int64) bool {
	spec, _ := obj["spec"].(map[string]any)
	desired := toInt64(spec, "replicas")
	ready := toInt64(status, "readyReplicas")
	updated := toInt64(status, "updatedReplicas")

	replicas["desired"] = desired
	replicas["ready"] = ready
	replicas["updated"] = updated

	return desired > 0 && ready == desired && updated == desired
}

func daemonSetComplete(status map[string]any, replicas map[string]int64) bool {
	desired := toInt64(status, "desiredNumberScheduled")
	ready := toInt64(status, "numberReady")
	updated := toInt64(status, "updatedNumberScheduled")
	available := toInt64(status, "numberAvailable")

	replicas["desired"] = desired
	replicas["ready"] = ready
	replicas["updated"] = updated
	replicas["available"] = available

	return desired > 0 && ready == desired && updated == desired && available == desired
}

// extractConditions pulls condition entries from the status object.
func extractConditions(status map[string]any) []statusCondition {
	raw, ok := status["conditions"].([]any)
	if !ok {
		return nil
	}

	conditions := make([]statusCondition, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		conditions = append(conditions, statusCondition{
			Type:    toString(m, "type"),
			Status:  toString(m, "status"),
			Reason:  toString(m, "reason"),
			Message: toString(m, "message"),
		})
	}
	return conditions
}

// toInt64 safely extracts an int64 from an unstructured map.
func toInt64(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}
	val, ok := m[key]
	if !ok {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case int:
		return int64(v)
	default:
		return 0
	}
}

// toString safely extracts a string from an unstructured map.
func toString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	val, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := val.(string)
	return s
}
