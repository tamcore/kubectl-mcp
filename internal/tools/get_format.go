package tools

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// formatGetResult formats a single resource according to the requested format.
func formatGetResult(obj *unstructured.Unstructured, format string) (*mcp.CallToolResult, error) {
	switch format {
	case "full", "json":
		return formatGetFull(obj)
	case "summary":
		return formatGetSummary(obj)
	case "yaml":
		return formatGetYAML(obj)
	default:
		return mcp.NewToolResultError(fmt.Sprintf("invalid format %q: must be full, json, summary, or yaml", format)), nil
	}
}

// formatGetFull returns the full JSON output with noisy metadata stripped.
func formatGetFull(obj *unstructured.Unstructured) (*mcp.CallToolResult, error) {
	cleaned := kube.StripNoisyMetadata(obj.Object)

	out, err := json.MarshalIndent(cleaned, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal resource: %v", err)), nil
	}
	return mcp.NewToolResultStructured(cleaned, string(out)), nil
}

// formatGetSummary returns a compact summary using baseFields + kind-specific enrichment.
func formatGetSummary(obj *unstructured.Unstructured) (*mcp.CallToolResult, error) {
	s := baseFields(*obj)
	if !enrichByKind(s, *obj) {
		enrichGeneric(s, obj)
	}

	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal summary: %v", err)), nil
	}
	return mcp.NewToolResultStructured(s, string(out)), nil
}

// enrichGeneric adds basic fields for unknown resource kinds.
func enrichGeneric(s map[string]any, obj *unstructured.Unstructured) {
	s["kind"] = obj.GetKind()
	s["apiVersion"] = obj.GetAPIVersion()
	if labels := obj.GetLabels(); len(labels) > 0 {
		s["labels"] = labels
	}
}

// formatGetYAML returns the resource as YAML with noisy metadata stripped.
func formatGetYAML(obj *unstructured.Unstructured) (*mcp.CallToolResult, error) {
	cleaned := kube.StripNoisyMetadata(obj.Object)

	out, err := yaml.Marshal(cleaned)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal resource as YAML: %v", err)), nil
	}
	return mcp.NewToolResultStructured(cleaned, string(out)), nil
}
