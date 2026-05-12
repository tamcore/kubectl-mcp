package cmd

import "github.com/tamcore/kubectl-mcp/internal/config"

// serverInstructions returns the MCP server instructions that are sent to
// clients in the initialize response. These help LLMs understand how to use
// the kubectl-mcp tools effectively.
//
// Keep this concise — it is injected into every session's context.
func serverInstructions(cfg *config.Config) string {
	base := `kubectl-mcp — Kubernetes MCP Server

## Safety Model
- All tools are read-only by default. Write and destructive operations require explicit server flags (--allow-write, --allow-destructive).
- Secrets are redacted by default. If a tool response looks incomplete because of redacted values, that is intentional.
- Destructive operations (delete_resource, drain_node) require user confirmation via elicitation before executing.
- Rate limits are enforced (default: 120 read/min, 30 write/min). If you hit a rate limit, wait briefly and retry.

## Getting Started
- Always call list_contexts first to discover available clusters and confirm which context to target.
- Use list_api_resources to discover available resource types, especially for unfamiliar clusters or CRDs.
- Use explain_resource to understand a resource kind's fields, verbs, and scope before operating on it.

## Effective Patterns
- Diagnosis: get_events → get_logs → describe_resource → exec_pod (if write is enabled).
- Rollout management: rollout_status → rollout_history → rollout_undo (if needed).
- Before mutating resources, prefer a dry-run first (apply_resource and patch_resource support dryRun).
- Use list_resources with label/field selectors to narrow results instead of fetching everything.
- Fuzzy kind matching is supported — short names like "deploy", "svc", "cm" resolve automatically.
- node_logs and node_stats talk directly to the kubelet — use them for node-level debugging.

## Multi-Cluster Awareness
- The server may expose multiple contexts. Always confirm the target context before write operations.
- Some contexts may be filtered out by the server operator — only contexts returned by list_contexts are available.

## What Not To Do
- Do not delete pods managed by controllers (Deployments, StatefulSets, etc.) — scale to 0 or manage the controller instead.
- Do not assume a namespace exists — verify with list_namespaces first.
- Do not retry destructive operations without user confirmation, even if the first attempt fails.`

	if cfg.RequireContext {
		base += `

## Context Required
- This server requires an explicit context parameter on every tool call.
- Use list_contexts to discover available contexts, then pass the chosen context to every subsequent tool call.`
	}

	return base
}
