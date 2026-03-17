---
phase: agent-autonomy
updated: 2026-03-17T06:00:00Z
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
- Gitea CI: CT 200 (40GB disk, weekly cleanup cron)
- Cloudflare Tunnel: e86ba6e3 (samverk + synapset + mcp subdomains)

### GPU Fleet

| Host | GPU | Model | Routing |
|------|-----|-------|---------|
| HDH-NZXT | RTX 5090 32GB | qwen3-coder:30b | triage, docs, research |
| VM 300 | RTX 3090 Ti 24GB | qwen2.5-coder:14b | triage, docs, research |
| CM-ASUS | RTX 2080 Ti 11GB | qwen2.5-coder:7b | triage |

Note: Ollama restricted to triage-only (PR #613). Code-gen routes to Claude CLI/API.

## Open Issues

2 remain, both deferred strategic decisions:

- #252 -- Gitea migration requirements checklist (deferred)
- #250 -- GitHub to Gitea migration strategy (deferred)

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

1. Deploy to CT 202 after PRs merge (`make redeploy`)
2. Run dispatcher on test issues to validate all 5 fixes end-to-end
3. Address medium gaps if autonomy validated
4. Consider v0.1.19 release (release-please PR #612 ready)

## Session Summary (2026-03-17, session 2)

Sprint: 4 worktree-isolated agents, 4 PRs, 7 issues resolved.

### PRs Created

- #610 -- Copilot feedback before auto-merge (closes #608) -- MERGED
- #611 -- CLI timeout + provider failover (closes #606) -- auto-merging
- #613 -- Ollama triage restriction + output validation (closes #605) -- auto-merging
- #614 -- Explore-before-code planning step (closes #607) -- auto-merging

### Issues Resolved

- #605 -- Ollama agents overwrite CLAUDE.md (PR #613)
- #606 -- Claude CLI hangs on CT 202 (PR #611)
- #607 -- Explore-before-code planning step (PR #614)
- #608 -- Copilot review feedback before merge (PR #610)
- #609 -- DevKit data to CT 202 (closed: Synapset covers it)
- #251 -- Documentation integrity epic (closed: doc-audit CLI sufficient)
- #250, #252 -- Gitea migration (deferred)

### Decisions Made

- Ollama: triage-only now, code-gen prompt template deferred to future issue
- DevKit data: no sync needed, Synapset is the knowledge transport
- Doc integrity: doc-audit CLI is sufficient for current scale
- Gitea migration: dual-forge stable, defer until needed

### Prior Session (2026-03-17, session 1)

6 PRs merged: #595-#599, #604. 9 issues closed. Key fix: MCP Custom Connector.

## Start Here

1. Read this file
2. Check open issues if relevant
3. Proceed -- do not ask the user to explain project state
