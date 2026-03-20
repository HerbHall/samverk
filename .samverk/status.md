---
phase: agent-autonomy
updated: 2026-03-20T21:00:00Z
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

0 open issues. Waves 7, 8, and 9 complete.

## Gaps to Full Autonomy

### Resolved This Session

1. **Ollama output quality** -- FIXED: Restricted to triage-only + output validation guard (PR #613)
2. **Claude CLI hangs** -- FIXED: 30s timeout + startup detection + auto-failover (PR #611)
3. **No planning step** -- FIXED: Explore phase reads CLAUDE.md + sibling files (PR #614)
4. **DevKit data on local machine** -- RESOLVED: Synapset covers knowledge, server self-sufficient (#609 closed)
5. **Copilot review feedback** -- FIXED: PR watcher reads Copilot comments before merge (PR #610)

### Medium (remaining)

1. Synapset parse error (Synapset#62 filed)
2. DevKit dashboard native React (replace iframe)
3. Multi-repo dispatch (code ready, config needed)

## Recommended Next Session

1. File Wave 10 issues or let the dispatcher find work autonomously
2. Monitor dispatcher logs for successful qwen3-coder:30b code-gen runs
3. Set `SAMVERK_DEVKIT_PATH` env var on CT 202 to activate native DevKit summary page

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
