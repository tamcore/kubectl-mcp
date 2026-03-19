package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

func registerCreateResource(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	tool := mcp.NewTool("create_resource",
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Create a Kubernetes resource; fails with a conflict error if it already exists (like kubectl create). Supports single and multi-document YAML. Requires --allow-write."),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("manifest",
			mcp.Required(),
			mcp.Description("The resource manifest as a JSON or YAML string (single or multi-document separated by ---)"),
		),
		mcp.WithBoolean("dryRun",
			mcp.Description("If true, validate the request without persisting the change (server-side dry run)"),
		),
		mcp.WithString("validate",
			mcp.Description("Server-side field validation: Strict (default), Warn, or Ignore"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		manifest, err := requireStringOrJSON(req, "manifest")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		objs, err := parseManifests(manifest)
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

		dryRun := dryRunOption(req.GetBool("dryRun", false))
		validate := req.GetString("validate", "Strict")

		var results []string
		for _, obj := range objs {
			kind := obj.GetKind()
			apiVersion := obj.GetAPIVersion()
			name := obj.GetName()
			namespace := obj.GetNamespace()

			gvr, err := resolveGVR(cc, kind, apiVersion)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			var res dynamic.ResourceInterface
			if namespace != "" {
				res = cc.Dynamic.Resource(gvr).Namespace(namespace)
			} else {
				res = cc.Dynamic.Resource(gvr)
			}

			result, err := res.Create(ctx, obj, metav1.CreateOptions{
				DryRun:          dryRun,
				FieldValidation: validate,
			})
			if err != nil {
				if errors.IsAlreadyExists(err) {
					msg := fmt.Sprintf("resource %q already exists", kind+"/"+name)
					if namespace != "" {
						msg += fmt.Sprintf(" in namespace %q", namespace)
					}
					return mcp.NewToolResultError(msg), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to create %s/%s: %v", kind, name, err)), nil
			}

			if !cfg.AllowSecrets {
				kube.RedactSecrets(result)
			}

			out, err := json.MarshalIndent(result.Object, "", "  ")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
			}

			prefix := "Created"
			if len(dryRun) > 0 {
				prefix = "DRY RUN: would create"
			}

			nsInfo := ""
			if namespace != "" {
				nsInfo = " in namespace " + namespace
			}
			results = append(results, fmt.Sprintf("%s %s/%s%s in context %q\n\n%s",
				prefix, kind, name, nsInfo, ctxName, string(out)))
		}

		return mcp.NewToolResultText(strings.Join(results, "\n\n---\n\n")), nil
	})
}

// parseManifests splits a potentially multi-document YAML/JSON string into
// individual documents and parses each one.
func parseManifests(manifest string) ([]*unstructured.Unstructured, error) {
	docs := splitYAMLDocuments(manifest)
	var objs []*unstructured.Unstructured
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		obj, err := parseManifest(doc)
		if err != nil {
			return nil, err
		}
		objs = append(objs, obj)
	}
	if len(objs) == 0 {
		return nil, fmt.Errorf("no valid documents found in manifest")
	}
	return objs, nil
}

// splitYAMLDocuments splits a YAML string on "---" document separators.
func splitYAMLDocuments(manifest string) []string {
	normalized := strings.ReplaceAll(manifest, "\r\n", "\n")
	var docs []string
	var current strings.Builder
	for _, line := range strings.Split(normalized, "\n") {
		if strings.TrimSpace(line) == "---" {
			docs = append(docs, current.String())
			current.Reset()
		} else {
			current.WriteString(line)
			current.WriteByte('\n')
		}
	}
	if current.Len() > 0 {
		docs = append(docs, current.String())
	}
	return docs
}
