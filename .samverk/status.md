---
phase: agent-autonomy
updated: 2026-03-21T14:30:00Z
updated_by: claude-code
---

# Samverk -- Current State

## Phase

Agent Autonomy -- getting Samverk to run itself. Infrastructure complete.
All critical and high autonomy gaps now have fixes in PRs or merged.

## What Is Running

- Samverk server: CT 202 (192.168.1.162:8080) -- healthy
- MCP-only listener: CT 202 port 8081 (no SPA, no auth)
- Claude.ai Custom Connector: `https://samverk.herbhall.net/mcp` -- CONNECTED
- Dispatcher: RUNNING (1-5 workers, 6 providers, free-first routing)
- Health monitor: 60s probes, WoL for sleeping hosts
- Watcher: auto-restart with backoff (no more silent hangs)
- Dashboard: unified with Synapset native + DevKit iframe
- Gitea CI: CT 200 (80GB disk, daily cleanup cron at 3am)
- Cloudflare Tunnel: e86ba6e3 (samverk + synapset + mcp subdomains)

### GPU Fleet

| Host | GPU | Model | Routing |
|------|-----|-------|---------|
| HDH-NZXT | RTX 5090 32GB | qwen3-coder:30b | triage, docs, research |
| VM 300 | RTX 3090 Ti 24GB | qwen2.5-coder:14b | triage, docs, research |
| CM-ASUS | RTX 2080 Ti 11GB | qwen2.5-coder:7b | triage |

