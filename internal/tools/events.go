package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerGetEvents(s *server.MCPServer, pool *kube.ClientPool) {
	tool := mcp.NewTool("get_events",
		mcp.WithDescription("Get Kubernetes events"),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("namespace",
			mcp.Description("Namespace to get events from (omit for all namespaces)"),
		),
		mcp.WithString("fieldSelector",
			mcp.Description("Field selector to filter events (e.g. involvedObject.kind=Pod)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of events to return (default 50)"),
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

		namespace := req.GetString("namespace", "")
		fieldSelector := req.GetString("fieldSelector", "")
		limit := int64(req.GetFloat("limit", 50))

		opts := metav1.ListOptions{
			FieldSelector: fieldSelector,
			Limit:         limit,
		}

		events, err := cc.Clientset.CoreV1().Events(namespace).List(ctx, opts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to list events: %v", err)), nil
		}

		if len(events.Items) == 0 {
			return mcp.NewToolResultText("No events found"), nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "%-25s %-8s %-20s %-40s %s\n", "AGE", "TYPE", "REASON", "OBJECT", "MESSAGE")
		for _, e := range events.Items {
			age := "<unknown>"
			if !e.LastTimestamp.IsZero() {
				age = duration.HumanDuration(metav1.Now().Sub(e.LastTimestamp.Time))
			}
			object := fmt.Sprintf("%s/%s", strings.ToLower(e.InvolvedObject.Kind), e.InvolvedObject.Name)
			fmt.Fprintf(&sb, "%-25s %-8s %-20s %-40s %s\n", age, e.Type, e.Reason, object, e.Message)
		}

		return mcp.NewToolResultText(sb.String()), nil
	})
}
