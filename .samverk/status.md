---
phase: agent-autonomy
updated: 2026-03-17T00:30:00Z
updated_by: claude-code
---

# Samverk -- Current State

## Phase

Agent Autonomy -- getting Samverk to run itself. Infrastructure complete.
Focus: fix agent quality, move data to server, enable planning workflow.

## What Is Running

- Samverk server: CT 202 (192.168.1.162:8080) -- healthy
- Dispatcher: RUNNING (1-5 workers, 6 providers, free-first routing)
- Health monitor: 60s probes, WoL for sleeping hosts
- Watcher: auto-restart with backoff (no more silent hangs)
- Dashboard: unified with Synapset native + DevKit iframe
- Gitea CI: CT 200 (40GB disk, weekly cleanup cron)

### GPU Fleet

| Host | GPU | Model | Status |
|------|-----|-------|--------|
| HDH-NZXT | RTX 5090 32GB | qwen3-coder:30b | HEALTHY |
| VM 300 | RTX 3090 Ti 24GB | qwen2.5-coder:14b | HEALTHY |
| CM-ASUS | RTX 2080 Ti 11GB | qwen2.5-coder:7b | HEALTHY |

## Gaps to Full Autonomy

### Critical

1. **Ollama output quality** -- models overwrite CLAUDE.md instead of
   implementing features. No tool use, raw chat format. DevKit#348.
2. **Claude CLI hangs** -- 60s timeout on CT 202, switch_provider works
   but wastes time.

### High

1. **No planning step** -- agents code without reading codebase first.
2. **DevKit data on local machine** -- claude.db not on CT 202.
3. **Copilot review feedback** -- PRs merge without reading comments.

### Medium

1. Synapset parse error (Synapset#62 filed)
2. DevKit dashboard native React (replace iframe)
3. Enhanced Agents page (#590)
4. Multi-repo dispatch (code ready, config needed)

## Recommended Next Session

1. Plan issues for gaps #1-5 with precise scope
2. Queue for agents with detailed prompts (not auto-dispatch)
3. Monitor and fix as they process

## Session Summary (2026-03-16/17)

13 PRs merged, 20+ issues closed. Multi-repo dispatcher, health probes,
WoL, 3 GPU hosts, unified dashboard, inline sparklines, clickable links,
watcher auto-restart, CT 200 disk fix.

## Start Here

1. Read this file
2. Check open issues if relevant
3. Proceed -- do not ask the user to explain project state
