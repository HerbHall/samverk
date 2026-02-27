# MCP Server Requirements

The MCP server is the bridge between the front-end agent (Claude on any device) and the Samverk back-end (Gitea + dispatcher + agents). It exposes project state and issue operations as MCP tools that Claude can call during check-in conversations.

## Architecture Position

```text
USER (phone/laptop/desktop)
        |
   Claude (any device)
        |
   MCP (Streamable HTTP)
        |
   SAMVERK MCP SERVER  <-- this document
   (standalone binary)
        |
   +----+----+
   |         |
 GITEA    REPO (git)
 (issues)  (files)
```

The MCP server is a standalone binary, separate from the dispatcher. Both run on the same Proxmox host but as independent processes.

## Transport

**Streamable HTTP** (MCP 2025 standard). Stateless HTTP request/response. Works through Tailscale tunnels and proxies without persistent connection issues.

- Endpoint: `https://<host>/mcp` (or via Tailscale: `https://<tailscale-hostname>/mcp`)
- Content-Type: `application/json`
- No session state between requests

## Authentication

Dual-mode authentication:

| Context | Method | Details |
|---------|--------|---------|
| Local network | API key | Static token in `Authorization: Bearer <key>` header. Stored in Claude MCP config. |
| Remote (mobile) | API key over Tailscale | Same API key mechanism, but traffic encrypted and access-controlled by Tailscale network. |

API keys are generated and managed by the MCP server. Stored in `.samverk/auth.yaml` (not committed to git). Keys are scoped per-user, rotatable, and revocable.

```yaml
# .samverk/auth.yaml (server-side, never committed)
api_keys:
  - name: herb-desktop
    key_hash: "sha256:..."
    created: 2026-02-27
    projects: ["*"]  # all projects
  - name: herb-mobile
    key_hash: "sha256:..."
    created: 2026-02-27
    projects: ["samverk", "subnetree"]
```

## Multi-Project Support

The MCP server manages multiple projects from a single instance. Each project is a registered Gitea repo with its own autonomy configuration.

```yaml
# .samverk/server.yaml
projects:
  samverk:
    forge: gitea
    url: https://gitea.local/herb/samverk
    repo_path: /data/repos/samverk  # local clone for file ops
    token_env: GITEA_TOKEN_SAMVERK
  subnetree:
    forge: gitea
    url: https://gitea.local/herb/subnetree
    repo_path: /data/repos/subnetree
    token_env: GITEA_TOKEN_SUBNETREE
```

Project context is set per-conversation via the `set_project` tool, or inferred from the user's first question.

## MCP Tools

### Project Management

| Tool | Description | Autonomy |
|------|-------------|----------|
| `list_projects` | List all registered projects with status summary | Tier 1 |
| `set_project` | Set active project context for this conversation | Tier 1 |

### Issue Operations (IssueTracker interface)

| Tool | Description | Autonomy |
|------|-------------|----------|
| `list_issues` | List issues with filters (status, labels, assignee, milestone) | Tier 1 |
| `get_issue` | Get full issue details including comments | Tier 1 |
| `create_issue` | Create a new issue with labels and assignment | Tier 1 |
| `update_issue` | Edit title, body, labels, assignee, milestone | Tier 2 |
| `add_comment` | Add a comment to an issue | Tier 1 |
| `close_issue` | Close an issue | Tier 2 |
| `reopen_issue` | Reopen a closed issue | Tier 2 |
| `set_labels` | Replace all labels on an issue | Tier 2 |
| `approve_action` | Approve a Tier 3 pending action (unblocks work) | Tier 3 |
| `reject_action` | Reject a Tier 3 pending action with reason | Tier 3 |

### Check-in Digest

| Tool | Description | Autonomy |
|------|-------------|----------|
| `get_digest` | Generate check-in digest: Tier 3 pending, Tier 2 completed, progress summary, cost | Tier 1 |
| `get_cost_summary` | Token usage and API cost since last check-in | Tier 1 |

