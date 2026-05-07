package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerRolloutPause(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	tool := mcp.NewTool("rollout_pause",
		mcp.WithDescription("Pause a Deployment rollout. Only Deployments support pause (not StatefulSet or DaemonSet). Requires --allow-write."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("kind",
			mcp.Required(),
			mcp.Description("Resource kind (must be Deployment)"),
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

	s.AddTool(tool, rolloutPauseHandler(pool, cfg, true))
}

func registerRolloutResume(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	tool := mcp.NewTool("rollout_resume",
		mcp.WithDescription("Resume a paused Deployment rollout. Only Deployments support pause/resume (not StatefulSet or DaemonSet). Requires --allow-write."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("kind",
			mcp.Required(),
			mcp.Description("Resource kind (must be Deployment)"),
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

	s.AddTool(tool, rolloutPauseHandler(pool, cfg, false))
}

func rolloutPauseHandler(pool *kube.ClientPool, cfg *config.Config, pause bool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		// Only Deployments support pause/resume.
		if !strings.EqualFold(kind, "deployment") {
			return mcp.NewToolResultError(fmt.Sprintf("kind %q does not support rollout pause/resume (only Deployment is supported)", kind)), nil
		}

		gvr, err := resolveGVR(cc, kind, "apps/v1")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Pause: set spec.paused=true; Resume: remove spec.paused (set to null).
		var patch string
		if pause {
			patch = `{"spec":{"paused":true}}`
		} else {
			patch = `{"spec":{"paused":null}}`
		}

		if err := applySafetyDelay(ctx, req, cfg.SafetyDelayWrite); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("safety delay interrupted: %v", err)), nil
		}

		_, err = cc.Dynamic.Resource(gvr).Namespace(namespace).Patch(
			ctx, name, types.MergePatchType, []byte(patch), metav1.PatchOptions{},
		)
		if err != nil {
			action := "pause"
			if !pause {
				action = "resume"
			}
			return mcp.NewToolResultError(fmt.Sprintf("failed to %s %s/%s: %v", action, kind, name, err)), nil
		}

		action := "Paused"
		if !pause {
			action = "Resumed"
		}
		return mcp.NewToolResultText(fmt.Sprintf("%s %s/%s in namespace %q (context: %s)", action, kind, name, namespace, ctxName)), nil
	}
}
