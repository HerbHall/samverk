---
phase: execution
updated: 2026-03-08T22:00:00Z
updated_by: claude-code
---

# Samverk -- Current State

## Phase

Phase 5 complete: agent runtime, provider integration, SPA embedding, PR watcher.
Q2 2026 execution plan: 62 issues across 3 streams. Windows 1-2 agent work in progress.

## What Is Running

- Samverk server: CT 202 (192.168.1.162:8080) -- healthy
- MCP endpoint: POST /mcp (Streamable HTTP, auth required)
- Dispatcher: running continuously (systemd, 30s poll, 3 workers)
- PR watcher: runs concurrently with dispatcher (auto-merge eligible PRs)
- Gitea: CT 200 (192.168.1.160:3000 / gitea.herbhall.net) -- primary runtime forge

## Dual-Forge Status

| Item | Status |
|------|--------|
| Gitea instance (CT 200) | Running -- 1.23.7 |
| samverk/samverk repo | Created on Gitea |
| Gitea adapter (IssueTracker + Repo + PR) | Merged (PRs #319, #322) |
| Integration tests against Gitea | Merged (PR #322) |
| Gitea Actions research | Merged (PR #321) |
| Gitea CI workflow (.gitea/workflows/) | PR #331 (auto-merge) |
| Gitea security.yml (govulncheck + trivy) | PR #331 (auto-merge) |
| dual-forge server.yaml config | PR #326 (auto-merge) |
| create-issues.sh --forge gitea | PR #327 (auto-merge) |
| migrate-issues.py (GitHub -> Gitea) | PR #330 (auto-merge) |
| ADR-031 dual-forge model | This session |
| Gitea Actions runner (CT 201) | Pending (B14, human setup) |
| Bidirectional git push remote | Pending (B24, human task) |
| MCP tools for Gitea project switching | Pending (#277) |

## Windows 1-2 Progress (B-track)

Completed B-track:

- B01-B09 (#256-#264): Gitea adapter implementation (PRs #316-#319, merged)
- B10 (#265): PR manager implementation (PR #319, merged)
- B13 (#268): Gitea Actions compatibility research (PR #321, merged)
- B03/B08/B10 (#258,#263,#265): Integration tests (PR #322, merged)
- B17 (#272): dual-forge server.yaml config (PR #326, auto-merge)
- B18 (#273): create-issues.sh --forge gitea (PR #327, auto-merge)
- B20 (#275): issue migration script (PR #330, auto-merge)
- B14 (#269): gitea-ci.yml (PR #331, auto-merge)
- B15/B16 (#270/271): security.yml (PR #331, auto-merge)
- B25 (#280): ADR-031 dual-forge model (this session)
- B26 (#281): CLAUDE.md + status.md update (this session)

Completed W-track (metrics/scaling):

- W01 (#284): PoolMetrics ring buffer (merged, PR #323)
- W02 (#285): DispatcherMetrics ring buffer + P95 (merged, PR #323)
- W03 (#286): SystemCollector (merged, PR #323)
- W04 (#287): Metrics load tests (PR #325, auto-merge)

Human tasks remaining:

- B11 (#266): Already done (Gitea repo created)
- B12 (#267): Verify Gitea label + webhook health
- B14 runner (#269): Provision CT 201 as Gitea Actions runner
- B24 (#279): Configure bidirectional git push remote
- B21-B22: Tag mirroring (deferred to after B24)

## Next Agent Tasks

W-track (ready, no blockers):

- W05 (#288): Add /api/v1/metrics REST endpoint
- W06 (#289): Add metrics dashboard page to React SPA
- W07 (#290): Add metrics to MCP get_digest
- W08 (#292): Refactor agent pool to support dynamic add/remove

B-track (ready after PRs merge):

- B21 (#276): Tag mirroring GitHub -> Gitea
- B22 (#277): MCP tools for Gitea project context switching
- B23 (#278): Verify MCP tools vs Gitea (human verification)

## Queued (pre-existing)

- `#152`: dispatcher routes to Copilot as provider
- `#153`: dispatch feedback loop (depends on #144 research)
- `#157`: Claude Code Remote Control spike (human task)
- `#186`: `samverk status --write` CLI automation
- `#324`: fix panic on send-to-closed-channel in pool.Submit

## Start Here (Cold Start Protocol)

1. Read this file
2. Call `samverk get_digest --since 168h` if MCP is configured
3. Read open issues if relevant to the task
4. Proceed -- do not ask the user to explain project state
