---
phase: agent-autonomy
updated: 2026-03-28T23:30:00Z
updated_by: claude-code
---

# Samverk -- Current State

## Phase

Agent Autonomy -- pipeline running autonomously. 4 runners online, dispatcher unpaused.

## What Is Running

- Samverk server: CT 202 (192.168.1.162:8080) -- healthy
- Dispatcher: RUNNING (1-5 workers, 8 providers, 3 projects)
- Projects: samverk, devkit, synapset (all Gitea, all with dedicated repo clones)
- Synapset memory: AUTHENTICATED
- Go 1.24.1 + Node v22.22.1 + pnpm 10.33.0: INSTALLED on CT 202
- Dashboard: unified with Synapset native + DevKit iframe
- Gitea CI: CT 200 (80GB disk, daily cleanup cron at 3am)
- Cloudflare Tunnel: e86ba6e3 (samverk + synapset + mcp subdomains)

### GPU Fleet

| Host | GPU | Model | Routing |
|------|-----|-------|---------|
| HDH-NZXT | RTX 5090 32GB | qwen3-coder:30b | default, local |
| VM 300 | RTX 3090 Ti 24GB | qwen2.5-coder:32b | triage, local |
| CM-ASUS | RTX 2080 Ti 11GB | qwen2.5-coder:7b | triage |

### Gitea Actions Runners (all online)

| Runner | Machine | Auto-start |
|--------|---------|-----------|
| samverk-runner (ID 7) | CT 200 | systemd enabled |
| hdh-d10u-runner (ID 6) | HDH-D10U | systemd enabled |
| unraid-runner (ID 5) | HDH-UNRAID | /boot/config/go script |
| hdh-nzxt-win (ID 4) | HDH-NZXT | Scheduled Task (logon) |

## Blocking Issue

None.

## Open Issues Summary (50 open / 50 closed)

| Status | Count | Notes |
|--------|-------|-------|
| needs-qc | 23 | Agent completed work, awaiting QC review |
| needs-human | 12 | 7 genuinely blocked, 2 planning (blocked on deps), 3 misc |
| queued | 11 | Pipeline processing (includes 3 planning Wave 1 issues) |
| blocked | 7 | Dependency chains |
| claimed | 5 | Agent actively working |

### Open PRs (4 remaining)

PR #435 (diagnostics endpoint) and #440 (canary opus) are CI-green, awaiting tier-2 auto-merge.

### Agent Performance Baselines (2026-03-28)

| Provider | Chain | Time | Output |
|----------|-------|------|--------|
| Ollama qwen3-coder:30b | local | 47s | Comment EDIT |
| Ollama qwen3-coder:30b | triage | 52s | Comment EDIT |
| Claude Sonnet 4.6 | code-gen | 57s | Comment EDIT |
| Claude Opus 4.6 | complex | 78s | Full git PR |

Note: Only claude-opus (complex chain) creates actual git PRs. All other providers produce comment-based EDIT output.

### Needs-Human Issues (15)

**Genuinely blocked on human:**

- #366 -- Split-horizon DNS + CF OAuth for remote MCP access
- #446 -- Docker Desktop feature audit

**Planning system (design session completed 2026-03-28):**

- #239 -- Agent capability profiles schema -- **QUEUED** (Wave 1)
- #242 -- Readiness review pre-filter -- **QUEUED** (Wave 1, rewritten as dispatcher function)
- #215 -- Research routing -- **QUEUED** (Wave 1, scope reduced to routing only)
- #214 -- Planning agent -- BLOCKED on #239 (Wave 2)
- #245 -- Continuous improvement -- DEFERRED (Wave 3, needs operational data)

See `docs/planning-system-design.md` for decisions and sequencing.

**Remaining (7 misc):**

- Various pipeline quality and UI issues awaiting human decisions

## Session Summary (2026-03-28 night)

Planning system design session -- resolved architectural decisions for 5 blocked issues.

### Planning System Decisions

- Created `docs/planning-system-design.md` with 5 architectural decisions
- #242 readiness review: rewritten as dispatcher pre-filter (no LLM on fast path)
- #215 research routing: scope reduced to routing only, per-issue phase gating (not per-project)
- #239 capability profiles: unblocked for pipeline (already had complete handoff spec)
- #214 planning agent: blocked on #239, sequencing comment added
- #245 continuous improvement: deferred to Wave 3, needs operational data
- 3 issues moved from needs-human to queued (Wave 1 parallel execution)

### DevKit Fix

- Found `claude/shared/record-usage.md` missing from all machines -- sync.ps1 never symlinks `shared/` directory
- Root cause: commit 8404130 moved the file but never updated `.sync-manifest.json` or sync.ps1
- Manual fix applied on HDH-NZXT: `ln -s ~/.devkit-stable/claude/shared ~/.claude/shared`
- DevKit issue #451 filed for proper fix (sync.ps1 + manifest + fleet propagation)
- 5 skills affected: autolearn, code-review, conformance-audit, devkit-sync, quality-control

### Previous Session (2026-03-28 evening)

Major operational session -- fixed infrastructure, triaged full backlog, established baselines.

### Infrastructure Fixed

- GitHub MCP (Docker Gateway): OAuth token re-authorized
- Gitea Actions runner (CT 200): re-registered (token expired)
- HDH-NZXT runner: restarted (process stopped)
- UNRAID runner: added auto-start to /boot/config/go
- Node.js + pnpm installed on CT 202 (unblocks frontend agent work)
- Ollama models verified on all 4 GPU hosts
- Dispatcher unpaused

### Features Shipped

- PR #435: `get_diagnostics` MCP tool + REST endpoint (CI green)
- PR #440: E2E canary from claude-opus complex chain (CI green)

### Triage Actions

- 4 canary issues created, baselined, closed (#436-439)
- 8 failing PRs closed and source issues re-queued (stale main)
- 11 needs-human issues reclassified to queued (pipeline can handle)
- 4 incorrectly closed issues reopened (pipeline scope clarification)
- 2 dashboard issues filed (#447 sparklines, #448 stale metrics)
- 1 Docker Desktop research issue filed (#446)

### Key Learnings Stored

- Pipeline issues are multi-project -- don't close for scope
- Interactive sessions are pipeline operators, not workers
- Docker Desktop model runner on HDH-NZXT (port conflict with Ollama)
- Gitea runner registration expires silently
- Only claude-opus creates git PRs (sonnet/haiku produce comment EDITs)

## Recommended Next Session

1. **Monitor Wave 1 pipeline throughput** -- #239, #242, #215 are queued. Check if agents pick them up and produce CI-green PRs.
2. **Unblock #214 after #239 merges** -- relabel to `status:queued` once capability profiles land.
3. **QC backlog** -- 23 needs-qc issues. Batch review or let pipeline QC agents process.
4. **#366 remote MCP access** -- research CF MCP portal, test on isolated subdomain.

## Start Here

1. Read this file
2. Check `docs/planning-system-design.md` for planning system context
3. Check open issues if relevant
4. Proceed -- do not ask the user to explain project state