Note: qwen3-coder:30b promoted to code-gen routing (PR #134, #113). Runtime validator is the protection.

## Open Issues

Remaining: agent:human (#57, #70, #72, #74). Wave 12 + CF auth housekeeping complete.

### Planned Next Waves

| Wave | Issues | What |
|------|--------|------|
| A | #157, #158 | CF Access for synapset.herbhall.net + security docs |
| B | #57 Option B | Structured JSON output for Ollama code-gen |
| C | #70 sub-issues (#64, #67, #69) | MCP parity tools |
| D | #72 Phase 1+2 | WebSocket hub + expanded API |

## Gaps to Full Autonomy

### Resolved This Session

1. **Ollama output quality** -- FIXED: Restricted to triage-only + output validation guard (PR #613)
2. **Claude CLI hangs** -- FIXED: 30s timeout + startup detection + auto-failover (PR #611)
3. **No planning step** -- FIXED: Explore phase reads CLAUDE.md + sibling files (PR #614)
4. **DevKit data on local machine** -- RESOLVED: Synapset covers knowledge, server self-sufficient (#609 closed)
5. **Copilot review feedback** -- FIXED: PR watcher reads Copilot comments before merge (PR #610)

### Medium (remaining)

1. Synapset parse error (Synapset#62 filed)
2. `CF_ACCESS_TEAM_DOMAIN` env var on CT 202 to activate CF auto-login (#88 shipped, waiting on Cloudflare Access setup)

## Recommended Next Session

1. Set `SAMVERK_DEVKIT_PATH` on CT 202 to activate DevKit native page
2. Configure Cloudflare Access policy for samverk.herbhall.net, then set `CF_ACCESS_TEAM_DOMAIN=herbhall.cloudflareaccess.com` on CT 202
3. Address agent:human issues (#57 Ollama code-gen validation, #70 MCP parity, #72 full dashboard)
4. Observe Quality page advisory panel after failure events accumulate (5+ events needed for KPIs)

## Session Summary (2026-03-21, session 2)

CF Access diagnosis and housekeeping session.

### Work Done

- Diagnosed CF Access not enforcing: root cause was `CF_ACCESS_TEAM_DOMAIN` not set (not missing IdP)
- Set `CF_ACCESS_TEAM_DOMAIN=herbhall.cloudflareaccess.com` on CT 202 → JWT auto-login (#88) now active
- Removed invalid `mcp.herbhall.net` tunnel entry + DNS record (agent-created workaround from prior session)
- Corrected CF Access architecture understanding: `samverk.herbhall.net/mcp` and `synapset.herbhall.net/mcp` are the only MCP endpoints
- Filed issues #155-#158 (CF Access diagnosis, JWT activation, synapset protection, security docs)
- Closed #155, #156 as resolved; #157 and #158 queued for automation
- Filed DevKit #487 (homelab security architecture docs)
- Closed #114 (self-healing KPI research -- deliverables already met by Wave 11)
- Housekeeping: closed stale PRs #40/#41, removed 7 old worktrees, cleared 9 old stashes, deleted all stale local branches
- Verified dashboard loads without token prompt at samverk.herbhall.net (CF JWT auto-login confirmed)
- Remembered: setup RDP on HDH-MSP8 (work laptop) for external network testing

### Security Architecture (clarified)

- Internal (LAN): bearer token only, minimal
- External (samverk.herbhall.net): CF Access Google OAuth (browser) + service token (programmatic)
- MCP endpoints: same domain/CF app, service token bypass
- Tailscale: separate, not part of CF stack
- mcp.herbhall.net: INVALID -- removed

## Session Summary (2026-03-21, session 1)

Wave 12 complete. 4 issues resolved (2 code-gen + 2 docs). Deployed successfully.

### Work Done

- #120 -- Advisory engine: background goroutine (15-min refresh), 5 pattern detectors, GET /api/v1/quality/recommendations, advisory panel on Quality page
- #88 -- CF Access JWT auto-login: `CFAccessMiddleware` with RS256 JWKS validation, 24h cache, `CF_ACCESS_TEAM_DOMAIN` env var gating
- #121 -- Dashboard IA spec: `docs/dashboard-ia.md` -- goals, 13-page inventory, 4 information groups, evaluation criteria
- #127 -- Dashboard evergreen process: `docs/dashboard-evergreen.md` + `.github/PULL_REQUEST_TEMPLATE.md` with evergreen checklist

### Architecture

Quality page self-healing loop now fully wired: failure_events → RCA fields → KPI computation → advisory engine pattern detection → recommendations panel.

### Lint gotcha

Parallel worktree agents use golangci-lint cache; missed 2 prealloc violations in `detectors.go` that passed pre-push but failed CI. Fixed immediately (2 min). Root cause: lint cache in worktree doesn't reset between runs.

## Session Summary (2026-03-20, session 5)

Wave 11 complete. 5 issues resolved (design + code-gen). Deployed v0.1.22-28.

### Work Done

- #115 -- RCA documentation standard (docs/rca-standard.md): 7 structured fields, enum values
- #116 -- KPI framework (docs/kpi-framework.md): 10 KPIs with SQL queries and targets
- #119 -- Gitea issue templates: bug-report.md with RCA fields, task.md schema template
- #117 -- failure_events schema: 10 RCA columns, incremental ALTER TABLE migration, GET /api/v1/kpis
- #118 -- Quality page: 4 KPI stat cards, root cause PieChart donut, advisory placeholder

### Architecture

Self-healing feedback loop now complete: agents file failures → RCA fields captured →
KPIs computed from failure_events → Quality page surfaces trends → advisory engine (#120) next.

## Session Summary (2026-03-20, session 4)

Wave 10 complete. 5 parallel agents, 5 PRs merged, 3 new pages added to dashboard. Deployed v0.1.22-22.

### Dashboard Pages Added

- #122 -- MCP page: server health cards (Samverk + Synapset), tool count via in-process ping
- #123 -- Projects page: forge health cards + per-project issue counts (open, needs-human, in-progress)
- #124 -- Provider health fixed: store-backed snapshots bridge dispatch→serve process gap; 503 resolved
- #125 -- Data page: SQLite + Synapset data source cards with size/record counts
- #126 -- Dashboard redesign: health banner, needs-attention badges, stat cards replacing old 4-item layout

### Architecture Decision

Provider health API was broken because serve and dispatch are separate OS processes. Fixed using SQLite store as bridge (same pattern as metrics snapshots). Dispatch writes 30s snapshots; serve reads them.

### Merge Complexity

Two PRs (#122, #123) required manual rebase after sequential Gitea squash merges diverged them. Auto-resolved "add both sides" conflicts in api.go and api.ts. api.ts had malformed interfaces after auto-resolve (missing closing braces) -- fixed manually.

## Session Summary (2026-03-20, session 3)

Wave 9 complete. 3 issues resolved, 1 PR merged, deployed v0.1.22-15.

### Work Done

- #138 -- Multi-repo dispatch: already working, closed (dispatcher logs confirmed devkit + synapset polling)
- #139 -- Synapset parse error: fixed by passing `format: "json"` to search_memory/search_all calls
- #140 -- DevKit native React: new `/api/v1/devkit/summary` endpoint + two-column rules/skills/agents view

### Infrastructure

Discovered and documented dual-forge sync pattern: Gitea squash merges diverge from GitHub main. Procedure: fast-forward local main from Gitea, push to GitHub, force-push to Gitea (with admin bypass).

### Deployed

CT 202 running `v0.1.22-15-g2b9fe3b`. To activate DevKit page: set `SAMVERK_DEVKIT_PATH=/path/to/devkit` in samverk-serve environment.

## Session Summary (2026-03-20, session 2)

Wave 8 complete. 4 agents, 4 PRs merged to Gitea. GitHub synced via cherry-pick.

### Features Added

- #110 -- Issue search: `GET /api/v1/issues/search`, debounced frontend search bar in Issues page
- #111 -- Bulk operations: `POST /api/v1/issues/bulk`, checkbox multi-select, floating toolbar, toasts
- #112 -- Nightly infra probe at 3am goroutine wired into serve command (nil-safe synapset client)
- #113 -- qwen3-coder:30b promoted to code-gen routing; CLAUDE.md-only validator test added

## Session Summary (2026-03-20, session 1)

Wave 7 complete. 5 parallel agents, 5 PRs merged to both forges.

### Dashboard Features Added

- #105 -- WorkerDetailPanel: slide-over with session log + token usage on worker card click
- #106 -- Mobile bottom tab bar (< 768px): Dashboard, Issues, Agents, Metrics, Logs
- #107 -- Queue depth sparkline on Dashboard (30-min history, Recharts AreaChart, 30s refresh)
- #108 -- Live log stream: backend broadcasts `log.entry` WS events; Logs page Live toggle + 500-entry cap
- #109 -- Chat drawer complete: multi-turn state, typing indicator, Escape/click-outside dismiss

### Infrastructure Note

Gitea main was 139 commits behind GitHub main (dual-forge drift). Force-synced GitHub → Gitea before
Wave 7 merges. Branch protection temporarily disabled during sync, restored after.

## Session Summary (2026-03-17, session 3)

Housekeeping and bug fix session.

### PRs Merged

- #644 -- Decouple MCP handler init from GitHub env vars (Gitea #38)
- #645 -- Phase-aware routing + set_project_phase MCP tool (Gitea #35, #36)
- #647 -- Revise ADR-031 to single-forge-per-project model (Gitea #39)
- #648 -- Fix get_digest "away" duration (echoed `since` param instead of real elapsed time)

### Synapset v0.3.1 Released

Synapset semantic-release pipeline fully working. Key fixes:

- Remove `@semantic-release/git` + changelog plugins (incompatible with branch protection)
- Add `repositoryUrl` for correct release note links
- Second `url.insteadOf` to bypass Cloudflare auth header stripping on tag push
- Gitea disk crisis resolved: CT 200 resized 40GB → 80GB, daily actcache pruning installed

### Prior Session (2026-03-17, session 2)

Sprint: 4 worktree agents, 4 PRs, 7 issues resolved. Key fixes: Ollama triage restriction, CLI timeout, explore-before-code, Copilot review watcher.

## Start Here

1. Read this file
2. Check open issues if relevant
3. Proceed -- do not ask the user to explain project state
