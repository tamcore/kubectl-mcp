package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/rest"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

const maxRawPathLength = 2048

// allowedMethods is the set of HTTP methods supported by api_raw.
var allowedMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
}

// RawRequester abstracts sending raw HTTP requests to the Kubernetes API.
// In production this uses rest.RESTClient; in tests a fake implementation.
type RawRequester interface {
	Do(ctx context.Context, method, path, contentType string, body []byte) ([]byte, int, error)
}

// restRawRequester is the production implementation using rest.Config.
type restRawRequester struct {
	cfg *rest.Config
}

func (r *restRawRequester) Do(ctx context.Context, method, path, contentType string, body []byte) ([]byte, int, error) {
	cfgCopy := rest.CopyConfig(r.cfg)
	cfgCopy.ContentConfig = rest.ContentConfig{}
	cfgCopy.NegotiatedSerializer = &simpleNegotiatedSerializer{}
	cfgCopy.APIPath = "/"
	cfgCopy.GroupVersion = nil

	client, err := rest.UnversionedRESTClientFor(cfgCopy)
	if err != nil {
		return nil, 0, fmt.Errorf("building REST client: %w", err)
	}

	req := client.Verb(method).AbsPath(path)
	if contentType != "" {
		req = req.SetHeader("Content-Type", contentType)
	}
	if len(body) > 0 {
		req = req.Body(body)
	}

	raw, err := req.DoRaw(ctx)
	if err != nil {
		return raw, 0, err
	}
	return raw, 200, nil
}

func registerRawAPI(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config, requester RawRequester) {
	tool := mcp.NewTool("api_raw",
		mcp.WithDescription(
			"Send a raw HTTP request to the Kubernetes API server. "+
				"Equivalent to 'kubectl get --raw'. Requires --allow-raw. "+
				"Non-GET methods additionally require --allow-write. "+
				"WARNING: bypasses secret redaction and structured formatting.",
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context)"),
		),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("API path starting with / (e.g. /healthz, /api/v1, /apis/apps/v1/deployments)"),
		),
		mcp.WithString("method",
			mcp.Description("HTTP method: GET (default), POST, PUT, PATCH, DELETE"),
		),
		mcp.WithString("body",
			mcp.Description("Request body for non-GET methods (JSON string)"),
		),
		mcp.WithString("content_type",
			mcp.Description("Content-Type header (default: application/json)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, _ := req.RequireString("path")
		method := strings.ToUpper(req.GetString("method", "GET"))
		body := req.GetString("body", "")
		contentType := req.GetString("content_type", "application/json")

		// Validate path.
		if path == "" {
			return mcp.NewToolResultError("path is required"), nil
		}
		if !strings.HasPrefix(path, "/") {
			return mcp.NewToolResultError("path must start with /"), nil
		}
		if len(path) > maxRawPathLength {
			return mcp.NewToolResultError(
				fmt.Sprintf("path exceeds maximum length of %d characters", maxRawPathLength),
			), nil
		}

		// Validate method.
		if !allowedMethods[method] {
			return mcp.NewToolResultError(
				fmt.Sprintf("invalid method %q: must be one of GET, POST, PUT, PATCH, DELETE", method),
			), nil
		}

		// Non-GET methods require --allow-write.
		if method != "GET" && !cfg.AllowWrite {
			return mcp.NewToolResultError(
				fmt.Sprintf("%s requests require --allow-write", method),
			), nil
		}

		// Resolve context.
		ctxName, err := pool.ResolveContext(req.GetString("context", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		cc, err := pool.ClientFor(ctxName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get client: %v", err)), nil
		}

		// Use injected requester or build one from the rest config.
		r := requester
		if r == nil {
			r = &restRawRequester{cfg: cc.RestConfig}
		}

		if method != "GET" {
			if err := applySafetyDelay(ctx, req, cfg.SafetyDelayWrite); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("safety delay interrupted: %v", err)), nil
			}
		}

		var bodyBytes []byte
		if body != "" {
			bodyBytes = []byte(body)
		}

		raw, _, err := r.Do(ctx, method, path, contentType, bodyBytes)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("raw API request failed: %v", err)), nil
		}

		return mcp.NewToolResultText(string(raw)), nil
	})
}
