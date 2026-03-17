package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

var runPodGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

// validRestartPolicies lists the accepted restart policy values.
var validRestartPolicies = map[string]bool{
	"Never":     true,
	"OnFailure": true,
	"Always":    true,
}

func registerRunPod(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("run_pod",
		mcp.WithDescription("Create and run a Pod with the given image (like kubectl run). Requires --allow-write."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace to create the pod in"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Pod name"),
		),
		mcp.WithString("image",
			mcp.Required(),
			mcp.Description("Container image (e.g. busybox:latest, nginx:1.25)"),
		),
		mcp.WithString("command",
			mcp.Description("Command to run as a shell string (e.g. 'sh -c \"echo hello\"'). Parsed using shell splitting."),
		),
		mcp.WithString("restartPolicy",
			mcp.Description("Restart policy: Never (default), OnFailure, or Always"),
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
		name, _ := req.RequireString("name")
		image, _ := req.RequireString("image")
		commandStr := req.GetString("command", "")
		restartPolicy := req.GetString("restartPolicy", "Never")

		if !validRestartPolicies[restartPolicy] {
			return mcp.NewToolResultError(fmt.Sprintf("invalid restartPolicy %q (must be Never, OnFailure, or Always)", restartPolicy)), nil
		}

		container := map[string]interface{}{
			"name":  name,
			"image": image,
		}

		if commandStr != "" {
			parts, err := shellSplit(commandStr)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid command: %v", err)), nil
			}
			cmdSlice := make([]interface{}, len(parts))
			for i, p := range parts {
				cmdSlice[i] = p
			}
			container["command"] = cmdSlice
		}

		pod := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      name,
					"namespace": namespace,
				},
				"spec": map[string]interface{}{
					"restartPolicy": restartPolicy,
					"containers":    []interface{}{container},
				},
			},
		}

		result, err := cc.Dynamic.Resource(runPodGVR).Namespace(namespace).Create(ctx, pod, metav1.CreateOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to create pod %q: %v", name, err)), nil
		}

		// Strip managedFields for cleaner output.
		if md, ok := result.Object["metadata"].(map[string]interface{}); ok {
			delete(md, "managedFields")
		}

		out, err := json.MarshalIndent(result.Object, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Created Pod %q in namespace %q (context: %s)\n\n%s",
			name, namespace, ctxName, string(out))), nil
	})
}
