---
phase: foundation-rebuild
updated: 2026-03-15T18:00:00Z
updated_by: claude-code
---

# Samverk -- Current State

## Phase

Phase 5A: Foundation rebuild. Dispatcher OFF pending pipeline fixes. Implementing solo developer agent model (ADR-035). Focus: fix the agent pipeline so the tool can build itself.

## What Is Running

- Samverk server: CT 202 (192.168.1.162:8080) -- healthy, API auth enabled
- MCP endpoint: POST /mcp (Streamable HTTP, bearer auth)
- Dispatcher: STOPPED (1 worker, scaling disabled) -- pending Phase 5A completion
- PR watcher: runs concurrently with dispatcher (auto-merge eligible PRs)
- Gitea: CT 200 (192.168.1.160:3000 / gitea.herbhall.net) -- primary runtime forge
- Staging: CT 203 (192.168.1.199:8080) -- available for testing

## Phase 5A Progress

### Completed

| Issue | Title | PR | Status |
|-------|-------|----|--------|
| #501 | allowedTools fix for Claude CLI | -- | CLOSED (merged) |
| #503-506 | Provider logging (all 4 providers) | #526 | MERGED |
| #515 | Tier enforcement in agent runner | #522 | MERGED |
| #516 | Intelligent failure response engine | #525 | MERGED |
| #519 | Staging CT 203 | #524 | MERGED |
| #517 | Isolated agent workspaces (git worktrees) | #529 | OPEN (CI passing, auto-merge) |

### Remaining

| Issue | Title | Depends On | Status |
|-------|-------|------------|--------|
| #518 | Pre-posting validation gate | #517 | Not started |
| #521 | Agent tooling (DevKit rules, MCP, Synapset) | #517 | Not started (Wave 3) |
| #527 | Research: Docker containers as agent workspaces | -- | Not started |

### Gate Criteria (Resume Dispatcher)

All of the following must be true before re-enabling the dispatcher:

1. #517 merged (workspace isolation)
2. #521 merged (agent tooling parity)
3. At least one successful end-to-end agent run on staging (CT 203)
4. Failure response engine verified with at least 3 failure classes

## Architecture

- ADR-035: Solo developer agent model (proposed) -- agents operate as independent developers with git worktrees, tiered permissions, and intelligent failure response
- All 10 agent types use single unified workflow with tiered permissions (read-only, code-write, file-write, manual)

## Start Here (Cold Start Protocol)

1. Read this file
2. Call `samverk get_digest --since 168h` if MCP is configured
3. Check open issues if relevant to the task
4. Proceed -- do not ask the user to explain project state
