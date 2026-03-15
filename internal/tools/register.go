package tools

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// RegisterAll registers every MCP tool on the given server.
func RegisterAll(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	registerListContexts(s, pool)
	registerListNamespaces(s, pool)
	registerListAPIResources(s, pool)
	registerGetResource(s, pool, cfg)
	registerListResources(s, pool, cfg)
	registerDescribeResource(s, pool, cfg)
	registerGetLogs(s, pool)
	registerGetEvents(s, pool)
}
