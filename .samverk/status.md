---
phase: agent-autonomy
updated: 2026-03-17T23:50:00Z
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

Note: Ollama restricted to triage-only (PR #613). Code-gen routes to Claude CLI/API.

## Open Issues

0 open issues. All resolved or closed.

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

1. Run dispatcher on real issues to validate autonomy end-to-end
2. Address medium gaps (Synapset#62 parse error, DevKit dashboard native React)
3. Consider filing new issues as needed

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
