# Samverk Sprint Plan

**Generated:** 2026-03-19
**Phase:** Agent Autonomy — MCP capability expansion + quality foundation

---

## Backlog Overview

| Wave | Theme | Issues | Effort |
|------|-------|--------|--------|
| 0 | Rebase stalled PRs | #53, #54 | Chore |
| 1 | QC queue — merge-ready fixes | #78, #79, #80, #81, #83 | Low |
| 2 | Quality foundation | #84, #85, #86 | Low |
| 3 | MCP capability (high-priority) | #55, #60, #64, #65, #66, #67 | High |
| 4 | Agent tooling (normal) | #56, #62, #68, #69 | Medium |
| 5 | Strategic (agent:human) | #57, #58, #70, #72, #74 | High |

---

## Wave 0 — Rebase Stalled PRs

These PRs are complete but drifted behind main. Rebase and merge before
any other code work to avoid conflict debt.

| Issue | PR | Title |
|-------|----|-------|
| #54 | PR #40 | feat: phase-aware routing and set_project_phase MCP tool (#35, #36) |
| #53 | PR #41 | docs: revise ADR-031 to single-forge-per-project model |

**Action:** `git rebase origin/main` on each branch, force-push, merge.

---

## Wave 1 — QC Queue (Ready to Merge)

All `status:needs-qc`. Review, verify CI, merge sequentially.

| # | Title | Type |
|---|-------|------|
| #78 | fix(dispatcher): handleLabeled stub ignores status:queued re-add | bug |
| #79 | fix(dispatcher): correction.escalate must set status:needs-human label | bug |
| #80 | fix(provider): claude-cli startupTimeout too short for complex issues | fix |
| #81 | chore(infra): Ollama loses GPU after extended uptime — health check + restart | infra |
| #83 | fix(dispatcher): stale Gitea watcher does not auto-reconnect | bug |

---

## Wave 2 — Quality Foundation

Small, focused. Closes gaps from the 2026-03-19 session audit.

| # | Title | Why Now |
|---|-------|---------|
| #84 | test(server): session persistence integration test | Closes regression gap — no restart cycle test exists |
| #86 | chore(ci): enforce golangci-lint v2 via go run | Prevents false-clean local lint results |
| #85 | docs(auth): session lifecycle contract | Documents two-tier auth model before it grows more complex |

---

## Wave 3 — MCP Capability (High Priority)

Core capability expansion. Enables agents to do more without human relay.
Run as parallel worktree agents where files don't overlap.

| # | Title | Impact |
|---|-------|--------|
| #64 | fix: get_diff returns empty content — body, pagination, path filter | Agents can't read diffs without this |
| #65 | fix: provider failover not triggered on Ollama timeout | Silent failures when GPU hosts are slow |
| #55 | fix: add agent:pc to autonomy gate skip list and overlay labels | PC agent can't act without this |
| #60 | feat: add write_file and create_branch MCP tools | Enables repo writes from any session |
| #67 | feat: agent observability — list_workers, get_session_log, get_provider_health | Visibility into running agents |
| #66 | feat: multi-project MCP architecture — project param, groups, aggregate queries | Scale beyond single-project scope |

**Parallelizable groups:**

- Group A (dispatcher/provider): #64, #65 (separate packages)
- Group B (MCP tools): #60, #67 (additive to tools.go — assign one agent to shared file)
- Group C (architecture): #66 (touches routing, do after A+B merge)

---

## Wave 4 — Agent Tooling (Normal Priority)

| # | Title | Notes |
|---|-------|-------|
| #56 | feat: pc-agent-task skill and get-pc-task.ps1 handoff workflow | Requires #55 first |
| #62 | feat: Claude Code CLI with Ollama backend for dispatcher inference | Free inference path |
| #68 | feat: search_issues and search_prs MCP tools with cross-project scope | Quality-of-life |
| #69 | feat: bulk forge operations and project summary tool | Quality-of-life |

---

## Wave 5 — Strategic (agent:human decisions required)

These need user input before implementation can begin.

| # | Title | Decision Needed |
|---|-------|----------------|
| #57 | research: promote qwen3-coder:30b (HDH-NZXT) to code-gen routing | Approve after quality validation |
| #58 | feat: nightly infrastructure probe for Synapset machines pool | Approve design |
| #70 | feat: MCP/dashboard parity — mobile Claude.ai as supervisory interface | Scope approval |
| #72 | feat: full real-time dashboard — WebSocket, expanded API, chat panel | Major scope — approve before starting |
| #74 | feat: homelab security overhaul — SSH keys, Cloudflare OAuth, credential keyring | Security-sensitive — explicit approval |

---

## Close Candidates

| # | Title | Reason |
|---|-------|--------|
| #73 | fix: implement dashboard and MCP authentication | Core deliverable (login wall + bearer auth) shipped in PR #664. Cloudflare Access portion is separate scope — split to new issue if still desired. |

---

## Dependency Order

```text
Wave 0 (rebase) → Wave 1 (QC) → Wave 2 (quality)
                                         ↓
                               Wave 3 Group A+B (parallel)
                                         ↓
                               Wave 3 Group C (#66)
                                         ↓
                               Wave 4 (#56 after #55, rest parallel)
```

Wave 5 items can start anytime user provides direction.

---

## Recommended Starting Point

1. **Now:** Close #73 (done), rebase PR #41 and PR #40 (Wave 0)
2. **Next:** Merge Wave 1 QC queue (5 quick merges)
3. **Then:** Wave 2 (session test + lint enforcement + auth docs)
4. **Sprint:** Wave 3 parallel agents for MCP capability
