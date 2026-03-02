# Phase 2 Validation Results

Validation date: 2026-03-02

## Test Setup

Created 4 test issues on the Samverk repo:

- **#94** `status:needs-human` + `priority:critical` -- Tier 3 blocking item
- **#95** `status:in-progress` -- active work
- **#96** `status:queued` + `priority:normal` -- waiting
- **#97** `type:research` (closed) -- completed Tier 2 action

All test issues cleaned up after validation.

## CLI Validation

Command: `samverk digest --owner herbhall --repo samverk --since 720h`

### Results

| Check | Status | Notes |
|-------|--------|-------|
| Tier 3 section populates | PASS | Shows #94 as `[1] BLOCK` with frontmatter-parsed type |
| Tier 2 section populates | PASS | Shows 30 completed items grouped by day (Today/Yesterday) |
| Status section populates | PASS | Active: 1, Queued: 1, Blocked: 0 |
| Priority sorting | PASS | Critical items shown first |
| Frontmatter parsing | PASS | `type: block` extracted from #94 body |
| Time grouping | PASS | "Today:" and "Yesterday:" sections correct |
| Cost data | N/A | "no cost data available" -- expected (no .samverk/samverk.db) |
| Edge case: long absence | PASS | "You've been away 30d" greeting |
| Quick-action syntax shown | PASS | Approve, reject, and context options displayed |

### Cost Database Warning

Expected warning when no database exists:

```text
WARN could not open cost database, continuing without cost data
```

Graceful degradation -- digest renders without cost section.

## MCP Validation

Command: `samverk serve --addr :9091` with `GITHUB_TOKEN`, `SAMVERK_GITHUB_OWNER`, `SAMVERK_GITHUB_REPO` set.

### Endpoint Results

| Method | Status | Response |
|--------|--------|----------|
| `initialize` | PASS | `serverInfo: {name: "samverk", version: "dev"}`, protocol `2025-06-18` |
| `tools/list` | PASS | 2 tools: `get_digest`, `get_cost_summary` with JSON Schema inputs |
| `tools/call get_digest` | PASS | Full digest text with all 3 sections, real GitHub data |

### Protocol Details

- Transport: Streamable HTTP (POST `/mcp`)
- Accept header: requires both `application/json` and `text/event-stream`
- Response mode: stateless JSON (no SSE streaming)
- JSON-RPC 2.0 compliant

## Prompt Validation

The front-end agent prompt design (`docs/frontend-agent-prompt.md`) covers:

| Aspect | Status | Notes |
|--------|--------|-------|
| System prompt structure | PASS | 4-section design: role, tools, presentation, grammar |
| Quick-action parsing | PASS | Grammar covers approve, reject, details, focus, hold, undo, pause |
| Conversation flows | PASS | 4 examples: standard, direction-setting, detail request, budget alert |
| Device adaptation | PASS | Phone/tablet/desktop density rules defined |
| Error handling | PASS | 5 failure modes with agent behaviors specified |

### Deferred to Interactive Testing

Full prompt validation with a live Claude session requires:

1. Configuring Claude Desktop or Claude Code to use Samverk as an MCP server
2. Running a real check-in conversation
3. Testing quick-action parsing with live forge operations

This is deferred to first real usage. The current validation confirms
the data pipeline works end-to-end.

## Issues Found

### Issue 1: Accept Header Requirement (Informational)

The Streamable HTTP transport requires `Accept: application/json, text/event-stream`
in every request. Clients sending only `Accept: application/json` get a plain-text
error response. This is correct per the MCP spec but may surprise integrators.

**Recommendation:** Add a note in `docs/mcp-server.md` about the required Accept header.

### Issue 2: Cost Store Not Available Without Database

Without `--db` pointing to a valid SQLite database, cost data is unavailable.
The `get_cost_summary` MCP tool returns "no cost data available".

**Recommendation:** This is expected for the current milestone. Cost tracking
requires session recording infrastructure (Phase 3 scope).

### Issue 3: Dependent Issue Counting Always Returns 0

The `Blocks: 0 dependent issues` line always shows 0 because the current
implementation doesn't cross-reference `depends_on` frontmatter between issues.

**Recommendation:** Known gap from spike #11 findings. Implement dependency
graph traversal when the dispatcher is fully wired.

## GO/NO-GO Decision

**GO** -- Phase 2 check-in MVP is validated.

### What Works

- Full digest pipeline: GitHub API -> frontmatter parsing -> priority sorting -> conversational format
- MCP server: Streamable HTTP transport, tool discovery, tool invocation with real data
- Server infrastructure: health endpoint, conditional MCP routing, graceful shutdown
- Cost adapter: interface wired, graceful degradation when no store available
- Agent prompt: comprehensive design covering all interaction patterns

### What's Deferred

- Live Claude MCP integration test (requires Claude Desktop config)
- Quick-action execution (needs `add_label`/`remove_label` MCP tools)
- Cost tracking (needs session recording infrastructure)
- Dependent issue counting (needs dependency graph)
- Device adaptation (needs user profile integration)

### Next Phase Priorities

1. Wire remaining forge operations as MCP tools (label changes, comments)
2. Implement session recording for cost tracking
3. Test with Claude Desktop as MCP client
4. Add authentication middleware for production use
