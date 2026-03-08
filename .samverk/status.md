---
phase: execution
updated: 2026-03-08T15:30:00Z
updated_by: claude-code
---

# Samverk -- Current State

## Phase

Phase 5 complete: agent runtime, provider integration, SPA embedding, PR watcher.
Phase 4 complete (2026-03-02): MCP tools, REST API, dispatcher CLI, multi-project, web dashboard scaffold.

## What Is Running

- Samverk server: CT 202 (192.168.1.162:8080) -- healthy
- MCP endpoint: POST /mcp (Streamable HTTP, auth required)
- Dispatcher: running continuously (systemd, 30s poll, 3 workers)
- PR watcher: runs concurrently with dispatcher (auto-merge eligible PRs)

## Completed This Session

- #233: failure counter preserves across re-queue cycles (PR #249)
- #237: suppress 404 on RemoveLabel when label not present (PR #249)
- #239: Ollama NewWithTimeout + wire timeout_seconds config (PR #249)
- fix: heartbeat interval 10min → 20min for opus session headroom (PR #249)
- Dispatcher restarted on CT 202, running clean

## Queued

- #152: dispatcher routes to Copilot as provider
- #153: dispatch feedback loop (depends on #144 research)
- #157: Claude Code Remote Control spike (human task)
- #186: `samverk status --write` CLI automation

## Last Session Summary

Applied 4 agent-generated fixes (handoff #3). Fixed the failure counter
reset bug (#233) that caused infinite re-queue loops, suppressed 404
false errors on RemoveLabel (#237), added configurable Ollama timeout
(#239), and increased heartbeat interval to 20min for opus headroom.
All in PR #249 (auto-merge queued). Dispatcher running clean on CT 202.

## Start Here (Cold Start Protocol)

1. Read this file
2. Call `samverk get_digest --since 168h` if MCP is configured
3. Read open issues if relevant to the task
4. Proceed -- do not ask the user to explain project state
