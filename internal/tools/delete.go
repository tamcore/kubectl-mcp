package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerDeleteResource(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) { //nolint:cyclop // elicitation adds a confirmation step
	mcpServer := s
	tool := mcp.NewTool("delete_resource",
		mcp.WithDescription("Delete a Kubernetes resource by kind, name, and namespace. Requires --allow-destructive."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("kind",
			mcp.Required(),
			mcp.Description("Resource kind (e.g. Pod, Deployment, Service)"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Resource name"),
		),
		mcp.WithString("namespace",
			mcp.Description("Namespace (required for namespaced resources)"),
		),
		mcp.WithString("apiVersion",
			mcp.Description("API version (e.g. v1, apps/v1). If omitted, the server will try to discover it."),
		),
		mcp.WithBoolean("dryRun",
			mcp.Description("If true, validate the request without persisting the change (server-side dry run)"),
		),
		mcp.WithNumber("gracePeriodSeconds",
			mcp.Description("Grace period in seconds before forceful deletion. 0 means immediate. Omit for the resource's default grace period."),
		),
		mcp.WithBoolean("force",
			mcp.Description("Force immediate deletion. Sets grace period to 0 and bypasses normal graceful termination. Use with caution — skips pre-delete hooks. Cannot be combined with gracePeriodSeconds > 0."),
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
		namespace := req.GetString("namespace", "")
		apiVersion := req.GetString("apiVersion", "")

		gvr, err := resolveGVR(cc, kind, apiVersion)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		dryRun := req.GetBool("dryRun", false)
		force := req.GetBool("force", false)

		gracePeriod := int64(req.GetFloat("gracePeriodSeconds", -1))
		if force && gracePeriod > 0 {
			return mcp.NewToolResultError("force and gracePeriodSeconds are mutually exclusive: force sets the grace period to 0, but gracePeriodSeconds > 0 was also specified"), nil
		}

		// Request elicitation confirmation for non-dry-run deletes.
		if !dryRun {
			target := fmt.Sprintf("%s/%s", kind, name)
			if namespace != "" {
				target += fmt.Sprintf(" in namespace %q", namespace)
			}
			prompt := fmt.Sprintf("Are you sure you want to delete %s?", target)
			if force {
				prompt = fmt.Sprintf("Are you sure you want to force delete %s? This bypasses graceful termination and skips pre-delete hooks.", target)
			}
			confirmed, err := confirmDestructiveAction(ctx, mcpServer, prompt)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("elicitation error: %v", err)), nil
			}
			if !confirmed {
				return mcp.NewToolResultText("Delete cancelled by user"), nil
			}
			if err := applySafetyDelay(ctx, req, cfg.SafetyDelayDestructive); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("safety delay interrupted: %v", err)), nil
			}
		}

		deleteOpts := metav1.DeleteOptions{
			DryRun: dryRunOption(dryRun),
		}
		if force {
			zero := int64(0)
			deleteOpts.GracePeriodSeconds = &zero
		} else if gracePeriod >= 0 {
			deleteOpts.GracePeriodSeconds = &gracePeriod
		}

		if namespace != "" {
			err = cc.Dynamic.Resource(gvr).Namespace(namespace).Delete(ctx, name, deleteOpts)
		} else {
			err = cc.Dynamic.Resource(gvr).Delete(ctx, name, deleteOpts)
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to delete %s/%s: %v", kind, name, err)), nil
		}

		msg := "Deleted"
		if len(deleteOpts.DryRun) > 0 {
			msg = "DRY RUN: would delete"
		}
		msg += fmt.Sprintf(" %s/%s", kind, name)
		if namespace != "" {
			msg += fmt.Sprintf(" in namespace %q", namespace)
		}
		msg += fmt.Sprintf(" (context: %s)", ctxName)

		return mcp.NewToolResultText(msg), nil
	})
}
