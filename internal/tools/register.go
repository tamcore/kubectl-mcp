package tools

import (
	"strings"

	"github.com/mark3labs/mcp-go/server"

	"github.com/tamcore/kubectl-mcp/internal/config"
	"github.com/tamcore/kubectl-mcp/internal/kube"
	"github.com/tamcore/kubectl-mcp/internal/ratelimit"
)

// writeTools lists tool names gated behind --allow-write.
var writeTools = []string{
	"apply_resource", "create_resource", "patch_resource", "scale_resource",
	"restart_rollout", "cordon_node", "uncordon_node", "exec_pod",
	"rollout_undo",
	"rollout_pause", "rollout_resume",
	"run_pod",
	"port_forward",
	"api_raw",
}

// destructiveTools lists tool names gated behind --allow-destructive.
var destructiveTools = []string{
	"delete_resource", "drain_node",
	"cleanup_pods",
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
	registerTopPods(s, pool)
	registerTopNodes(s, pool)
	registerRolloutStatus(s, pool)
	registerRolloutHistory(s, pool)
	registerNodeLogs(s, pool)
	registerNodeStats(s, pool)
	registerExplainResource(s, pool)
	registerStopPortForward(s)
	registerListRBACBindings(s, pool)
	registerListRBACRoles(s, pool)
	registerListServiceAccounts(s, pool)

	// Write tools (require --allow-write).
	if cfg.AllowWrite {
		registerApplyResource(s, pool, cfg)
		registerCreateResource(s, pool, cfg)
		registerPatchResource(s, pool, cfg)
		registerScaleResource(s, pool, cfg)
		registerRestartRollout(s, pool, cfg)
		registerCordonNode(s, pool, cfg)
		registerUncordonNode(s, pool, cfg)
		registerExecPod(s, pool, nil, cfg)
		registerRolloutUndo(s, pool, cfg)
		registerRolloutPause(s, pool, cfg)
		registerRolloutResume(s, pool, cfg)
		registerRunPod(s, pool, cfg)
		registerPortForward(s, pool, nil, cfg)
	}

	// Destructive tools (require --allow-destructive).
	if cfg.AllowDestructive {
		registerDeleteResource(s, pool, cfg)
		registerDrainNode(s, pool, cfg)
		registerCleanupPods(s, pool, cfg)
	}

	// Raw API tools (require --allow-raw).
	if cfg.AllowRaw {
		registerRawAPI(s, pool, cfg, nil)
	}

	// Mark context parameter as required when --require-context is set.
	if cfg.RequireContext {
		applyRequireContext(s)
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

// applyRequireContext patches all registered tools that have a "context"
// property to mark it as required and update its description.
func applyRequireContext(s *server.MCPServer) {
	tools := s.ListTools()
	patched := make([]server.ServerTool, 0, len(tools))
	for _, st := range tools {
		if prop, ok := st.Tool.InputSchema.Properties["context"]; ok {
			st.Tool.InputSchema.Required = appendUnique(st.Tool.InputSchema.Required, "context")
			if m, ok := prop.(map[string]any); ok {
				if desc, ok := m["description"].(string); ok {
					m["description"] = strings.Replace(desc, " (defaults to current context)", "", 1)
				}
			}
		}
		patched = append(patched, *st)
	}
	s.SetTools(patched...)
}

func appendUnique(ss []string, val string) []string {
	for _, s := range ss {
		if s == val {
			return ss
		}
	}
	return append(ss, val)
}
