package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerRestartRollout(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	tool := mcp.NewTool("restart_rollout",
		mcp.WithDescription("Restart a rollout by patching the pod template annotation (like kubectl rollout restart). Requires --allow-write."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("kind",
			mcp.Required(),
			mcp.Description("Resource kind: Deployment, StatefulSet, or DaemonSet"),
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

		lowerKind := strings.ToLower(kind)
		if lowerKind != "deployment" && lowerKind != "statefulset" && lowerKind != "daemonset" {
			return mcp.NewToolResultError(fmt.Sprintf("kind %q does not support rollout restart (supported: Deployment, StatefulSet, DaemonSet)", kind)), nil
		}

		gvr, err := resolveGVR(cc, kind, "apps/v1")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := applySafetyDelay(ctx, req, cfg.SafetyDelayWrite); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("safety delay interrupted: %v", err)), nil
		}

		restartedAt := time.Now().UTC().Format(time.RFC3339)
		patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, restartedAt)

		_, err = cc.Dynamic.Resource(gvr).Namespace(namespace).Patch(
			ctx, name, types.MergePatchType, []byte(patch), metav1.PatchOptions{},
		)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to restart %s/%s: %v", kind, name, err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Restarted %s/%s in namespace %q (context: %s)\nAnnotation kubectl.kubernetes.io/restartedAt set to %s",
			kind, name, namespace, ctxName, restartedAt)), nil
	})
}
