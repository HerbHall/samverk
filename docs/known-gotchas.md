---
title: Known Gotchas
description: Production issues and debugging traps in Samverk
entry_count: 3
last_updated: 2026-03-28
---

## Known Gotchas

This document catalogs production issues, debugging traps, and environmental quirks that have caused confusion or misdiagnosis during development.

## PR Watcher Runs in Dispatch Process, Not Serve

**Platform:** Samverk (Go)

**Issue:** The PR watcher goroutine runs in `samverk-dispatch`, not `samverk-serve`. The "MCP wiring gap: prManager" warning logged by `samverk-serve` is cosmetic — it affects MCP tools (create_pr, merge_pr) but not the watcher. Commonly misdiagnosed as "watcher not running" during triage.

**Fix:** Check `samverk-dispatch` logs for `poll_summary`, not `samverk-serve`. The serve process only needs prManager for MCP tool calls.

**Added:** 2026-03-28

---

## Non-Mergeable PRs Invisible to Prwatcher Remediation Loop

**Platform:** Samverk (Go)

**Issue:** `isEligible()` checks `pr.Mergeable` first. PRs with merge conflicts (`mergeable=false`) never reach the CI remediation path. This caused 4 of 6 open PRs to sit indefinitely with no action.

**Fix:** See #415 — add second pass for non-mergeable stale PRs after the eligible PR loop to ensure remediation attempts.

**Added:** 2026-03-28

---

## Dual Status Labels from Race Conditions in Label Operations

**Platform:** Samverk / Gitea

**Issue:** Issues accumulate both `status:needs-human` AND `status:queued` from external label operations (API calls, prwatcher escalation, concurrent label adds). The dispatcher guards against dispatching these but never cleans up the conflicting label. Found 8 issues in this state during triage.

**Fix:** See #416 — add defensive cleanup in pollQueued. Remove conflicting labels when both are detected.

**Added:** 2026-03-28
