package tools

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerGetLogs(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("get_logs",
		mcp.WithDescription("Get logs from a Kubernetes pod or from all pods matching a label selector"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Namespace of the pod(s)"),
		),
		mcp.WithString("pod",
			mcp.Description("Pod name. Required unless labelSelector is provided."),
		),
		mcp.WithString("labelSelector",
			mcp.Description("Label selector to match pods (e.g. 'app=nginx'). Fetches logs from all matching pods."),
		),
		mcp.WithString("container",
			mcp.Description("Container name (required if pod has multiple containers)"),
		),
		mcp.WithNumber("tail",
			mcp.Description("Number of lines from the end to return per pod (default 100)"),
		),
		mcp.WithString("since",
			mcp.Description("Only return logs newer than a relative duration (e.g. 5m, 1h)"),
		),
		mcp.WithBoolean("previous",
			mcp.Description("Return logs from previous terminated container"),
		),
		mcp.WithBoolean("timestamps",
			mcp.Description("Include RFC3339 timestamps at the beginning of each log line"),
		),
		mcp.WithString("sinceTime",
			mcp.Description("Only return logs after this RFC3339 timestamp (e.g. '2024-01-15T10:00:00Z'). Mutually exclusive with 'since'."),
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
		podName := req.GetString("pod", "")
		labelSelector := req.GetString("labelSelector", "")
		container := req.GetString("container", "")
		previous := req.GetBool("previous", false)
		tailLines := int64(req.GetFloat("tail", 100))

		if podName == "" && labelSelector == "" {
			return mcp.NewToolResultError("either pod or labelSelector must be provided"), nil
		}

		sinceStr := req.GetString("since", "")
		sinceTimeStr := req.GetString("sinceTime", "")
		timestamps := req.GetBool("timestamps", false)

		if sinceStr != "" && sinceTimeStr != "" {
			return mcp.NewToolResultError("since and sinceTime are mutually exclusive — provide only one"), nil
		}

		opts := &corev1.PodLogOptions{
			TailLines:  &tailLines,
			Previous:   previous,
			Timestamps: timestamps,
		}
		if container != "" {
			opts.Container = container
		}

		if sinceStr != "" {
			d, err := parseDuration(sinceStr)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid since duration: %v", err)), nil
			}
			secs := int64(d.Seconds())
			opts.SinceSeconds = &secs
		}
		if sinceTimeStr != "" {
			t, err := time.Parse(time.RFC3339, sinceTimeStr)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid sinceTime (must be RFC3339): %v", err)), nil
			}
			mt := metav1.NewTime(t)
			opts.SinceTime = &mt
		}

		// Single-pod path.
		if podName != "" {
			return fetchPodLogs(ctx, cc, namespace, podName, opts)
		}

		// Multi-pod path: resolve pods by label selector via dynamic client.
		sel, err := labels.Parse(labelSelector)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid labelSelector: %v", err)), nil
		}

		podGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
		podList, err := cc.Dynamic.Resource(podGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: sel.String(),
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to list pods: %v", err)), nil
		}
		if len(podList.Items) == 0 {
			return mcp.NewToolResultError(fmt.Sprintf("no pods found matching labelSelector %q in namespace %q", labelSelector, namespace)), nil
		}

		var sb strings.Builder
		for i, pod := range podList.Items {
			if i > 0 {
				sb.WriteString("\n")
			}
			res, _ := fetchPodLogs(ctx, cc, namespace, pod.GetName(), opts)
			text := extractText(res)
			// Prefix each line with pod name.
			for _, line := range strings.Split(text, "\n") {
				if line == "" {
					continue
				}
				fmt.Fprintf(&sb, "[%s] %s\n", pod.GetName(), line)
			}
		}

		if sb.Len() == 0 {
			return mcp.NewToolResultText("(no logs)"), nil
		}
		return mcp.NewToolResultText(sb.String()), nil
	})
}

// fetchPodLogs fetches logs for a single pod.
func fetchPodLogs(ctx context.Context, cc *kube.ContextClient, namespace, pod string, opts *corev1.PodLogOptions) (*mcp.CallToolResult, error) {
	stream, err := cc.Clientset.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[%s] failed to get logs: %v", pod, err)), nil
	}
	defer func() { _ = stream.Close() }()

	data, err := io.ReadAll(stream)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[%s] failed to read logs: %v", pod, err)), nil
	}

	if len(data) == 0 {
		return mcp.NewToolResultText("(no logs)"), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// extractText gets the text content from a tool result.
func extractText(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		return ""
	}
	return tc.Text
}
