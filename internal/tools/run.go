package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

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
		mcp.WithString("ports",
			mcp.Description("Comma-separated container ports to expose, e.g. \"80\", \"80,443\", or \"8080/TCP,9090/UDP\". Protocol defaults to TCP. Valid protocols: TCP, UDP, SCTP."),
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
		portsStr := req.GetString("ports", "")

		if !validRestartPolicies[restartPolicy] {
			return mcp.NewToolResultError(fmt.Sprintf("invalid restartPolicy %q (must be Never, OnFailure, or Always)", restartPolicy)), nil
		}

		containerPorts, err := parsePortsString(portsStr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
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

		if len(containerPorts) > 0 {
			portsSlice := make([]interface{}, len(containerPorts))
			for i, p := range containerPorts {
				portsSlice[i] = p
			}
			container["ports"] = portsSlice
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

// validProtocols is the set of port protocols accepted by Kubernetes.
var validProtocols = map[string]bool{
	"TCP":  true,
	"UDP":  true,
	"SCTP": true,
}

// parsePortsString parses a comma-separated ports string like "80", "80,443",
// or "8080/TCP,9090/UDP" into a slice of container port maps ready for an
// unstructured Pod spec.
func parsePortsString(ports string) ([]map[string]interface{}, error) {
	ports = strings.TrimSpace(ports)
	if ports == "" {
		return nil, nil
	}

	parts := strings.Split(ports, ",")
	result := make([]map[string]interface{}, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		portStr := part
		protocol := "TCP"

		if idx := strings.Index(part, "/"); idx >= 0 {
			portStr = part[:idx]
			protocol = strings.ToUpper(part[idx+1:])
		}

		if !validProtocols[protocol] {
			return nil, fmt.Errorf("invalid protocol %q in port %q (must be TCP, UDP, or SCTP)", protocol, part)
		}

		portNum, err := strconv.ParseInt(strings.TrimSpace(portStr), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid port %q: not a valid integer", part)
		}
		if portNum < 1 || portNum > 65535 {
			return nil, fmt.Errorf("invalid port %d: must be between 1 and 65535", portNum)
		}

		result = append(result, map[string]interface{}{
			"containerPort": portNum,
			"protocol":      protocol,
		})
	}

	return result, nil
}
