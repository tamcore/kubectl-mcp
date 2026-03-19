# kubectl-mcp

> Built with ❤️ by AI, for AI.

A minimalistic Kubernetes MCP (Model Context Protocol) server that
lets LLMs query and manage your clusters safely.

## Features

- **Read-only by default** — no accidental mutations
- **Secrets redacted by default** — `.data` and `.stringData` replaced with `<redacted>`
- **Multi-kubeconfig** — honours `KUBECONFIG` with colon-separated paths
- **Context filtering** — allow/deny contexts via glob (`prod-*`) or regex (`/^staging-.+$/`)
- **Three transports** — stdio (default), SSE, and streamable-HTTP
- **Write operations** — opt-in via `--allow-write` for apply, patch, scale, restart, cordon, uncordon, exec, rollout undo, run, port-forward
- **Destructive operations** — opt-in via `--allow-destructive` for delete and drain
- **Fuzzy kind matching** — resolves short names (`deploy`, `svc`) and suggests corrections for typos
- **Rate limiting** — configurable per-minute limits for read and write operations
- **MCP tool annotations** — every tool declares `readOnlyHint`, `destructiveHint`, `idempotentHint`, and `openWorldHint` so MCP clients can make informed decisions
- **Elicitation confirmation** — destructive operations (delete, drain) prompt for user confirmation via MCP elicitation
- **Structured content** — get, list, and describe responses include machine-readable structured content alongside text
- **31 MCP tools** — 16 read-only + 12 write + 3 destructive
- **MCP resources** — read any Kubernetes resource via `k8s://` URI scheme (2 resource templates)

## Installation

```bash
go install github.com/tamcore/kubectl-mcp/cmd/kubectl-mcp@latest
```

## Usage

```bash
# Start with stdio transport (default)
kubectl-mcp serve

# Start with SSE transport
kubectl-mcp serve --transport sse --sse-address :9090

# Start with streamable-HTTP transport
kubectl-mcp serve --transport streamable-http --http-address :9090

# Use a specific kubeconfig and context
kubectl-mcp serve --kubeconfig ~/.kube/config --context my-cluster

# Allow secrets and restrict to specific contexts
kubectl-mcp serve --allow-secrets --allowed-contexts "dev-*,staging-*"

# Deny production contexts
kubectl-mcp serve --denied-contexts "/^prod-/"

# Enable write operations
kubectl-mcp serve --allow-write

# Enable all operations including delete and drain
kubectl-mcp serve --allow-write --allow-destructive
```

## Configuration

