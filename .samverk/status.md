---
phase: execution
updated: 2026-03-14T12:00:00Z
updated_by: claude-code
---

# Samverk -- Current State

## Phase

Phase 5 complete. Q2 2026 execution: 3 streams (B/W/P), 62 issues. Security batch done. Dispatcher improvements (timeout calibration, pre-flight decomposition, cross-model QC, PROGRESS protocol) deployed. Now implementing #323: automated failure analysis loop.

## What Is Running

- Samverk server: CT 202 (192.168.1.162:8080) -- healthy, API auth enabled
- MCP endpoint: POST /mcp (Streamable HTTP, bearer auth)
- Dispatcher: running continuously (systemd, 30s poll, 3 workers, autoscaling 1-5)
- PR watcher: runs concurrently with dispatcher (auto-merge eligible PRs)
- Gitea: CT 200 (192.168.1.160:3000 / gitea.herbhall.net) -- primary runtime forge

## Recent Completions

### Wave 1 Batch (2026-03-14)

- PR #427: `samverk status` CLI command (#186 CLOSED)
- PR #428: Documentation refresh + operations guide (#248 CLOSED)
- PR #429: Timeout calibration feedback loop with historical p90 (#246 CLOSED)
- PR #430: Pre-flight issue decomposition for oversized tasks (#245 CLOSED)
- #359 CLOSED: FormatDuration helper promoted to pkg/models (PR #424)
- #411 CLOSED: Caddy basicauth on ollama.herbhall.net

### Security Batch (2026-03-13 to 2026-03-14)

- PR #420: BearerAuth middleware on all API and MCP routes (#407, #408 CLOSED)
- PR #421: Dashboard auth token injection (#409 CLOSED)
- PR #422: Scoped worker identity (#410 CLOSED)
- PR #423: Cross-process metrics bridge (#399 CLOSED)
- PR #425: Restored systemd hardening

### R-stream Hardening (2026-03-13)

- PR #416: Cross-model QC routing (#412 CLOSED)
- PR #417: Per-issue token aggregation (#414 CLOSED)
- PR #418: PROGRESS comment protocol (#413 CLOSED)

## Open Issues by Category

### In Progress

- #323: epic: automated failure analysis loop (active work this session)

### Blocked

- #282: End-to-end validation: full agent loop on Gitea (agent:infra)
- #314: Verify: PC agent runs autonomously for 2 hours (agent:pc)
- #317: Verify: Multi-session PC agent processes a full batch (agent:pc)

### Needs Human / Human Pending

- #250: epic: GitHub to Gitea migration strategy (agent:human)
- #251: epic: documentation integrity system (agent:human)
- #252: research: Samverk GitHub to Gitea migration requirements (agent:human)
- #283: End-to-end validation: conversational check-in via MCP (human-pending)
- #291: Verify: Metrics visible on dashboard and in MCP digest (human-pending)
- #303: Verify: Full adaptive scaling lifecycle over 24-hour soak (human-pending)
- #315: Implement multi-session CC support (needs design decision)
- #392: infra: fix RTX 5090 GPU passthrough to Docker Desktop

## Start Here (Cold Start Protocol)

1. Read this file
2. Call `samverk get_digest --since 168h` if MCP is configured
3. Check open issues if relevant to the task
4. Proceed -- do not ask the user to explain project state
