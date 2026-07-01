package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// tableAcceptHeader is the Accept header for the server-side Table API.
const tableAcceptHeader = "application/json;as=Table;g=meta.k8s.io;v=v1"

// wrapListEnvelope wraps a list of items in an object envelope for structuredContent.
// The MCP protocol requires structuredContent to be a JSON object, not an array.
func wrapListEnvelope(items []map[string]any, continueToken string) map[string]any {
	envelope := map[string]any{
		"items": items,
		"count": len(items),
	}
	if continueToken != "" {
		envelope["continue"] = continueToken
	}
	return envelope
}

// formatListAsJSON strips noisy metadata from each item and returns a JSON array.
func formatListAsJSON(items []unstructured.Unstructured) (string, []map[string]any, error) {
	cleaned := make([]map[string]any, 0, len(items))
	for _, item := range items {
		cleaned = append(cleaned, StripNoisyMetadata(item.Object))
	}

	out, err := json.MarshalIndent(cleaned, "", "  ")
	if err != nil {
		return "", nil, err
	}
	return string(out), cleaned, nil
}

// fetchTableList fetches resources using the server-side Table API and returns
// the rendered table text. It uses the REST client directly with the Table
// accept header.
func fetchTableList(
	ctx context.Context,
	cc *kube.ContextClient,
	gvr schema.GroupVersionResource,
	namespace string,
	opts metav1.ListOptions,
) (string, error) {
	if cc.RestConfig == nil {
		return "", fmt.Errorf("table format requires a REST config (not available in this context)")
	}

	restClient, err := buildRESTClient(cc.RestConfig, gvr)
	if err != nil {
		return "", fmt.Errorf("failed to build REST client: %w", err)
	}

	path := buildResourcePath(gvr, namespace)

	req := restClient.Get().
		AbsPath(path).
		SetHeader("Accept", tableAcceptHeader)

	if opts.LabelSelector != "" {
		req = req.Param("labelSelector", opts.LabelSelector)
	}
	if opts.FieldSelector != "" {
		req = req.Param("fieldSelector", opts.FieldSelector)
	}
	if opts.Limit > 0 {
		req = req.Param("limit", fmt.Sprintf("%d", opts.Limit))
	}
	if opts.Continue != "" {
		req = req.Param("continue", opts.Continue)
	}

	raw, err := req.DoRaw(ctx)
	if err != nil {
		return "", fmt.Errorf("table request failed: %w", err)
	}

	var table metav1.Table
	if err := json.Unmarshal(raw, &table); err != nil {
		return "", fmt.Errorf("failed to parse table response: %w", err)
	}

	return formatTable(&table), nil
}

// buildRESTClient creates a REST client configured for the given GVR's API path.
func buildRESTClient(cfg *rest.Config, gvr schema.GroupVersionResource) (*rest.RESTClient, error) {
	cfgCopy := rest.CopyConfig(cfg)
	cfgCopy.ContentConfig = rest.ContentConfig{}
	cfgCopy.NegotiatedSerializer = &simpleNegotiatedSerializer{}

	if gvr.Group == "" {
		cfgCopy.APIPath = "/api"
	} else {
		cfgCopy.APIPath = "/apis"
	}
	cfgCopy.GroupVersion = &schema.GroupVersion{
		Group:   gvr.Group,
		Version: gvr.Version,
	}

	return rest.RESTClientFor(cfgCopy)
}

// buildResourcePath constructs the API path for listing resources.
func buildResourcePath(gvr schema.GroupVersionResource, namespace string) string {
	var base string
	if gvr.Group == "" {
		base = fmt.Sprintf("/api/%s", gvr.Version)
	} else {
		base = fmt.Sprintf("/apis/%s/%s", gvr.Group, gvr.Version)
	}

	if namespace != "" {
		return fmt.Sprintf("%s/namespaces/%s/%s", base, namespace, gvr.Resource)
	}
	return fmt.Sprintf("%s/%s", base, gvr.Resource)
}

// simpleNegotiatedSerializer is a minimal implementation needed for the REST client.
type simpleNegotiatedSerializer struct{}

func (s *simpleNegotiatedSerializer) SupportedMediaTypes() []runtime.SerializerInfo {
	return []runtime.SerializerInfo{
		{MediaType: "application/json", MediaTypeType: "application", MediaTypeSubType: "json"},
	}
}

func (s *simpleNegotiatedSerializer) EncoderForVersion(_ runtime.Encoder, _ runtime.GroupVersioner) runtime.Encoder {
	return nil
}

func (s *simpleNegotiatedSerializer) DecoderToVersion(_ runtime.Decoder, _ runtime.GroupVersioner) runtime.Decoder {
	return nil
}

// handleListTable handles the table format by fetching from the server-side Table API.
func handleListTable(
	ctx context.Context,
	cc *kube.ContextClient,
	gvr schema.GroupVersionResource,
	namespace string,
	opts metav1.ListOptions,
) (*mcp.CallToolResult, error) {
	tableText, err := fetchTableList(ctx, cc, gvr, namespace, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("table format error: %v", err)), nil
	}
	return mcp.NewToolResultText(tableText), nil
}

// handleListFormat formats the list results according to the requested format.
func handleListFormat(
	format string,
	items []unstructured.Unstructured,
	filters []filterExpr,
	list *unstructured.UnstructuredList,
	kind string,
	usedDefaultLimit bool,
) (*mcp.CallToolResult, error) {
	var header string
	if len(filters) > 0 {
		header = fmt.Sprintf("Matched %d of %d %s\n\n", len(items), len(list.Items), kind)
	}
	if ct := list.GetContinue(); ct != "" {
		if usedDefaultLimit {
			header += fmt.Sprintf("Showing first %d results. ", defaultListLimit)
		}
		header += fmt.Sprintf("Pagination: use continue=%q to fetch the next page\n\n", ct)
	}

	switch format {
	case "json":
		jsonText, structured, err := formatListAsJSON(items)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to format results: %v", err)), nil
		}
		envelope := wrapListEnvelope(structured, list.GetContinue())
		return mcp.NewToolResultStructured(envelope, header+jsonText), nil

	default: // "summary"
		jsonOut, summaries, err := formatResourceList(items)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to format results: %v", err)), nil
		}
		envelope := wrapListEnvelope(summaries, list.GetContinue())
		return mcp.NewToolResultStructured(envelope, header+jsonOut), nil
	}
}
