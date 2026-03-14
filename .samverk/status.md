---
phase: execution
updated: 2026-03-14T01:12:36Z
updated_by: claude-code
---

# Samverk -- Current State

## Phase

Phase 5 complete. Q2 2026 execution: 3 streams (B/W/P), 62 issues. Security batch (#407-#410, #399) done. Auth middleware, dashboard token injection, and cross-process metrics fix deployed.

## What Is Running

- Samverk server: CT 202 (192.168.1.162:8080) -- healthy, API auth enabled
- MCP endpoint: POST /mcp (Streamable HTTP, bearer auth)
- Dispatcher: running continuously (systemd, 30s poll, 3 workers, autoscaling 1-5)
- PR watcher: runs concurrently with dispatcher (auto-merge eligible PRs)
- Gitea: CT 200 (192.168.1.160:3000 / gitea.herbhall.net) -- primary runtime forge

## Recent Completions

### Security Batch (2026-03-13 to 2026-03-14)

- PR #420: BearerAuth middleware on all API and MCP routes
- PR #421: Dashboard auth token injection into SPA via `window.__SAMVERK_TOKEN__`
- PR #422: Scoped worker identity in KeyStore (per-worker API keys)
- PR #423: Cross-process metrics bridge between dispatch and serve
- PR #425: Restored systemd hardening with `.claude` ReadWritePaths

### Dispatcher Improvements (2026-03-13)

- PR #402: Dynamic per-issue timeout based on complexity
- PR #416: Cross-model QC routing via dedicated provider chain
- PR #417: Per-issue token aggregation with outlier detection
- PR #418: PROGRESS comment protocol for periodic mid-task state

### Agent Runtime (2026-03-09)

- PR #397: Migrated logging from slog to zap with dual-mode output
- PR #398: Pagination for label cache, issue listing, and comments
- PR #400: Inline SPA build in Gitea CI
- PR #403: Query check runs API for GitHub Actions CI status
- PR #405: Streaming progress detection with heartbeat reset
- PR #406: Session checkpoint and resume

### Other

- PR #415: Copilot #402 followup + multi-agent research docs
- PR #419: Production pipeline design v0.9
- PR #404: Release 0.1.13

## Open Issues by Category

### In Progress / Claimed

- #245: feat: pre-flight issue decomposition for oversized tasks (status:claimed, status:blocked)

### Blocked

- #282: End-to-end validation: full agent loop on Gitea (agent:infra, status:blocked)
- #314: Verify: PC agent runs autonomously for 2 hours (agent:pc, status:blocked)
- #317: Verify: Multi-session PC agent processes a full batch (agent:pc, status:blocked)

### Needs Human / Human Pending

- #186: feat: samverk status --write CLI command (status:needs-human)
- #246: feat: timeout calibration feedback loop (status:needs-human)
- #248: docs: stale documentation vs actual deployed state (status:needs-human)
- #250: epic: GitHub to Gitea migration strategy (agent:human, status:needs-human)
- #251: epic: documentation integrity system (agent:human, status:needs-human)
- #252: research: Samverk GitHub to Gitea migration requirements (agent:human, status:needs-human)
- #283: End-to-end validation: conversational check-in via MCP (agent:human, status:human-pending)
- #291: Verify: Metrics visible on dashboard and in MCP digest (agent:human, status:human-pending)
- #303: Verify: Full adaptive scaling lifecycle over 24-hour soak (agent:human, status:human-pending)
- #315: Implement multi-session CC support with concurrent worktrees (status:needs-human)
- #323: epic: automated failure analysis loop (agent:human, status:needs-human)
- #392: infra: fix RTX 5090 GPU passthrough to Docker Desktop (agent:human, status:needs-human)
- #411: sec: ollama.herbhall.net exposed without authentication (agent:human, status:needs-human)

### Queued

- #359: test: Add FormatDuration helper to pkg/models (agent:code-gen, status:needs-qc)

## Start Here (Cold Start Protocol)

1. Read this file
2. Call `samverk get_digest --since 168h` if MCP is configured
3. Check open issues if relevant to the task
4. Proceed -- do not ask the user to explain project state
