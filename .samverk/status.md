---
phase: 5
updated: 2026-03-08T01:30:00Z
updated_by: claude-code
---

# Samverk -- Current State

## Phase

Phase 5 in progress: agent runtime, provider integration, SPA embedding.
Phase 4 complete (2026-03-02): MCP tools, REST API, dispatcher CLI, multi-project, web dashboard scaffold.

## What Is Running

- Samverk server: CT 202 (192.168.1.162:8080) -- healthy
- MCP endpoint: POST /mcp (Streamable HTTP, auth required)
- Dispatcher: running continuously (systemd, 30s poll, 3 workers)

## In Flight

- #185: roll out status.md and cold-start to registered projects (this PR)

## Queued

- #152: dispatcher routes to Copilot as provider
- #153: dispatch feedback loop (depends on #144 research)
- #157: Claude Code Remote Control spike (human task)
- #186: `samverk status --write` CLI automation
- #187: document repo-first principle in multi-session-safety.md

## Last Session Summary

Rolled out .samverk/status.md and cold-start protocol to all 4 registered
projects (subnetree, dockpulse, runbooks, devkit). Fixed dispatcher
false-positive (#180, PR #188). Created status.md (#183) and cold-start
protocol (#184, PR #190).

## Start Here (Cold Start Protocol)

1. Read this file
2. Call `samverk get_digest --since 168h` if MCP is configured
3. Read open issues if relevant to the task
4. Proceed -- do not ask the user to explain project state
