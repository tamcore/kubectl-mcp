package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerApplyResource(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	tool := mcp.NewTool("apply_resource",
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Apply a Kubernetes resource from a JSON or YAML manifest (like kubectl apply). Requires --allow-write."),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("manifest",
			mcp.Required(),
			mcp.Description("The resource manifest as a JSON or YAML string"),
		),
		mcp.WithBoolean("dryRun",
			mcp.Description("If true, validate the request without persisting the change (server-side dry run)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		manifest, err := requireStringOrJSON(req, "manifest")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Parse manifest into unstructured object.
		obj, err := parseManifest(manifest)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to parse manifest: %v", err)), nil
		}

		ctxName, err := pool.ResolveContext(req.GetString("context", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		cc, err := pool.ClientFor(ctxName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get client: %v", err)), nil
		}

		kind := obj.GetKind()
		apiVersion := obj.GetAPIVersion()
		gvr, err := resolveGVR(cc, kind, apiVersion)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		namespace := obj.GetNamespace()
		name := obj.GetName()

		var result *unstructured.Unstructured
		var res dynamic.ResourceInterface
		if namespace != "" {
			res = cc.Dynamic.Resource(gvr).Namespace(namespace)
		} else {
			res = cc.Dynamic.Resource(gvr)
		}

		dryRun := dryRunOption(req.GetBool("dryRun", false))

		// Try Create first; if the resource already exists, Update it.
		result, err = res.Create(ctx, obj, metav1.CreateOptions{DryRun: dryRun})
		if err != nil {
			if !isAlreadyExists(err) {
				return mcp.NewToolResultError(fmt.Sprintf("failed to apply %s/%s: %v", kind, name, err)), nil
			}
			// Resource exists — fetch it to get resourceVersion, then update.
			existing, getErr := res.Get(ctx, name, metav1.GetOptions{})
			if getErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get existing %s/%s: %v", kind, name, getErr)), nil
			}
			obj.SetResourceVersion(existing.GetResourceVersion())
			result, err = res.Update(ctx, obj, metav1.UpdateOptions{DryRun: dryRun})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to update %s/%s: %v", kind, name, err)), nil
			}
		}

		if !cfg.AllowSecrets {
			kube.RedactSecrets(result)
		}

		out, err := json.MarshalIndent(result.Object, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
		}

		prefix := "Applied"
		if len(dryRun) > 0 {
			prefix = "DRY RUN: would apply"
		}
		return mcp.NewToolResultText(fmt.Sprintf("%s %s/%s in context %q\n\n%s", prefix, kind, name, ctxName, string(out))), nil
	})
}

// parseManifest parses a JSON or YAML string into an unstructured object.
func parseManifest(manifest string) (*unstructured.Unstructured, error) {
	// Try JSON first, then YAML.
	jsonData := []byte(manifest)

	// If it's YAML, convert to JSON.
	if !json.Valid(jsonData) {
		var err error
		jsonData, err = yaml.YAMLToJSON(jsonData)
		if err != nil {
			return nil, fmt.Errorf("invalid YAML: %w", err)
		}
	}

	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal(jsonData, &obj.Object); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	if obj.GetKind() == "" {
		return nil, fmt.Errorf("manifest must include 'kind'")
	}
	if obj.GetAPIVersion() == "" {
		return nil, fmt.Errorf("manifest must include 'apiVersion'")
	}
	if obj.GetName() == "" {
		return nil, fmt.Errorf("manifest must include 'metadata.name'")
	}

	return obj, nil
}

func isAlreadyExists(err error) bool {
	return errors.IsAlreadyExists(err)
}
