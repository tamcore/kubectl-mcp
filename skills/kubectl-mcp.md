---
name: kubectl-mcp
description: Kubernetes operations via kubectl-mcp MCP server — workflows, safety patterns, and issue reporting
globs:
---

# kubectl-mcp Kubernetes Skill

You have access to a Kubernetes MCP server (`kubectl-mcp`) that provides tools for
cluster operations. This skill teaches you how to use those tools effectively.

## Safety First

- **Read-only by default.** Write tools only work if the server operator enabled `--allow-write`.
  If a write tool returns "not enabled", do not retry — inform the user.
- **Secrets are redacted.** Values in Secrets and ConfigMaps may appear as `[REDACTED]`.
  This is intentional. If the server operator enabled `--allow-secrets`, you will see raw values.
- **Destructive ops require confirmation.** `delete_resource` and `drain_node` will prompt the
  user for confirmation via elicitation. Never try to bypass this.
- **Rate limits apply.** Default: 120 reads/min, 30 writes/min. If you get a rate-limit error,
  pause briefly and retry.

## Workflows

### Orientation (always start here)
1. `list_contexts` — discover available clusters
2. `list_namespaces` — discover namespaces in the target context
3. `list_api_resources` — discover available resource kinds (especially useful for CRDs)

### Diagnosing a Failing Pod
1. `get_events` with `fieldSelector: involvedObject.name=<pod>` — check for scheduling/pull errors
2. `get_logs` with the pod name (and `previous: true` if the container is crash-looping)
3. `describe_resource` on the pod — check conditions, status, and recent events
4. `exec_pod` to run diagnostic commands inside the container (requires `--allow-write`)

### Diagnosing a Failing Deployment
1. `rollout_status` — see if the rollout is stuck or progressing
2. `list_resources` for pods with the deployment's label selector — find which pods are unhealthy
3. Follow the "Diagnosing a Failing Pod" workflow for unhealthy pods
4. `rollout_history` — check if a recent revision caused the issue
5. `rollout_undo` — revert to a known-good revision if needed (requires `--allow-write`)

### Investigating Node Issues
1. `top_nodes` — identify nodes with high CPU/memory usage
2. `node_stats` — get detailed kubelet-level resource usage
3. `node_logs` — check kubelet, containerd, or kernel logs
4. `describe_resource` on the node — check conditions (DiskPressure, MemoryPressure, etc.)
5. `get_events` filtered to the node — check for recent issues

### Applying Changes Safely
1. **Always dry-run first:** use `apply_resource` or `patch_resource` with `dryRun: true`
2. Review the dry-run output with the user
3. Apply for real only after user confirmation
4. Verify with `get_resource` or `describe_resource` after applying

### Managing Rollouts
1. `rollout_status` — check current state
2. `rollout_pause` — pause a problematic rollout
3. Fix the issue (patch image, edit config, etc.)
4. `rollout_resume` — continue the rollout
5. `rollout_status` — verify it completes

### Port Forwarding for Debugging
1. `port_forward` — forward a local port to a pod (auto-timeout after 10 minutes)
2. Use the forwarded port to test the service locally
3. `stop_port_forward` — clean up when done (or let it auto-expire)

## Tips

- **Fuzzy kind matching:** You can use short names like `deploy`, `svc`, `cm`, `sts`, `ds`, `rs`,
  `pvc`, `pv`, `ing`, `cj`, `hpa`, `sa`. The server resolves them automatically and suggests
  corrections for typos.
- **Label selectors:** Most list/get tools support `labelSelector` (e.g., `app=nginx,tier=frontend`).
  Use them to narrow results instead of fetching everything.
- **Field selectors:** `list_resources` and `get_events` support `fieldSelector` for server-side
  filtering (e.g., `status.phase=Running`, `involvedObject.name=my-pod`).
- **Pagination:** `list_resources` supports `limit` and `continueToken` for large result sets.
- **explain_resource:** Use this to discover the schema and available fields for any resource kind,
  including CRDs. Supports a `fieldPath` parameter for drilling into nested fields.
- **Multi-cluster:** Always confirm the context before write operations. The user may have multiple
  clusters configured.

## Common Mistakes to Avoid

- **Deleting controller-managed pods.** They will be recreated immediately. Scale the controller
  to 0 or delete/modify the controller instead.
- **Assuming namespaces exist.** Verify with `list_namespaces` before targeting a namespace.
- **Ignoring dry-run.** Always dry-run mutations before applying for real.
- **Retrying destructive operations.** If `delete_resource` or `drain_node` fails, investigate
  the error. Do not retry without user confirmation.
- **Fetching all resources.** Use selectors and limits. Unbounded list calls on large clusters
  are slow and may hit rate limits.

## Auto-Issue Reporting

When you encounter what appears to be a **bug in kubectl-mcp itself** (not a Kubernetes error),
you should offer to file a GitHub issue. This helps improve the tool for everyone.

### When to Offer Issue Filing

File an issue when:
- A tool returns an unexpected error that is clearly a server bug (e.g., panic, nil pointer,
  JSON marshal failure)
- A tool's behavior contradicts its description
- The server returns malformed or truncated responses
- A tool silently returns no results when results are expected

Do NOT file an issue when:
- The Kubernetes API returns an error (403, 404, 409, etc.) — these are cluster issues
- The server returns "not enabled" for write/destructive tools — this is configuration
- Rate limits are hit — this is expected behavior
- The user's kubeconfig is misconfigured

### How to File

1. **Check for duplicates first:**
   ```bash
   gh issue list --repo tamcore/kubectl-mcp --search "<brief description>"
   ```

2. **Anonymize all data** before including it in the issue:
   - Replace cluster/context names with `<context>`
   - Replace namespace names with `<namespace>`
   - Replace resource names with `<resource-name>`
   - Replace IP addresses with `<redacted-ip>`
   - Replace hostnames with `<redacted-host>`
   - Remove any environment variables or secret values
   - Replace kubeconfig paths with `<kubeconfig-path>`
   - Replace any user-identifying information

3. **Show the draft to the user** and ask for explicit confirmation before filing.
   If you are uncertain whether the output is fully anonymized, stop and ask the user
   to review the draft before proceeding.

4. **File the issue:**
   ```bash
   gh issue create --repo tamcore/kubectl-mcp \
     --title "bug: <tool_name> — <brief description>" \
     --body "$(cat <<'EOF'
   ## Bug Report

   **Tool:** `<tool_name>`
   **Transport:** stdio | sse | streamable-http

   ### What happened
   <description of unexpected behavior>

   ### Expected behavior
   <what the tool description says should happen>

   ### Anonymized input
   ```json
   <tool arguments with all identifying info replaced>
   ```

   ### Anonymized output
   ```
   <error or response with all identifying info replaced>
   ```

   ### Additional context
   <any relevant details about the sequence of operations>

   ---
   *This issue was auto-filed by an LLM using kubectl-mcp. All cluster-specific
   information has been anonymized.*
   EOF
   )"
   ```

5. **Share the issue URL** with the user after filing.
