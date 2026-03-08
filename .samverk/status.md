---
phase: 5
updated: 2026-03-08T21:00:00Z
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

- #194: specialized system prompts per agent type (PR #208)
- #203: dispatcher skips PRs, no longer escalates (PR #205)
- #202: handle issue.assigned events (PR #205)
- #195: code-gen and test agents open PRs with EDIT blocks (PR #207)
- #204: PR watcher auto-merges eligible PRs (PR #209)
- #197: SPA build/embed in CI workflow (PR #212)
- #198: end-to-end integration tests (PR #213)
- #210: make redeploy works on Windows, stops services before scp (PR #214)

## Queued

- #152: dispatcher routes to Copilot as provider
- #153: dispatch feedback loop (depends on #144 research)
- #157: Claude Code Remote Control spike (human task)
- #186: `samverk status --write` CLI automation

## Last Session Summary

Completed 8 issues across 7 PRs. Added specialized agent prompts,
fixed dispatcher PR/issue conflation and unknown event types, enabled
code-gen agents to open PRs, added PR watcher with auto-merge,
built SPA in CI, created integration tests, and fixed Windows deploy.

## Start Here (Cold Start Protocol)

1. Read this file
2. Call `samverk get_digest --since 168h` if MCP is configured
3. Read open issues if relevant to the task
4. Proceed -- do not ask the user to explain project state
