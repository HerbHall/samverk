---
phase: 5
updated: 2026-03-08T00:15:00Z
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

- #183: create .samverk/status.md -- this file
- #184: add cold-start protocol to CLAUDE.md

## Queued

- #152: dispatcher routes to Copilot as provider
- #153: dispatch feedback loop (depends on #144 research)
- #157: Claude Code Remote Control spike (human task)
- #186: `samverk status --write` CLI automation
- #187: document repo-first principle in multi-session-safety.md

## Last Session Summary

Merged PR #188 fixing dispatcher false-positive escalation on issues without
YAML frontmatter (issue #180). Added heuristic classification using labels and
title prefixes. Created repo-first session orientation files.

## Start Here (Cold Start Protocol)

1. Read this file
2. Call `samverk get_digest --since 168h` if MCP is configured
3. Read open issues if relevant to the task
4. Proceed -- do not ask the user to explain project state
