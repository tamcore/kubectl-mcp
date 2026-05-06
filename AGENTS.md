# AGENTS.md

Guidelines for humans and AI agents contributing to kubectl-mcp.

## Project Overview

kubectl-mcp is an MCP (Model Context Protocol) server that exposes Kubernetes
operations as MCP tools. Built with Go, mcp-go, and client-go.

## Development Workflow

1. **Plan** before writing code for non-trivial changes.
2. **TDD** — write tests first (RED), implement (GREEN), refactor (IMPROVE).
3. **Lint/vet** before committing: `golangci-lint run` and `go vet ./...`.
4. **Semantic commits** — `feat:`, `fix:`, `refactor:`, `test:`, `ci:`, `docs:`, `chore:`.
5. **Per-commit CI rule** — after every commit, push to `master` and wait for the GitHub CI and E2E workflows to go green before continuing. Fix any failures before stacking the next commit.

## Package Layout

```
cmd/kubectl-mcp/       CLI entrypoint
internal/
  cmd/                 Cobra commands (root, serve)
  config/              Configuration / flags
  kube/                Kubernetes client pool, helpers
  mcplog/              Structured MCP logging
  ratelimit/           Token-bucket rate limiting middleware
  prompts/             MCP prompt definitions and handlers (diagnostic workflows)
  resources/           MCP resource templates and handlers (k8s:// URI)
  tools/               MCP tool definitions and handlers
e2e/                   End-to-end tests (build tag: e2e)
```

Each tool lives in its own file under `internal/tools/` (e.g. `get.go`, `list.go`,
`delete.go`). The project currently has 35 MCP tools (19 read-only, 13 write, 3 destructive),
2 MCP resource templates (namespaced + cluster-scoped via `k8s://` URI),
and 4 MCP prompts (diagnostic workflows in `internal/prompts/`).
Keep files under 400 lines; extract helpers when they grow.

## Adding a New MCP Tool

1. Create `internal/tools/<name>.go` with a `register<Name>(s, pool, cfg)` function.
   - Define the tool with `mcp.NewTool(...)` and add parameters.
   - **Always set MCP tool annotations** for every tool:
     ```go
     mcp.WithReadOnlyHintAnnotation(true),      // true for read-only tools
     mcp.WithDestructiveHintAnnotation(false),   // true only for delete/drain/exec
     mcp.WithIdempotentHintAnnotation(true),     // false for create-style operations
     mcp.WithOpenWorldHintAnnotation(true),      // true for all tools (Kubernetes is open-world)
     ```
   - Register it via `s.AddTool(tool, handlerFunc)`.
2. Wire it into `RegisterAll()` in `internal/tools/register.go`.
   - Read-only tools are always registered (currently 14).
   - Write tools require `cfg.AllowWrite` (currently 10); add the tool name to `writeTools`.
   - Destructive tools require `cfg.AllowDestructive` (currently 2); add the tool name to `destructiveTools`.
3. Write unit tests in `internal/tools/<name>_test.go` using fake clients from
   `internal/kube/testing.go`.
4. Add E2E coverage in `e2e/` (build tag `e2e`).

## Testing

Three layers, each with its own build tag and CI job:

| Layer     | Command                                                        | Build Tag  | What It Tests                        |
|-----------|----------------------------------------------------------------|------------|--------------------------------------|
| Unit      | `go test -race ./...`                                          | *(none)*   | Logic, handlers with fake clients    |
| Envtest   | `KUBEBUILDER_ASSETS=... go test -tags=envtest ./internal/kube/`| `envtest`  | Real API server (no kubelet/etcd)    |
| E2E       | `go test -tags=e2e -timeout=10m ./e2e/...`                     | `e2e`      | Full MCP server against KinD cluster |

Always run unit tests with `-race`. Aim for 80%+ coverage on new code.

For envtest, install the binary:
```
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
export KUBEBUILDER_ASSETS=$(setup-envtest use --print path)
```

For E2E, a running cluster is required (KinD in CI):
```
kind create cluster --name e2e
go test -tags=e2e -timeout=10m ./e2e/...
```

## CI Pipeline

All workflows run on push to `master` and on PRs:

| Workflow   | File                          | Jobs                              |
|------------|-------------------------------|-----------------------------------|
| CI         | `.github/workflows/ci.yaml`  | lint, build, unit tests, envtest  |
| E2E        | `.github/workflows/e2e.yaml` | E2E tests on KinD                 |
| Release    | `.github/workflows/release.yaml` | Tagged releases                |

## Quality Gates

- **golangci-lint** — runs in CI; fix all warnings before merging.
- **go vet** — included in the lint step.
- **Race detector** — enabled for all test runs (`-race`).
- **Build** — `go build ./...` must succeed.

## Code Conventions

- Immutable data: return new objects rather than mutating in place.
- Handle all errors explicitly; never silently discard them.
- Validate inputs at system boundaries (MCP request parameters, kubeconfig).
- Functions under 50 lines, files under 400 lines.
- No hardcoded values — use `config.Config` or constants.