All flags can also be set via environment variables with a `KUBECTL_MCP_` prefix.
`KUBECONFIG` is honoured directly.

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--transport` | `KUBECTL_MCP_TRANSPORT` | `stdio` | Transport: `stdio`, `sse`, or `streamable-http` |
| `--sse-address` | `KUBECTL_MCP_SSE_ADDRESS` | `:8080` | SSE listen address |
| `--http-address` | `KUBECTL_MCP_HTTP_ADDRESS` | `:8080` | Streamable-HTTP listen address |
| `--kubeconfig` | `KUBECONFIG` | `~/.kube/config` | Colon-separated kubeconfig paths |
| `--context` | `KUBECTL_MCP_CONTEXT` | *(current-context)* | Default kube-context override |
| `--allowed-contexts` | `KUBECTL_MCP_ALLOWED_CONTEXTS` | `*` | Comma-separated glob/regex allow patterns |
| `--denied-contexts` | `KUBECTL_MCP_DENIED_CONTEXTS` | *(none)* | Comma-separated glob/regex deny patterns |
| `--allow-write` | `KUBECTL_MCP_ALLOW_WRITE` | `false` | Enable write operations (apply, patch, scale, restart, cordon, uncordon, exec, rollout undo, rollout pause/resume, run, port-forward) |
| `--allow-destructive` | `KUBECTL_MCP_ALLOW_DESTRUCTIVE` | `false` | Enable destructive operations (delete, drain, cleanup-pods); implies `--allow-write` |
| `--allow-secrets` | `KUBECTL_MCP_ALLOW_SECRETS` | `false` | Allow reading Secret data |
| `--rate-limit-read` | `KUBECTL_MCP_RATE_LIMIT_READ` | `120` | Max read tool calls per minute (0 = unlimited) |
| `--rate-limit-write` | `KUBECTL_MCP_RATE_LIMIT_WRITE` | `30` | Max write tool calls per minute (0 = unlimited) |

### Context Filtering

Contexts are allowed if they match at least one `--allowed-contexts` pattern
**and** do not match any `--denied-contexts` pattern. Deny takes precedence.

- **Glob patterns:** `prod-*`, `dev-??`, `*-staging`
- **Regex patterns:** Wrap in forward slashes: `/^prod-.+$/`

## MCP Tools

All tools accept an optional `context` parameter to target a specific
kube-context. If omitted, the configured default context is used.

### Read-only tools (always available)

| Tool | Description |
|------|-------------|
| `list_contexts` | List available (allowed) kube-contexts |
| `list_namespaces` | List namespaces |
| `list_api_resources` | List API resources (kind, apiVersion, namespaced, verbs) |
| `get_resource` | Get a single resource as JSON |
| `list_resources` | List resources with label/field selectors, pagination, and client-side filters |
| `describe_resource` | Rich describe output with conditions, spec, and events |
| `get_logs` | Get pod/container logs (supports label selectors, timestamps, sinceTime, resource references like deployment/nginx) |
| `get_events` | Get cluster events |
| `top_pods` | Get CPU/memory usage for pods (requires metrics-server) |
| `top_nodes` | Get CPU/memory usage for nodes with allocatable percentages |
| `rollout_status` | Get rollout status of a Deployment, StatefulSet, or DaemonSet |
| `rollout_history` | Show rollout revision history of a Deployment |
| `explain_resource` | Explain a resource kind (metadata, verbs, scope) via discovery API |
| `node_logs` | Get logs from a node via the kubelet proxy |
| `node_stats` | Get node-level resource usage (CPU, memory, filesystem) from the kubelet stats/summary API |
| `stop_port_forward` | Stop an active port-forward session or list all active sessions |

### Write tools (require `--allow-write`)

| Tool | Description |
|------|-------------|
| `apply_resource` | Apply a JSON/YAML manifest with optional dry-run |
| `patch_resource` | Patch a resource (json, merge, or strategic) with optional dry-run |
| `scale_resource` | Scale a Deployment, StatefulSet, or ReplicaSet |
| `restart_rollout` | Restart a Deployment, StatefulSet, or DaemonSet rollout |
| `cordon_node` | Mark a node as unschedulable |
| `uncordon_node` | Mark a node as schedulable |
| `exec_pod` | Execute a command in a pod container (with timeout) |
| `rollout_undo` | Undo a Deployment rollout to a previous revision |
| `rollout_pause` | Pause a Deployment rollout |
| `rollout_resume` | Resume a paused Deployment rollout |
| `run_pod` | Create and run a pod with a given image (like `kubectl run`) |
| `port_forward` | Forward a local port to a pod port (with auto-timeout) |

### Destructive tools (require `--allow-destructive`)

| Tool | Description |
|------|-------------|
| `delete_resource` | Delete a resource with optional dry-run and grace period (with elicitation confirmation) |
| `drain_node` | Cordon a node and evict all eligible pods (with elicitation confirmation) |
| `cleanup_pods` | Delete pods in error states (Evicted, Failed, Succeeded) from a namespace |

## MCP Resources

Resources expose Kubernetes objects via `k8s://` URIs, allowing MCP clients
to read cluster state directly.

### URI Scheme

Two URI patterns are supported:

| Pattern | Scope |
|---------|-------|
| `k8s://{context}/namespaces/{namespace}/{group}/{version}/{resource}/{name}` | Namespaced resources |
| `k8s://{context}/{group}/{version}/{resource}/{name}` | Cluster-scoped resources |

Use `core` as the group for core API resources (pods, services, configmaps, nodes, etc.).

### Examples

```
k8s://my-cluster/namespaces/default/core/v1/pods/nginx
k8s://my-cluster/namespaces/kube-system/apps/v1/deployments/coredns
k8s://my-cluster/core/v1/nodes/worker-1
k8s://my-cluster/core/v1/namespaces/kube-system
k8s://my-cluster/namespaces/default/core/v1/secrets/my-secret
```

