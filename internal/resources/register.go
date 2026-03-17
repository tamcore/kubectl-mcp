package resources

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// RegisterAll registers MCP resource templates for k8s:// URI scheme.
// Two templates are registered:
//   - Namespaced: k8s://{context}/namespaces/{namespace}/{group}/{version}/{resource}/{name}
//   - Cluster-scoped: k8s://{context}/{group}/{version}/{resource}/{name}
func RegisterAll(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	handler := func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return readResource(ctx, req, pool, cfg)
	}

	namespacedTemplate := mcp.NewResourceTemplate(
		"k8s://{context}/namespaces/{namespace}/{group}/{version}/{resource}/{name}",
		"Namespaced Kubernetes Resource",
		mcp.WithTemplateDescription(
			"Read a namespaced Kubernetes resource by context, namespace, API group, version, resource type, and name. "+
				"Use 'core' as the group for core API resources (e.g. pods, services, configmaps).",
		),
		mcp.WithTemplateMIMEType("application/json"),
	)

	clusterTemplate := mcp.NewResourceTemplate(
		"k8s://{context}/{group}/{version}/{resource}/{name}",
		"Cluster-scoped Kubernetes Resource",
		mcp.WithTemplateDescription(
			"Read a cluster-scoped Kubernetes resource by context, API group, version, resource type, and name. "+
				"Use 'core' as the group for core API resources (e.g. nodes, namespaces).",
		),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(namespacedTemplate, handler)
	s.AddResourceTemplate(clusterTemplate, handler)
}
