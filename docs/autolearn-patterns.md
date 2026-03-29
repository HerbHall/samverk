---
title: Autolearn Patterns
description: Systematic workflows and processes that have proven effective
entry_count: 1
last_updated: 2026-03-28
---

## Autolearn Patterns

This document captures systematic workflows, process improvements, and best practices discovered through operational experience and refined through validation.

## Pipeline Triage Workflow for Queue Health

**Category:** workflow-pattern

**Added:** 2026-03-28 | **Status:** active

**Context:** During a single triage session, the queue went from 0 queued issues to 18 using a systematic diagnostic and remediation approach. This workflow proved effective for identifying blocked issues, unblocking dependency chains, and categorizing work.

**Workflow:**

1. Get failure summary for looping issues (`get_failure_summary`)
2. Cross-reference with recent fixes to identify resolvable patterns
3. Check for dual-label conflicts (e.g., `status:needs-human` + `status:queued`)
4. Reset failure counts on issues eligible for retry
5. Close stale non-mergeable PRs to reduce noise
6. Categorize `needs-human` issues: genuinely-blocked vs automatable
7. Unblock satisfied dependency chains and re-queue resolved issues

**Results:** Re-queued 14 issues from needs-human, closed 7 stale PRs, filed 3 systemic issues (#414, #415, #416) for infrastructure improvements.

**See also:** Known-Gotchas — PR watcher location, non-mergeable PR invisibility, dual status label race conditions.
