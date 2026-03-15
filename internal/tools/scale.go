package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerScaleResource(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("scale_resource",
		mcp.WithDescription("Scale a Deployment, StatefulSet, or ReplicaSet to a given number of replicas. Requires --allow-write."),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("kind",
			mcp.Required(),
			mcp.Description("Resource kind: Deployment, StatefulSet, or ReplicaSet"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Resource name"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace of the resource"),
		),
		mcp.WithNumber("replicas",
			mcp.Required(),
			mcp.Description("Desired number of replicas"),
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
		replicas := int32(req.GetFloat("replicas", 0))

		lowerKind := strings.ToLower(kind)
		if lowerKind != "deployment" && lowerKind != "statefulset" && lowerKind != "replicaset" {
			return mcp.NewToolResultError(fmt.Sprintf("kind %q is not scalable (supported: Deployment, StatefulSet, ReplicaSet)", kind)), nil
		}

		// Get current scale to report old replica count.
		var oldReplicas int32
		var scaleResult *autoscalingv1.Scale

		switch lowerKind {
		case "deployment":
			scale, getErr := cc.Clientset.AppsV1().Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
			if getErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get scale for %s/%s: %v", kind, name, getErr)), nil
			}
			oldReplicas = scale.Spec.Replicas
			scale.Spec.Replicas = replicas
			scaleResult, err = cc.Clientset.AppsV1().Deployments(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
		case "statefulset":
			scale, getErr := cc.Clientset.AppsV1().StatefulSets(namespace).GetScale(ctx, name, metav1.GetOptions{})
			if getErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get scale for %s/%s: %v", kind, name, getErr)), nil
			}
			oldReplicas = scale.Spec.Replicas
			scale.Spec.Replicas = replicas
			scaleResult, err = cc.Clientset.AppsV1().StatefulSets(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
		case "replicaset":
			scale, getErr := cc.Clientset.AppsV1().ReplicaSets(namespace).GetScale(ctx, name, metav1.GetOptions{})
			if getErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get scale for %s/%s: %v", kind, name, getErr)), nil
			}
			oldReplicas = scale.Spec.Replicas
			scale.Spec.Replicas = replicas
			scaleResult, err = cc.Clientset.AppsV1().ReplicaSets(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
		}

		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to scale %s/%s: %v", kind, name, err)), nil
		}

		result := map[string]interface{}{
			"kind":        kind,
			"name":        name,
			"namespace":   namespace,
			"context":     ctxName,
			"oldReplicas": oldReplicas,
			"newReplicas": scaleResult.Spec.Replicas,
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Scaled %s/%s from %d to %d replicas\n\n%s",
			kind, name, oldReplicas, replicas, string(out))), nil
	})
}
