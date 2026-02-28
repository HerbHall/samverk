# ADR-020: Web Dashboard for Operations

**Status**: Accepted
**Date**: 2026-02-27

## Context

[ADR-011](ADR-011-chat-as-interface.md) established the chat (Claude + MCP) as the primary user interface for project progress, decisions, and direction. This works well for the async check-in model -- the user's 5-15 minute interaction with their project.

However, operational concerns do not fit naturally into a conversational interface:

- **Configuration management** -- editing autonomy tiers, API keys, model priority, forge connections
- **System monitoring** -- dispatcher health, agent container status, queue depth, resource usage
- **Cost visibility** -- burn rate graphs, per-project spend, budget alerts, historical trends
- **Log inspection** -- structured log viewing with filtering by agent, severity, and task
- **Troubleshooting** -- diagnosing failed agents, reviewing QC cycles, dependency graph visualization

These are visual, data-dense, and often require scanning rather than asking. A chat interface would either produce walls of text or require many back-and-forth queries to surface the same information a dashboard shows at a glance.

## Decision

Add a web dashboard embedded in the Go binary alongside the MCP server. The dashboard is an operational admin panel, not a replacement for the chat interface.

### Clear Boundary

The chat handles **what** (project progress, decisions, direction). The dashboard handles **how** (infrastructure, operations, diagnostics).

| Chat (Claude + MCP) | Dashboard (Web UI) |
|---------------------|-------------------|
| "How's my project?" | System health metrics |
| "Approve this merge" | Agent container status |
| "Change priority on #42" | Cost graphs and trends |
| "What got done today?" | Log viewer and search |
| Give direction, make decisions | Edit autonomy config visually |
| | Troubleshoot failed agents |
| | API key management |

### Implementation

- React + TypeScript SPA, built with Vite
- Embedded in the Go binary via `embed.FS`
- Served on the same port as the MCP server
- Three HTTP surfaces: `/mcp` (Claude), `/api/v1/` (dashboard REST API), `/` (SPA static files)
- No separate web server process, no CORS, no reverse proxy

## Consequences

**Positive:**

- Single binary deployment preserved -- dashboard ships with the server
- Operational visibility without cluttering the check-in conversation
- Familiar technology (React) consistent with SubNetree project
- REST API created for dashboard also enables future integrations and automation
- Dashboard accessible from any browser on the local network or via Tailscale

**Negative:**

- Adds frontend build tooling (Node.js, Vite, npm/pnpm) to the development workflow
- More code to maintain (React components, API handlers, REST endpoints)
- Dashboard must not become a second primary interface -- the boundary must be enforced through design, not just documentation

## Related

- [ADR-011: Chat as Primary Interface](ADR-011-chat-as-interface.md) -- establishes chat for project interaction
- [ADR-019: Self-Hosted First](ADR-019-self-hosted-first.md) -- dashboard runs on the same self-hosted server
- [Tech Stack](../tech-stack.md) -- full technology choices including frontend libraries
