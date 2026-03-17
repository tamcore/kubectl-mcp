package tools

import (
	"context"
	"encoding/json"
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
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
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
			return mcp.NewToolResultText("[]"), nil
		}

		type eventSummary struct {
			Age       string `json:"age"`
			Type      string `json:"type"`
			Reason    string `json:"reason"`
			Object    string `json:"object"`
			Message   string `json:"message"`
			Namespace string `json:"namespace,omitempty"`
			Count     int32  `json:"count,omitempty"`
		}

		items := make([]eventSummary, 0, len(events.Items))
		for _, e := range events.Items {
			age := "<unknown>"
			if !e.LastTimestamp.IsZero() {
				age = duration.HumanDuration(metav1.Now().Sub(e.LastTimestamp.Time))
			}
			items = append(items, eventSummary{
				Age:       age,
				Type:      e.Type,
				Reason:    e.Reason,
				Object:    fmt.Sprintf("%s/%s", strings.ToLower(e.InvolvedObject.Kind), e.InvolvedObject.Name),
				Message:   e.Message,
				Namespace: e.Namespace,
				Count:     e.Count,
			})
		}

		out, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	})
}