### Repository Operations

| Tool | Description | Autonomy |
|------|-------------|----------|
| `list_files` | List files/directories in the repo tree | Tier 1 |
| `read_file` | Read file contents (with line range support) | Tier 1 |
| `get_diff` | View diff between branches or commits | Tier 1 |
| `list_branches` | List branches with last commit info | Tier 1 |
| `get_commit_log` | Recent commits on a branch | Tier 1 |
| `search_code` | Search file contents (grep-like) | Tier 1 |

All repository operations are read-only. The front-end agent does not modify files directly -- that is the back-end agents' job. File changes happen through the issue system.

## Autonomy Tier Enforcement

The MCP server enforces autonomy tiers as a first line of defense. The dispatcher also validates independently (defense in depth).

### Enforcement Logic

1. MCP server receives a tool call (e.g., `close_issue`)
2. Looks up the action's tier in the active project's `autonomy.yaml`
3. Checks branch-specific and agent-type overrides
4. **Tier 1/2**: Execute immediately, return result
5. **Tier 3**: Return a structured response indicating confirmation is required

```json
{
  "status": "confirmation_required",
  "action": "close_issue",
  "tier": 3,
  "issue_number": 42,
  "reason": "Close operations require user confirmation in this project",
  "confirm_tool": "approve_action",
  "context": {
    "issue_title": "Refactor dispatcher routing",
    "current_labels": ["status:needs-qc", "agent:code-gen"]
  }
}
```

The front-end agent (Claude) presents this to the user conversationally. On approval, Claude calls `approve_action` which executes the original operation.

### Front-End Agent Context

The front-end agent (Claude) is always treated as `agent:human-proxy` for tier evaluation. This means:

- The user speaking through Claude is effectively the user themselves
- `approve_action` / `reject_action` are the only Tier 3 tools (they ARE the confirmation)
- The MCP server trusts that Claude is faithfully relaying user intent

## Security Model

### What the Front-End Can Do

- Read anything (all repo and issue data)
- Create issues (new work, but agents still need to claim and execute)
- Modify issue metadata (labels, status, assignment)
- Approve/reject Tier 3 pending actions

### What the Front-End Cannot Do

- Modify files directly (no write access to repo)
- Execute commands on the server
- Access other projects without explicit key scope
- Bypass autonomy tiers (enforcement is server-side)

### What Requires Tier 3 Confirmation

Determined by the project's `autonomy.yaml`. Default Tier 3 actions when triggered through MCP:

- Approving a merge to main
- Deleting issues
- Changing autonomy configuration itself

## Future Considerations

### Claude Mobile MCP Support

When Claude mobile supports MCP connections natively:

- The Streamable HTTP transport works directly -- no migration needed
- Tailscale on mobile provides the network layer
- API key auth works identically
- No protocol changes required

### Webhook Push (Future Enhancement)

Current design is request/response (Claude asks, server answers). Future enhancement: server pushes notifications to Claude when Tier 3 blocks appear, rather than waiting for the next check-in.

This requires MCP server-initiated messages, which Streamable HTTP supports via streaming responses. Not needed for v0.0.1 -- the check-in model is sufficient.

### Voice Interface

The check-in conversation is text-based. Voice input (phone dictation) works naturally since Claude handles transcription. No MCP changes needed for voice support.

## Related Decisions

- [ADR-011: Chat as Interface](decisions/ADR-011-chat-as-interface.md)
- [ADR-013: Forge Abstraction](decisions/ADR-013-forge-abstraction.md)
- [ADR-015: Three-Tier Autonomy Model](decisions/ADR-015-three-tier-autonomy.md)
- [ADR-019: Self-Hosted First](decisions/ADR-019-self-hosted-first.md)
- [Architecture](architecture.md)
- [Communication Protocol](communication-protocol.md)
- [Autonomy Model](autonomy-model.md)
