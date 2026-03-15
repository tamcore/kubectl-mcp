package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/duration"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerListResources(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	tool := mcp.NewTool("list_resources",
		mcp.WithDescription("List Kubernetes resources of a given kind"),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("apiVersion",
			mcp.Description("API version (e.g. v1, apps/v1). If omitted, the server will try to discover it."),
		),
		mcp.WithString("kind",
			mcp.Required(),
			mcp.Description("Resource kind (e.g. Pod, Deployment, Service)"),
		),
		mcp.WithString("namespace",
			mcp.Description("Namespace to list in (omit for cluster-scoped or all namespaces)"),
		),
		mcp.WithString("labelSelector",
			mcp.Description("Label selector (e.g. app=nginx)"),
		),
		mcp.WithString("fieldSelector",
			mcp.Description("Field selector (e.g. metadata.name=foo)"),
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
		namespace := req.GetString("namespace", "")
		apiVersion := req.GetString("apiVersion", "")
		labelSelector := req.GetString("labelSelector", "")
		fieldSelector := req.GetString("fieldSelector", "")

		gvr, err := resolveGVR(cc, kind, apiVersion)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		opts := metav1.ListOptions{
			LabelSelector: labelSelector,
			FieldSelector: fieldSelector,
		}

		var list *unstructured.UnstructuredList
		if namespace != "" {
			list, err = cc.Dynamic.Resource(gvr).Namespace(namespace).List(ctx, opts)
		} else {
			list, err = cc.Dynamic.Resource(gvr).List(ctx, opts)
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to list %s: %v", kind, err)), nil
		}

		if !cfg.AllowSecrets {
			kube.RedactSecretsList(list)
		}

		if len(list.Items) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No %s found", kind)), nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "%-50s %-20s %s\n", "NAME", "NAMESPACE", "AGE")
		for _, item := range list.Items {
			name := item.GetName()
			ns := item.GetNamespace()
			age := "<unknown>"
			if ts := item.GetCreationTimestamp(); !ts.IsZero() {
				age = duration.HumanDuration(metav1.Now().Sub(ts.Time))
			}
			fmt.Fprintf(&sb, "%-50s %-20s %s\n", name, ns, age)
		}
		return mcp.NewToolResultText(sb.String()), nil
	})
}
