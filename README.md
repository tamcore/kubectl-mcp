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
- **Write operations** — opt-in via `--allow-write` for apply, patch, scale, restart, cordon, uncordon
- **Destructive operations** — opt-in via `--allow-destructive` for delete and drain
- **16 MCP tools** — 8 read-only + 6 write + 2 destructive

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
| `--allow-write` | `KUBECTL_MCP_ALLOW_WRITE` | `false` | Enable write operations (apply, patch, scale, restart, cordon, uncordon) |
| `--allow-destructive` | `KUBECTL_MCP_ALLOW_DESTRUCTIVE` | `false` | Enable destructive operations (delete, drain); implies `--allow-write` |
| `--allow-secrets` | `KUBECTL_MCP_ALLOW_SECRETS` | `false` | Allow reading Secret data |

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
| `list_resources` | List resources with label/field selectors |
| `describe_resource` | Rich describe output with conditions, spec, and events |
| `get_logs` | Get pod/container logs |
| `get_events` | Get cluster events |

### Write tools (require `--allow-write`)

| Tool | Description |
|------|-------------|
| `apply_resource` | Apply a JSON/YAML manifest (create or update) |
| `patch_resource` | Patch a resource (json, merge, or strategic merge patch) |
| `scale_resource` | Scale a Deployment, StatefulSet, or ReplicaSet |
| `restart_rollout` | Restart a Deployment, StatefulSet, or DaemonSet rollout |
| `cordon_node` | Mark a node as unschedulable |
| `uncordon_node` | Mark a node as schedulable |

### Destructive tools (require `--allow-destructive`)

| Tool | Description |
|------|-------------|
| `delete_resource` | Delete a resource by kind, name, and namespace |
| `drain_node` | Cordon a node and evict all eligible pods |

## MCP Client Configuration

### Claude Desktop

```json
{
  "mcpServers": {
    "kubectl": {
      "command": "kubectl-mcp",
      "args": ["serve"]
    }
  }
}
```

### Claude Desktop (with options)

```json
{
  "mcpServers": {
    "kubectl": {
      "command": "kubectl-mcp",
      "args": ["serve", "--allow-secrets", "--denied-contexts", "/^prod-/"]
    }
  }
}
```

### Claude Desktop (SSE)

```json
{
  "mcpServers": {
    "kubectl": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

### Claude Desktop (streamable-HTTP)

```json
{
  "mcpServers": {
    "kubectl": {
      "url": "http://localhost:8080/mcp"
    }
  }
}
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

Start the server in the background, then add to `~/.copilot/mcp-config.json`:

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

Start the server in the background, then add to `~/.copilot/mcp-config.json`:

```bash
kubectl-mcp serve --transport streamable-http &
```

```json
{
  "mcpServers": {
    "kubectl": {
      "type": "sse",
      "url": "http://localhost:8080/mcp",
      "headers": {},
      "tools": ["*"]
    }
  }
}
```

## License

MIT
