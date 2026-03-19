package tools

import (
	"bufio"
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

const (
	followMaxLines = 10_000
	followMaxBytes = 1024 * 1024 // 1 MB
)

func registerGetLogs(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("get_logs",
		mcp.WithDescription("Get logs from a Kubernetes pod, from all pods matching a label selector, or from a resource like deployment/nginx"),
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
			mcp.Description("Number of lines from the end to return per pod (default 100). Cannot be used with follow=true."),
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
		mcp.WithString("resource",
			mcp.Description("Resource reference (e.g. 'deployment/nginx', 'job/my-job'). Resolves to pod label selector. Supported: Deployment, Job, StatefulSet, ReplicaSet, DaemonSet. Mutually exclusive with pod and labelSelector."),
		),
		mcp.WithBoolean("follow",
			mcp.Description("Stream log output for up to followTimeout seconds, then return all accumulated lines. Cannot be combined with tail."),
		),
		mcp.WithNumber("followTimeout",
			mcp.Description("Maximum seconds to follow before returning (default 30, range 1-120). Only used when follow=true."),
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
		resource := req.GetString("resource", "")
		container := req.GetString("container", "")
		previous := req.GetBool("previous", false)
		follow := req.GetBool("follow", false)
		followTimeout := int(req.GetFloat("followTimeout", 30))

		// Validate follow constraints before any other processing.
		if follow {
			tailRaw := req.GetFloat("tail", 0)
			if tailRaw != 0 {
				return mcp.NewToolResultError("follow and tail are mutually exclusive — cannot use tail when follow=true"), nil
			}
			if followTimeout < 1 || followTimeout > 120 {
				return mcp.NewToolResultError(fmt.Sprintf("followTimeout must be between 1 and 120 seconds (got %d)", followTimeout)), nil
			}
		}

		tailLines := int64(req.GetFloat("tail", 100))

		// Exactly one of pod, labelSelector, or resource must be provided.
		specified := countSpecified(podName, labelSelector, resource)
		if specified == 0 {
			return mcp.NewToolResultError("one of pod, labelSelector, or resource must be provided"), nil
		}
		if specified > 1 {
			return mcp.NewToolResultError("pod, labelSelector, and resource are mutually exclusive — provide only one"), nil
		}

		// Resolve resource reference to label selector.
		if resource != "" {
			resolved, err := resolveResourceToLabelSelector(ctx, cc, namespace, resource)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to resolve resource %q: %v", resource, err)), nil
			}
			labelSelector = resolved
		}

		sinceStr := req.GetString("since", "")
		sinceTimeStr := req.GetString("sinceTime", "")
		timestamps := req.GetBool("timestamps", false)

		if sinceStr != "" && sinceTimeStr != "" {
			return mcp.NewToolResultError("since and sinceTime are mutually exclusive — provide only one"), nil
		}

		opts := &corev1.PodLogOptions{
			Previous:   previous,
			Timestamps: timestamps,
			Follow:     follow,
		}
		if !follow {
			opts.TailLines = &tailLines
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
			if follow {
				return fetchFollowLogs(ctx, cc, namespace, podName, opts, followTimeout)
			}
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
			var podRes *mcp.CallToolResult
			if follow {
				podRes, _ = fetchFollowLogs(ctx, cc, namespace, pod.GetName(), opts, followTimeout)
			} else {
				podRes, _ = fetchPodLogs(ctx, cc, namespace, pod.GetName(), opts)
			}
			text := extractText(podRes)
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

// fetchPodLogs fetches logs for a single pod (non-follow mode).
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

// fetchFollowLogs opens a follow stream for a single pod, accumulates lines
// up to followTimeout seconds (or until the stream closes), then returns.
func fetchFollowLogs(ctx context.Context, cc *kube.ContextClient, namespace, pod string, opts *corev1.PodLogOptions, timeoutSecs int) (*mcp.CallToolResult, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := cc.Clientset.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(streamCtx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[%s] failed to get logs: %v", pod, err)), nil
	}

	timeout := time.Duration(timeoutSecs) * time.Second
	lines, truncated, _ := readFollowStream(stream, timeout)

	// Cancel the stream context to release server-side resources.
	cancel()
	_ = stream.Close()

	if len(lines) == 0 {
		return mcp.NewToolResultText("(no logs)"), nil
	}

	var sb strings.Builder
	if truncated {
		sb.WriteString("[truncated: buffer limit reached — showing first 10000 lines / 1 MB]\n")
	}
	sb.WriteString(strings.Join(lines, "\n"))
	sb.WriteString("\n")

	return mcp.NewToolResultText(sb.String()), nil
}

// readFollowStream reads lines from r until the stream closes, timeout fires,
// or a buffer cap (followMaxLines lines or followMaxBytes bytes) is hit.
// It returns the accumulated lines, a truncation flag, and any non-EOF read error.
//
// Implementation note: lines are sent to an unbuffered channel by a scanning
// goroutine. The main goroutine collects from that channel until timeout or
// the channel is closed, so lines accumulated before the timeout are never lost.
func readFollowStream(r io.Reader, timeout time.Duration) (lines []string, truncated bool, err error) {
	type lineMsg struct {
		text string
		err  error
	}

	ch := make(chan lineMsg)

	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			ch <- lineMsg{text: scanner.Text()}
		}
		if scanErr := scanner.Err(); scanErr != nil {
			ch <- lineMsg{err: scanErr}
		}
	}()

	timer := time.After(timeout)
	var (
		acc        []string
		totalBytes int
		trunc      bool
		readErr    error
	)

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				// Channel closed: stream finished naturally.
				return acc, trunc, readErr
			}
			if msg.err != nil {
				readErr = msg.err
				return acc, trunc, readErr
			}
			totalBytes += len(msg.text) + 1 // +1 for newline
			acc = append(acc, msg.text)
			if len(acc) >= followMaxLines || totalBytes >= followMaxBytes {
				trunc = true
				return acc, trunc, nil
			}
		case <-timer:
			return acc, trunc, nil
		}
	}
}

// countSpecified returns the number of non-empty string arguments.
func countSpecified(args ...string) int {
	count := 0
	for _, a := range args {
		if a != "" {
			count++
		}
	}
	return count
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
