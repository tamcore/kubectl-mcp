package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerPatchResource(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	tool := mcp.NewTool("patch_resource",
		mcp.WithDescription("Patch a Kubernetes resource using JSON patch, merge patch, or strategic merge patch. Requires --allow-write."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
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
		mcp.WithString("patch",
			mcp.Required(),
			mcp.Description("The patch body as a JSON string"),
		),
		mcp.WithString("patchType",
			mcp.Description("Patch type: json, merge, or strategic (default: strategic)"),
		),
		mcp.WithBoolean("dryRun",
			mcp.Description("If true, validate the request without persisting the change (server-side dry run)"),
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
		patchStr, err := requireStringOrJSON(req, "patch")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		patchTypeStr := req.GetString("patchType", "strategic")

		pt, err := parsePatchType(patchTypeStr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		gvr, err := resolveGVR(cc, kind, apiVersion)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		patchData := []byte(patchStr)
		dryRun := dryRunOption(req.GetBool("dryRun", false))

		var result *unstructured.Unstructured
		if namespace != "" {
			result, err = cc.Dynamic.Resource(gvr).Namespace(namespace).Patch(ctx, name, pt, patchData, metav1.PatchOptions{DryRun: dryRun})
		} else {
			result, err = cc.Dynamic.Resource(gvr).Patch(ctx, name, pt, patchData, metav1.PatchOptions{DryRun: dryRun})
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to patch %s/%s: %v", kind, name, err)), nil
		}

		if !cfg.AllowSecrets {
			kube.RedactSecrets(result)
		}

		out, err := json.MarshalIndent(result.Object, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
		}

		prefix := "Patched"
		if len(dryRun) > 0 {
			prefix = "DRY RUN: would patch"
		}
		return mcp.NewToolResultText(fmt.Sprintf("%s %s/%s in context %q\n\n%s", prefix, kind, name, ctxName, string(out))), nil
	})
}

func parsePatchType(s string) (types.PatchType, error) {
	switch s {
	case "json":
		return types.JSONPatchType, nil
	case "merge":
		return types.MergePatchType, nil
	case "strategic", "":
		return types.StrategicMergePatchType, nil
	default:
		return "", fmt.Errorf("invalid patchType %q: must be json, merge, or strategic", s)
	}
}
