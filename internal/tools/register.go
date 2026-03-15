package tools

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
)

// RegisterAll registers every MCP tool on the given server.
func RegisterAll(s *server.MCPServer, pool *kube.ClientPool, cfg *config.Config) {
	// Read-only tools (always registered).
	registerListContexts(s, pool)
	registerListNamespaces(s, pool)
	registerListAPIResources(s, pool)
	registerGetResource(s, pool, cfg)
	registerListResources(s, pool, cfg)
	registerDescribeResource(s, pool, cfg)
	registerGetLogs(s, pool)
	registerGetEvents(s, pool)

	// Write tools (require --allow-write).
	if cfg.AllowWrite {
		registerApplyResource(s, pool, cfg)
		registerPatchResource(s, pool, cfg)
		registerScaleResource(s, pool)
		registerRestartRollout(s, pool)
		registerCordonNode(s, pool)
		registerUncordonNode(s, pool)
	}

	// Destructive tools (require --allow-destructive).
	if cfg.AllowDestructive {
		registerDeleteResource(s, pool)
		registerDrainNode(s, pool)
	}
}
