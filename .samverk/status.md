---
phase: agent-autonomy
updated: 2026-03-18T20:10:00Z
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
VM 300 runs qwen2.5-coder:32b (not 14b -- confirmed via /api/tags). 600s triage timeout acceptable; claude-haiku fallback works.

## Open Issues

3 issues in samverk/devkit (E2E test issues):

- #2 `status:needs-qc` -- docs audit (ran to completion, quality gate: output too short)
- #3 `status:needs-qc` -- validate-issue-schema.sh (completed prior session)
- #4 (no status) -- forge-wrappers tests (escalated after 4 timeout attempts; needs `status:needs-human`)

## Gaps to Full Autonomy

### Resolved This Session

1. **claude-cli startup timeout too short** -- FIXED: Increased 30s → 120s (PR #672, merged)
   - Root cause: `claude --print` buffers ALL output until session ends; 30s was insufficient
   - Confirmed via strace: process was making active Anthropic API TLS calls within window
   - Validated: issue #2 responded in 78s (would have failed at 30s)

2. **handleLabeled no-op causes missed re-queues** -- KNOWN GAP (not fixed this session)
   - If `status:queued` is manually re-added, dispatcher doesn't pick it up
   - Workaround: restart `samverk-dispatch.service` after manual label changes

### Medium (remaining)

1. Synapset parse error (Synapset#62 filed) -- all memory lookups fail with `invalid character 'F'`
2. DevKit dashboard native React (replace iframe)
3. handleLabeled is no-op -- re-queued issues require service restart to pick up
4. Issue #4 escalated with no `status:needs-human` label (correction sets no label on escalate path)
5. Quality gate "output too short" fires on triage-routed tasks (claude-haiku produces brief output for complex requests -- correct behavior, agent just needs to be re-run with higher complexity routing)

## Recommended Next Session

1. Close devkit issues #2, #3, #4 or re-route with `complexity:cloud` to claude-sonnet
2. Fix handleLabeled to re-route when `status:queued` is added (removes need for restart)
3. Fix escalation path to set `status:needs-human` label on Gitea
4. Synapset#62 parse error (affects all agent memory enrichment)

## Session Summary (2026-03-18, session 4)

E2E dispatcher test + root cause fix for claude-cli startup timeout.

### Root Cause Found and Fixed

**claude-cli `--print` mode buffers all output** until the entire agentic session completes.
The old 30s `startupTimeout` was killing legitimate processes that were actively making
Anthropic API calls but hadn't yet written any bytes. Confirmed via strace on CT 202.

Fix: `startupTimeout` 30s → 120s (`internal/provider/claudecli/claudecli.go`).
Validated: issue #2 (docs) responded in 78s -- would have been killed at 30s.

### PRs Merged

- #666 -- Asset hash change (index.html, carry-over from prior session)
- #672 -- claude-cli startup timeout 30s → 120s (E2E test validation)

### Dispatcher E2E Test Results

Created 3 issues in Gitea `samverk/devkit` repo:

- #2 (docs): claude-haiku completed in 78s, `status:needs-qc` (quality: output too short)
- #3 (code-gen): completed prior session, `status:needs-qc`
- #4 (test): timed out after 4 attempts, escalated (no human label set -- gap)

### Secondary Findings

- `handleLabeled` is a Phase 1 stub (no-op): manually re-adding `status:queued` requires service restart
- `correction.escalate` path does not set any Gitea label
- VM 300 actually runs `qwen2.5-coder:32b` (not 14b); 600s Ollama triage timeout is fine
- Concurrent claude-cli processes (2 simultaneous) may interfere -- issue #4 timed out while #2 succeeded

### Prior Session (2026-03-17, session 3)

Housekeeping and bug fix session.

#### PRs Merged

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