Resources respect the `--allow-secrets` flag: Secret data is redacted by default.
Noisy metadata (managedFields, last-applied-configuration) is automatically stripped.

## MCP Client Configuration

### Claude Code (stdio)

```bash
claude mcp add --transport stdio --scope user kubectl -- kubectl-mcp serve
```

With options:

```bash
claude mcp add --transport stdio --scope user kubectl -- \
  kubectl-mcp serve --allow-write --allow-secrets --denied-contexts "/^prod-/"
```

To run directly from source (e.g. during development):

```bash
claude mcp add --transport stdio --scope user kubectl -- \
  go run -C /path/to/kubectl-mcp ./cmd/kubectl-mcp serve \
  --allow-write --allow-destructive --allow-secrets
```

> **Note:** Use absolute paths — `~` is not expanded. Use `--scope local` (default) for project-only config.

### Claude Code (SSE)

Start the server, then register it:

```bash
kubectl-mcp serve --transport sse &
claude mcp add --transport sse --scope user kubectl http://localhost:8080/sse
```

### Claude Code (streamable-HTTP)

Start the server, then register it:

```bash
kubectl-mcp serve --transport streamable-http &
claude mcp add --transport http --scope user kubectl http://localhost:8080/mcp
```

### GitHub Copilot CLI (stdio)

Add to `~/.copilot/mcp-config.json`:

```json
{
  "mcpServers": {
    "kubectl": {
      "type": "stdio",
      "command": "kubectl-mcp",
      "args": ["serve"],
      "tools": ["*"]
    }
  }
}
```

To run directly from source (e.g. during development):

```json
{
  "mcpServers": {
    "kubectl": {
      "type": "stdio",
      "command": "go",
      "args": [
        "run",
        "-C", "/path/to/kubectl-mcp",
        "./cmd/kubectl-mcp",
        "serve",
        "--allow-write",
        "--allow-destructive",
        "--allow-secrets"
      ],
      "tools": ["*"]
    }
  }
}
```

> **Note:** Use absolute paths — `~` is not expanded in JSON config.

### GitHub Copilot CLI (SSE)

Start the server, then add to `~/.copilot/mcp-config.json`:

```bash
kubectl-mcp serve --transport sse &
```

```json
{
  "mcpServers": {
    "kubectl": {
      "type": "sse",
      "url": "http://localhost:8080/sse",
      "headers": {},
      "tools": ["*"]
    }
  }
}
```

### GitHub Copilot CLI (streamable-HTTP)

Start the server, then add to `~/.copilot/mcp-config.json`:

```bash
kubectl-mcp serve --transport streamable-http &
```

```json
{
  "mcpServers": {
    "kubectl": {
      "type": "streamable-http",
      "url": "http://localhost:8080/mcp",
      "headers": {},
      "tools": ["*"]
    }
  }
}
```

## Companion Skill (Claude Code)

kubectl-mcp ships with a companion skill file that teaches Claude Code
effective Kubernetes workflows, safety patterns, and auto-issue reporting.

Install it:

```bash
cp skills/kubectl-mcp.md ~/.claude/skills/
```

The skill provides:
- **Diagnosis workflows** — step-by-step patterns for debugging pods, deployments, and nodes
- **Safety patterns** — dry-run-first, multi-cluster awareness, common mistakes to avoid
- **Auto-issue reporting** — when Claude encounters a server bug, it will offer to file an
  anonymized GitHub issue via `gh` CLI (always asks for your confirmation first)

> The server also embeds concise instructions in the MCP `initialize` response, so all
> MCP clients get baseline guidance automatically — even without the skill file.

## Known Limitations

### Follow / streaming logs

MCP tools use a request-response model — there is no server-to-client
streaming support. Therefore, `kubectl logs -f` (follow) behaviour cannot
be replicated. As a workaround, use the `since` or `sinceTime` parameter
with periodic polling to approximate tailing:

```
# First call
get_logs(namespace="default", pod="my-pod", since="1m")

# Subsequent calls
get_logs(namespace="default", pod="my-pod", sinceTime="<last-seen-timestamp>")
```

## License

MIT
