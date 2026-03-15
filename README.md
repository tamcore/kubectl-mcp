# kubectl-mcp

A minimalistic, read-only Kubernetes MCP (Model Context Protocol) server that
lets LLMs query your clusters safely.

## Features

- **Read-only by default** — no accidental mutations
- **Secrets redacted by default** — `.data` and `.stringData` replaced with `<redacted>`
- **Multi-kubeconfig** — honours `KUBECONFIG` with colon-separated paths
- **Context filtering** — allow/deny contexts via glob (`prod-*`) or regex (`/^staging-.+$/`)
- **Two transports** — stdio (default) and SSE
- **8 MCP tools** — list_contexts, list_namespaces, list_api_resources, get_resource, list_resources, describe_resource, get_logs, get_events

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

# Use a specific kubeconfig and context
kubectl-mcp serve --kubeconfig ~/.kube/config --context my-cluster

# Allow secrets and restrict to specific contexts
kubectl-mcp serve --allow-secrets --allowed-contexts "dev-*,staging-*"

# Deny production contexts
kubectl-mcp serve --denied-contexts "/^prod-/"
```

## Configuration

All flags can also be set via environment variables with a `KUBECTL_MCP_` prefix.
`KUBECONFIG` is honoured directly.

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--transport` | `KUBECTL_MCP_TRANSPORT` | `stdio` | Transport: `stdio` or `sse` |
| `--sse-address` | `KUBECTL_MCP_SSE_ADDRESS` | `:8080` | SSE listen address |
| `--kubeconfig` | `KUBECONFIG` | `~/.kube/config` | Colon-separated kubeconfig paths |
| `--context` | `KUBECTL_MCP_CONTEXT` | *(current-context)* | Default kube-context override |
| `--allowed-contexts` | `KUBECTL_MCP_ALLOWED_CONTEXTS` | `*` | Comma-separated glob/regex allow patterns |
| `--denied-contexts` | `KUBECTL_MCP_DENIED_CONTEXTS` | *(none)* | Comma-separated glob/regex deny patterns |
| `--allow-write` | `KUBECTL_MCP_ALLOW_WRITE` | `false` | Reserved for future write operations |
| `--allow-secrets` | `KUBECTL_MCP_ALLOW_SECRETS` | `false` | Allow reading Secret data |

### Context Filtering

Contexts are allowed if they match at least one `--allowed-contexts` pattern
**and** do not match any `--denied-contexts` pattern. Deny takes precedence.

- **Glob patterns:** `prod-*`, `dev-??`, `*-staging`
- **Regex patterns:** Wrap in forward slashes: `/^prod-.+$/`

## MCP Tools

All tools accept an optional `context` parameter to target a specific
kube-context. If omitted, the configured default context is used.

| Tool | Description |
|------|-------------|
| `list_contexts` | List available (allowed) kube-contexts |
| `list_namespaces` | List namespaces |
| `list_api_resources` | List API resources (kind, apiVersion, namespaced, verbs) |
| `get_resource` | Get a single resource as YAML |
| `list_resources` | List resources with label/field selectors |
| `describe_resource` | Rich describe output with conditions, spec, and events |
| `get_logs` | Get pod/container logs |
| `get_events` | Get cluster events |

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

## License

MIT
