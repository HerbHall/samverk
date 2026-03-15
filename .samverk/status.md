---
phase: foundation-rebuild
updated: 2026-03-15T22:00:00Z
updated_by: claude-code
---

# Samverk -- Current State

## Phase

Phase 5A: Foundation rebuild. Dispatcher re-enabled on CT 202 with workspace isolation. Implementing solo developer agent model (ADR-035). Next: agent tooling (#521) and end-to-end verification.

## What Is Running

- Samverk server: CT 202 (192.168.1.162:8080) -- healthy, API auth enabled
- MCP endpoint: POST /mcp (Streamable HTTP, bearer auth)
- Dispatcher: RUNNING (1 worker, workspace isolation via /var/lib/samverk/repo)
- PR watcher: runs concurrently with dispatcher (auto-merge eligible PRs)
- Gitea: CT 200 (192.168.1.160:3000 / gitea.herbhall.net) -- primary runtime forge
- Staging: CT 203 (192.168.1.199:8080) -- available for testing (not yet deployed with latest)

## Phase 5A Progress

### Completed

| Issue | Title | PR | Status |
|-------|-------|----|--------|
| #501 | allowedTools fix for Claude CLI | -- | CLOSED (merged) |
| #503-506 | Provider logging (all 4 providers) | #526 | MERGED |
| #515 | Tier enforcement in agent runner | #522 | MERGED |
| #516 | Intelligent failure response engine | #525 | MERGED |
| #519 | Staging CT 203 | #524 | MERGED |
| #517 | Isolated agent workspaces (git worktrees) | #529 | MERGED |
| #535 | Wire SetRepoDir in dispatcher startup | #536 | MERGED |
| #518 | Pre-posting validation gate | #537 | MERGED |

### Remaining

| Issue | Title | Depends On | Status |
|-------|-------|------------|--------|
| #521 | Agent tooling (DevKit rules, MCP, Synapset) | #517 | Not started |
| #527 | Research: Docker containers as agent workspaces | -- | Not started |

### Gate Criteria (Resume Dispatcher)

Progress toward full gate:

1. ~~#517 merged (workspace isolation)~~ DONE
2. #521 merged (agent tooling parity) -- NOT YET
3. At least one successful end-to-end agent run on staging (CT 203) -- NOT YET
4. Failure response engine verified with at least 3 failure classes -- NOT YET

Dispatcher is running in production with partial gate (workspace isolation + validation gate active). Full gate completion requires #521 and e2e verification.

### Known Issues

- SQLITE_BUSY warnings in logs: autoscaler and metric writer contend on shared SQLite DB when serve and dispatch run concurrently. Non-blocking but should be addressed.

## Architecture

- ADR-035: Solo developer agent model (proposed) -- agents operate as independent developers with git worktrees, tiered permissions, and intelligent failure response
- All 10 agent types use single unified workflow with tiered permissions (read-only, code-write, file-write, manual)

## Start Here (Cold Start Protocol)

1. Read this file
2. Call `samverk get_digest --since 168h` if MCP is configured
3. Check open issues if relevant to the task
4. Proceed -- do not ask the user to explain project state
