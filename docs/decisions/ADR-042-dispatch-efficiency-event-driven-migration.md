# ADR-042: Dispatch Efficiency and Event-Driven Migration Path

## Status

Proposed

## Context

Issue #516 exposed a structural problem: the dispatch pipeline scans Gitea
issue comments every 30-60 seconds to derive boolean state (QC passed? quality
warning posted? CI failed?). At 80+ issues with 50+ comments each, this
allocates multi-GB per cycle and caused repeated OOM kills on CT 202 (20 GB
RAM, scaling-max reduced to 1).

Band-aid fixes (PR #517 cloneString, PR #520 disabled checkpoint scanning,
PR #522 SQLite checkpoints with 4KB body truncation) reduced the immediate
pressure but pprof still shows 7+ GB from `prwatcher.hasQCApproval` and
`dispatcher.tryApplyEdits` -- both call `ListComments` in polling loops.

The deeper issue is that the pipeline conflates two data needs:

1. **Decision surface**: compact boolean/enum state for routing (is QC passed?
   how many CI failures?). Read every tick by machines.
2. **Audit surface**: full text comments for human review and AI triage. Read
   occasionally by humans and triage AI.

Currently, the decision surface is derived by scanning the audit surface every
cycle. This does not scale.

Additionally, four subsystems (Watch, issue_sync, pollQueued, prwatcher)
independently call `ListIssues` against the forge, producing O(N) API calls
per tick where N = total open issues. At 500 issues, this consumes ~37 API
calls/minute; at 1000 issues, ~63 calls/minute.

### Options Considered

**Option A: Fix ListComments only (labels as markers)**

Replace comment scanning with label checks for boolean markers. Labels are
already cached in `issue_cache`. Zero API calls, zero allocations for marker
reads.

Pros: Minimal change, fixes the immediate OOM, labels are human-visible.
Cons: Does not address O(N) polling, does not establish a path to event-driven.

**Option B: Full event-driven rewrite**

Implement webhooks, internal event bus, and encoded state register all at once.
Eliminate all polling.

Pros: Architecturally clean, scales indefinitely.
Cons: Weeks of work while pipeline is down. Over-engineered for current scale.
Risk of introducing new bugs in a system that needs stability now.

**Option C: Phased migration (Selected)**

Fix the OOM immediately with labels (Option A), then incrementally optimize
polling and build toward event-driven architecture. Each phase delivers
standalone value and does not require later phases to justify the work.

Phase 1: Labels as markers (eliminate ListComments from hot paths)
Phase 2: Incremental sync (O(N) -> O(delta), consolidate polling loops)
Phase 3: Internal event log + encoded state register
Phase 4: Webhook support (future, when polling is insufficient)

## Decision

Adopt Option C: phased migration from polling-with-text-scanning to
event-driven-with-encoded-state.

### Key principles

1. **Decision surface and audit surface are separate concerns.** Comments are
   for humans; labels/SQLite are for machines. Hot paths never read comments.

2. **Encode, don't scan.** Routing decisions should read compact indexed state,
   not parse text. The target is a "VIN number" model: each issue's decision
   state fits in ~64 bytes.

3. **Labels are the input; computed state is the cache.** Labels remain the
   write surface (human-visible, forge-native). The `issue_state` register
   (Phase 3) is derived from labels, not an independent truth.

4. **Forge remains authoritative** (per ADR-027). SQLite caches and internal
   event logs are always rebuildable from the forge.

5. **Each phase is independently valuable.** Phase 1 fixes OOM. Phase 2 reduces
   API calls 7x. Phase 3 enables event-driven patterns. Phase 4 adds
   sub-second latency. No phase depends on a later phase for its value.

6. **Review before implement.** Each wave gets a `/plan-review` gate before
   code is written. Quick fixes must conform to the long-term architecture.

### Phase 1 specifics (immediate)

- Add `qc:pass`, `qc:fail`, `qc:review`, `warning:quality`, `warning:doc-gate`
  labels
- Dual-write: comment (audit) + label (decision) at every write site
- Rewrite all hot-path reads to check labels via issue_cache
- CI failure counts read from existing `issue_failure_counts` table
- EDIT blocks read from `session.partial_output` (already done in PR #522)
- Batch label API: forge interface accepts `[]string` for multi-label ops

### Phase 2 specifics (next sprint)

- Incremental sync using `?since=` parameter on ListIssues
- Merge Watch + issue_sync into single sync goroutine
- PR watcher reads linked issue labels from issue_cache
- Configurable polling intervals per subsystem

### Phase 3 specifics (following sprint)

- `issue_events` table for state transition logging
- In-process EventBus (pub/sub) for inter-subsystem communication
- `issue_state` table as compact computed cache
- Dispatcher and prwatcher subscribe to events instead of polling

### Phase 4 specifics (future)

- Gitea webhook integration
- Polling demoted to reconciliation-only (15 min)

## Consequences

### Positive

- Immediate OOM fix (Phase 1): RSS drops from 7+ GB to < 500 MB
- 7x reduction in API calls (Phase 2): from ~37/min to ~5/min at 500 issues
- Clear separation of decision/audit surfaces prevents future conflation
- Event log (Phase 3) provides debugging and replay capability
- Encoded state register makes routing decisions O(1) instead of O(comments)
- Each phase is a natural stopping point -- no "half-done" intermediate states

### Negative

- Dual-write (comment + label) adds a second write per state change in Phase 1
- Labels are Gitea-specific coupling (GitHub labels work differently at scale)
- Event log + state register add SQLite tables and sync complexity
- The forge abstraction must accommodate label batching differences

### Neutral

- ADR-027's hybrid approach (forge as truth, SQLite as cache) is reinforced
  and extended, not replaced
- Polling intervals become configurable, shifting complexity from code to config
- The Watch loop (forge-level event detection) is subsumed by the sync
  goroutine in Phase 2 -- one fewer goroutine but same logical function

## References

- [DES-001: Dispatch Efficiency Design](../dispatch-efficiency-design.md) -- full design and implementation plan
- [ADR-027: Failure Recovery](ADR-027-failure-recovery.md) -- forge as source of truth
- [ADR-013: Forge Abstraction](ADR-013-forge-abstraction.md) -- platform-agnostic interface
- [ADR-012: Git Issues Protocol](ADR-012-git-issues-protocol.md) -- issue schema
- [ADR-039: Two-Location Rule](ADR-039-two-location-centralization-rule.md) -- single source of truth
- Issue #516: Dispatch memory leak
- PRs #517, #520, #522: Band-aid fixes
