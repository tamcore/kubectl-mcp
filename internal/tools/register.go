package tools

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
	"github.com/tamcore/kubectl-mcp/internal/ratelimit"
)

// writeTools lists tool names gated behind --allow-write.
var writeTools = []string{
	"apply_resource", "patch_resource", "scale_resource",
	"restart_rollout", "cordon_node", "uncordon_node",
}

// destructiveTools lists tool names gated behind --allow-destructive.
var destructiveTools = []string{
	"delete_resource", "drain_node",
}

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

	// Apply rate limiting to all registered tools.
	applyRateLimits(s, cfg)
}

// applyRateLimits wraps all registered tool handlers with rate limiters.
func applyRateLimits(s *server.MCPServer, cfg *config.Config) {
	readLimiter := ratelimit.NewLimiter(cfg.RateLimitRead)
	writeLimiter := ratelimit.NewLimiter(cfg.RateLimitWrite)

	writeSet := make(map[string]bool, len(writeTools)+len(destructiveTools))
	for _, name := range writeTools {
		writeSet[name] = true
	}
	for _, name := range destructiveTools {
		writeSet[name] = true
	}

	tools := s.ListTools()
	wrapped := make([]server.ServerTool, 0, len(tools))
	for _, t := range tools {
		limiter := readLimiter
		if writeSet[t.Tool.Name] {
			limiter = writeLimiter
		}
		wrapped = append(wrapped, server.ServerTool{
			Tool:    t.Tool,
			Handler: ratelimit.Wrap(t.Handler, limiter),
		})
	}
	s.SetTools(wrapped...)
}
