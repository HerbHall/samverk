# DES-001: Dispatch Efficiency and Event-Driven Migration

- **Status**: Draft
- **Date**: 2026-03-30
- **Author**: Herb Hall + Claude
- **Related**: ADR-027 (failure recovery), ADR-032 (adaptive scaling), ADR-013 (forge abstraction), ADR-012 (git issues protocol), ADR-039 (two-location rule)

## Problem Statement

The dispatch pipeline OOMs at 80+ issues because subsystems independently poll
the forge for full data sets (comments, issues, PRs) every 30-60 seconds. Each
subsystem fetches what it needs in isolation, producing O(N) API calls and
multi-GB allocations per cycle. The architecture treats text-based comment
bodies as a queryable data store when the pipeline only needs boolean/numeric
state for routing decisions.

This is not a single bug -- it is a structural mismatch between how the
pipeline consumes state (text scanning) and how it should consume state
(encoded signals). Fixing the immediate OOM without addressing the structure
guarantees we hit the next wall at 200-500 issues.

## Goals

- Eliminate OOM from dispatch hot paths (immediate: pipeline is down)
- Reduce per-cycle API calls from O(N) to O(delta) for steady-state operation
- Separate the **decision surface** (compact, encoded, machine-readable) from
  the **audit surface** (text, human-readable, append-only)
- Establish a foundation that supports event-driven architecture without
  requiring a rewrite
- Scale to 1000+ issues across multiple projects with sub-100 API calls/minute

## Non-Goals

- Replacing Gitea/GitHub as the source of truth (forge remains authoritative)
- Building a real-time event bus or message queue (overkill for solo/small team)
- Changing the agent execution model (container/CLI agents stay as-is)
- Multi-instance samverk (Watchkit is a separate future project)

## Core Concept: Decision Surface vs Audit Surface

The pipeline currently conflates two fundamentally different data needs:

### Audit Surface (text, human-readable)

- Issue comments: QC reports, agent output, failure logs, triage evaluations
- PR review comments: code suggestions, blocking feedback
- Issue body: requirements, acceptance criteria, context

**Consumers**: Humans reading Gitea UI, triage AI needing full context, MCP
`list_comments` tool for interactive queries.

**Access pattern**: Read occasionally, append-only, unbounded size.

### Decision Surface (encoded, machine-readable)

- Is QC passed? (boolean)
- How many CI failures? (counter)
- Is quality warning posted? (boolean)
- What status is this issue? (enum)
- What agent type? (enum)
- Is this blocked? On what? (reference set)

**Consumers**: Dispatcher routing loop, PR watcher merge logic, scaling policy.

**Access pattern**: Read every tick (30-60s), mutated on state transitions,
bounded size.

### The Mismatch

Today, the decision surface is derived by scanning the audit surface. The
dispatcher reads 50+ comments per issue to answer a yes/no question. This is
analogous to reading an entire car's service history to check if the oil was
changed, when the answer could be a single flag on a dashboard.

### The Fix: Encoded State Register

Every piece of information the pipeline needs for routing decisions should exist
in a compact, indexed, machine-readable form. Text comments remain for audit
but are never read in hot paths.

**Encoding model**: Like a VIN (Vehicle Identification Number), the issue's
decision state packs into a small structured record. Each field is typed and
bounded:

```text
Issue #142 Decision State:
  status:     queued (enum, 3 bits)
  qc:         pass   (enum, 2 bits)
  ci_fails:   0      (uint8)
  warnings:   0x03   (bitfield: quality=1, doc-gate=1)
  agent_type: claude (enum, 4 bits)
  priority:   medium (enum, 2 bits)
  blocked_by: []     (int array, typically 0-3 entries)
  claimed_by: ""     (string, worker ID or empty)
  last_heart: 1711792800 (unix timestamp)
```

Total: ~64 bytes per issue. At 10,000 issues: 640 KB. Compare to the current
approach: 10,000 issues x 50 comments x 4KB = 2 GB.

**Storage options** (evaluated in Phase 3):

| Option | Pros | Cons |
|--------|------|------|
| Gitea labels (current plan) | Zero API calls (cached), human-visible | Limited to string presence checks, no counters |
| SQLite `issue_state` table | Full query support, typed columns, indexes | Second source of truth, sync required |
| Bitfield in issue_cache | Single row per issue, minimal storage | Opaque, hard to debug |
| Hybrid: labels for booleans + SQLite for counters | Best of both, matches current infrastructure | Two read paths |

**Phase 1 uses labels + existing SQLite tables** (hybrid). Phase 3 evaluates
whether to consolidate into a single `issue_state` table.

## Proposed Design

### Architecture Layers

```text
┌─────────────────────────────────────────────────────┐
│                    AUDIT SURFACE                     │
│  Issue comments, PR reviews, agent logs              │
│  Written on events, read by humans + triage AI       │
│  Append-only, unbounded, text                        │
└──────────────────────┬──────────────────────────────┘
                       │ write-only (never read in hot paths)
                       │
┌──────────────────────▼──────────────────────────────┐
│                  DECISION SURFACE                    │
│  Labels (booleans) + SQLite (counters/timestamps)    │
│  + issue_cache (synced state)                        │
│  Read every tick, mutated on transitions, bounded    │
└──────────────┬───────────────────┬──────────────────┘
               │                   │
       ┌───────▼───────┐   ┌──────▼──────┐
       │  Dispatcher    │   │  PR Watcher  │
       │  Router        │   │  Merge Logic │
       │  Scaling       │   │  CI Monitor  │
       └───────┬───────┘   └──────┬──────┘
               │                   │
       ┌───────▼───────────────────▼──────┐
       │         INTERNAL EVENT LOG        │  ← Phase 3
       │  SQLite table: state transitions  │
       │  Subscribers notified on change   │
       └──────────────────────────────────┘
```

### Phase 1: Emergency Fix -- Labels as Markers (pipeline is down)

**Goal**: Eliminate ListComments from all polling hot paths. Get pipeline
running again.

**Approach**: Dual-write (comment + label) at write sites. Replace all
comment-scanning reads with label checks against issue_cache.

**Changes** (from the existing rustling-riding-sunset plan):

| Write site | Add label |
|------------|-----------|
| qc_handler.go: handleQCPass | `qc:pass` |
| qc_handler.go: handleQCFail | `qc:fail` |
| qc_handler.go: handleQCReview | `qc:review` |
| router.go: postQualityWarningIfNeeded | `warning:quality` |
| runner.go: checkDocGate | `warning:doc-gate` |

| Read site (eliminate ListComments) | New read method |
|------------------------------------|-----------------|
| router.go: quality warning check | Check `issue.Labels` for `warning:quality` |
| watcher.go: hasQCApproval | Check issue_cache labels for `qc:pass` |
| watcher.go: QC on linked issues | Check issue_cache labels for `qc:pass` |
| watcher.go: CI failure count | Query `issue_failure_counts` table |
| apply_edits.go: EDIT blocks | Query `session.partial_output` (already done in PR #522) |
| apply_edits.go: backfill | Same session query |
| qc_handler.go: QC fallback | Remove comment fallback; escalate if no session output |

**Error handling**: Label writes are fire-and-forget for informational labels
(`warning:*`). QC labels (`qc:*`) log on failure but do not block the
dispatch loop. The comment is the fallback audit record.

**Batch label support**: Modify `AddLabel` in forge interface to accept
`[]string` and use Gitea's batch label API. Reduces API calls when adding
multiple labels in one handler.

**Verification**: Deploy to CT 202, monitor pprof on :6060 for 30 min. Target:
zero ListComments in top allocators, RSS under 500 MB steady state.

### Phase 2: Polling Optimization -- Incremental Sync

**Goal**: Reduce API calls from O(N) to O(delta) per sync cycle.

**Approach**: Use `?since=<timestamp>` parameter on ListIssues to fetch only
issues updated since last sync. Both Gitea and GitHub support this.

**Changes**:

1. **Track last-sync timestamp per tracker** in SQLite
   (`dispatcher_state.last_sync_at`)

2. **Incremental issue_sync**: Fetch only issues with `updated_at > last_sync`.
   Full sync demoted to reconciliation interval (every 15 min per ADR-027).

3. **Consolidate Watch + issue_sync**: The Watch loop and issue_sync both call
   `ListIssues` independently. Merge into a single sync goroutine that:
   - Fetches incremental updates every 30-60s
   - Runs full reconciliation every 15 min
   - Emits change events to internal subscribers

4. **PR watcher: use issue_cache for linked issue labels** instead of live API
   calls. The only per-PR API call that remains is `GetPRChecks()` (CI status
   cannot be cached).

5. **Tunable polling intervals**: Make intervals configurable per subsystem in
   `server.yaml`. Defaults:
   - Issue sync: 60s incremental, 15m full
   - PR watcher: 60s (up from 30s -- PRs don't change status that fast)
   - Heartbeat check: 60s (unchanged)

**API call budget after Phase 2** (steady state, 500 issues, ~5 updates/min):

| Subsystem | Before | After |
|-----------|--------|-------|
| Watch loop | 20 calls/min | 0 (merged into sync) |
| Issue sync | 10 calls/min | 1-2 calls/min (incremental) |
| PR watcher | 6 calls/min | 2 calls/min (checks only) |
| Heartbeat | 1 call/min | 1 call/min |
| **Total** | **~37 calls/min** | **~5 calls/min** |

### Phase 3: Internal Event Log and State Register

**Goal**: Eliminate inter-subsystem polling. Each subsystem reacts to state
changes instead of re-scanning the world.

**Approach**: Add an `issue_events` table that records state transitions. A
lightweight pub/sub within the Go process notifies subscribers.

**Data model**:

```sql
CREATE TABLE issue_events (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  project    TEXT NOT NULL,
  number     INTEGER NOT NULL,
  event_type TEXT NOT NULL,    -- 'label_added', 'label_removed', 'status_changed', 'qc_verdict', 'ci_result'
  key        TEXT NOT NULL,    -- the label name or field that changed
  value      TEXT,             -- new value (nullable for removals)
  old_value  TEXT,             -- previous value (nullable)
  source     TEXT NOT NULL,    -- 'sync', 'dispatcher', 'prwatcher', 'agent'
  created_at TEXT NOT NULL,
  INDEX idx_events_project_number (project, number),
  INDEX idx_events_type_created (event_type, created_at)
);
```

**Internal subscriber model**:

```go
type EventBus struct {
    subscribers map[string][]chan Event  // event_type -> channels
}

func (b *EventBus) Subscribe(eventType string) <-chan Event
func (b *EventBus) Publish(event Event)
```

**Issue state register** (evaluated at this phase):

Consider normalizing the decision surface into a single `issue_state` table:

```sql
CREATE TABLE issue_state (
  project       TEXT NOT NULL,
  number        INTEGER NOT NULL,
  status        INTEGER NOT NULL DEFAULT 0,  -- enum: 0=queued, 1=claimed, ...
  qc_verdict    INTEGER NOT NULL DEFAULT 0,  -- enum: 0=none, 1=pass, 2=fail, 3=review
  ci_fail_count INTEGER NOT NULL DEFAULT 0,
  warnings      INTEGER NOT NULL DEFAULT 0,  -- bitfield
  agent_type    INTEGER NOT NULL DEFAULT 0,  -- enum
  priority      INTEGER NOT NULL DEFAULT 0,  -- enum
  claimed_by    TEXT NOT NULL DEFAULT '',
  last_heartbeat INTEGER NOT NULL DEFAULT 0, -- unix timestamp
  updated_at    TEXT NOT NULL,
  PRIMARY KEY (project, number)
);
```

This is the "VIN number" concept: every routing decision reads a single compact
row instead of scanning labels, comments, and multiple tables. The enum/bitfield
encoding makes comparisons trivially fast and storage minimal.

**Trade-off to evaluate**: Labels are human-visible in Gitea UI. A pure
`issue_state` table is not. The recommendation is to **keep labels as the
write surface** (human visibility preserved) and **derive issue_state from
label sync** (machine reads the compact form). Labels become the input;
issue_state is the computed cache.

### Phase 4: Webhook Support (Future -- Research Track)

**Goal**: Sub-second event processing. Polling becomes reconciliation-only.

**Approach**: Register Gitea webhooks for issue and PR events. Events flow
directly into the event log (Phase 3). Polling demoted to reconciliation
(every 15 min).

**Research items**:

- [ ] Gitea webhook delivery guarantees (at-least-once? retry policy?)
- [ ] Webhook signature verification for Gitea
- [ ] Event deduplication (webhook + polling reconciliation may produce duplicates)
- [ ] Webhook endpoint security (samverk is behind Cloudflare tunnel)
- [ ] Failure mode: what happens when webhook delivery fails for extended periods?
- [ ] Can Gitea webhooks be configured per-org or only per-repo?

**Not needed now**. The incremental sync from Phase 2 is sufficient for
1000+ issues. Webhooks become valuable at 5000+ or when sub-second latency
matters (e.g., interactive dashboard updates).

## Encoded State Representation -- Research Direction

The "VIN number" concept deserves deeper investigation. The core insight is
that the pipeline's routing decisions are based on a small number of discrete
signals, but those signals are currently scattered across text blobs.

### Current State Encoding

```text
Issue #142:
  To check QC status:     Fetch 50 comments, regex scan for "**QC Review: [PASS]**"
  To check CI failures:   Fetch 50 comments, count "CI FAILED" matches
  To check quality warn:  Fetch 50 comments, scan for "[dispatcher] Issue quality warning"
  To check blocked deps:  Parse YAML frontmatter, call ListIssues per dependency
```

### Target State Encoding (Phase 3)

```text
Issue #142:
  SELECT status, qc_verdict, ci_fail_count, warnings FROM issue_state
  WHERE project = 'samverk' AND number = 142
  → (1, 1, 0, 3)  -- claimed, qc:pass, 0 failures, quality+docgate warnings
  → Single row, ~64 bytes, indexed lookup, <1ms
```

### Beyond Samverk: Generalized Signal Encoding

For the broader pipeline concept, consider a signal encoding scheme:

```text
Signal ID format:  <domain>:<category>:<specific>
  qc:verdict:pass
  ci:status:green
  dep:blocked:142,155
  agent:type:claude
  cost:tier:free

Encoded as bitfield or enum map per issue:
  signals = {qc_verdict: PASS, ci_status: GREEN, dep_count: 0, agent: CLAUDE}
```

This maps to the "compact truth table" idea -- each issue's routing decision
becomes a lookup into a fixed-width record, not a text scan. The signal
vocabulary is finite and known at compile time.

**Research questions**:

- [ ] What is the complete signal vocabulary for routing decisions? (audit all
      `if` conditions in router.go, watcher.go, qc_handler.go)
- [ ] Can the signal register replace the `claimed` map entirely?
- [ ] What is the right encoding: SQLite enum columns, Go bitfield struct,
      or protobuf-like compact binary?
- [ ] How does the signal register interact with the forge abstraction?
      (Labels are forge-native; the register is samverk-internal)

## Implementation Plan

### Wave 1: Emergency OOM Fix (immediate -- pipeline is down)

**Scope**: Phase 1 only. Get pipeline running.

1. Add label constants to `labels_gen.go`
2. Create labels in Gitea via overlay
3. Dual-write labels at all write sites
4. Rewrite all hot-path reads to use labels/SQLite
5. Batch label API support in forge interface
6. Remove `qualityChecked` sync.Map
7. Deploy, verify pprof, confirm RSS < 500 MB

**Review gate**: `/plan-review` before implementation.

### Wave 2: Polling Consolidation (next sprint)

**Scope**: Phase 2. Reduce API calls 7x.

1. Add `last_sync_at` tracking to `dispatcher_state`
2. Implement incremental sync with `?since=` parameter
3. Merge Watch + issue_sync into single goroutine
4. PR watcher: read labels from issue_cache
5. Make polling intervals configurable
6. Deploy, measure API calls/min

**Review gate**: Design review of sync consolidation before implementation.

### Wave 3: Event Log Foundation (following sprint)

**Scope**: Phase 3. Internal event-driven communication.

1. Create `issue_events` table
2. Implement EventBus (in-process pub/sub)
3. Sync goroutine publishes events on detected changes
4. Refactor dispatcher + prwatcher to subscribe instead of poll
5. Prototype `issue_state` table as computed cache
6. Evaluate: does `issue_state` replace labels for reads?

**Review gate**: ADR for event log design + `/plan-review`.

### Wave 4: Encoded State Register (research + prototype)

**Scope**: Phase 3 completion. Compact routing state.

1. Audit complete signal vocabulary from routing code
2. Design `issue_state` schema with enum/bitfield encoding
3. Implement sync: labels -> issue_state derivation
4. Refactor routing decisions to read issue_state
5. Benchmark: measure routing decision time before/after

**Review gate**: Design doc for signal encoding + benchmarks.

### Future: Webhook Integration (when needed)

**Scope**: Phase 4. Only when incremental sync is insufficient.

- Research Gitea webhook capabilities
- Implement webhook endpoint with signature verification
- Event deduplication with polling reconciliation
- Demote polling to 15-min reconciliation

## Open Questions

- [ ] Should `issue_state` be a materialized view (auto-derived from labels)
      or an independently-managed table?
- [ ] What is the right reconciliation interval for Phase 2? ADR-027 says 15
      min but this was before incremental sync existed.
- [ ] Should the EventBus be typed (Go generics) or stringly-typed?
- [x] Does the forge interface need a `BatchAddLabels([]string)` method or
      should the existing `AddLabel` accept variadic args?
      **Resolved (Wave 1)**: Variadic `AddLabels(...string)` -- backward-compatible,
      batch-capable, single API call.
- [ ] At what scale does the SQLite issue_state table need WAL mode or
      connection pooling?

## Wave 1 Results (2026-03-30)

**PR #523** merged and deployed to CT 202.

| Metric | Before | Target | Actual |
|--------|--------|--------|--------|
| Dispatch RSS | 7+ GB (OOM) | < 500 MB | 23 MB |
| ListComments in hot paths | 80+ issues x 50+ comments/30s | 0 | 0 |
| Pipeline status | Down (OOM) | Running | Running, pressure: low |

### Lessons learned

1. **Backfill CLI has the same OOM risk**: `backfill-labels` calls
   `ListComments` on all open issues -- the exact pattern Wave 1 eliminates.
   Three concurrent backfill processes consumed 18+ GB. Run backfills from a
   local machine via API, not on CT 202. Add `GOMEMLIMIT` or pagination if
   the CLI must run on the server.

2. **Backfill was unnecessary**: All QC-passed issues were already closed.
   Zero open issues needed `qc:pass` labels. Future backfills should either
   include closed issues or accept that pre-migration issues don't need labels.

3. **gen-labels groupOrder is hardcoded in 3 places**: New label prefixes
   require updating Go, TypeScript, and PowerShell generators. Missing one
   silently omits the group. Consider deriving groupOrder from the groups map.

4. **Base process RSS is ~20 MB**: Scaling estimates should include process
   overhead, not just per-subsystem costs. The "< 1 MB for marker checking"
   estimate was correct for that subsystem but misleading as a total RSS
   target.

### Input to Wave 2

- The `IssueCacheReader` interface and `WithIssueCache` option pattern
  established in Wave 1 is the injection point for Wave 2's incremental sync.
- The `qualityChecked` sync.Map remains as in-process dedup but the label
  check makes it redundant for restart recovery -- could be removed in Wave 2.
- Backfill tooling should be improved before Wave 3 adds more label types.

## Wave 2 Results (2026-03-30)

### What Shipped

**PR watcher: ListIssues elimination (prwatcher/watcher.go)**

- `checkReviewComments`: replaced `ListIssues(state=open, labels=[pr:N])`
  with `ListCachedIssues` SQLite query. Falls back to API when cache
  unavailable.
- `unblockDependents`: replaced `ListIssues(state=open, labels=[status:blocked])`
  with `ListCachedIssues` SQLite query. Extracted `tryUnblockIssue` helper
  for both cache and fallback paths.
- `checkAllDependenciesSatisfied`: replaced per-dependency `GetIssue` API
  calls with `GetCachedIssue` SQLite lookups. Eliminates N API calls per
  blocked issue check.

**Issue cache: body column + ListCachedIssues**

- Added `body` column to `issue_cache` table (schema migration for existing
  databases). Enables frontmatter parsing from cache without API calls.
- Added `ListCachedIssues(ctx, project, state, labels)` to store. Uses
  `LIKE` on JSON-encoded label arrays for label filtering.
- Extended `IssueCacheReader` interface so prwatcher can query cache.
- Updated `forgeIssuesToCached` to include body.

**Watch + issue_sync consolidation**

- Eliminated incremental sync ticker (was 60s). Watch events now update
  the cache directly via `updateCacheFromEvent` called from `handleEvent`.
- Full reconciliation remains at 15-minute intervals as a safety net.
- Removed `syncAllIssuesIncremental` function and
  `defaultIncrementalSyncInterval` constant.
- Net result: ~15 fewer API calls per 15-minute window (eliminated the
  `?since=` polling entirely).

### API Call Reduction

| Component | Before Wave 2 | After Wave 2 | Savings |
| --------- | ------------- | ------------ | ------- |
| Issue sync incremental (60s) | ~15 calls/15min | 0 | -15 |
| prwatcher ListIssues (remediation check) | 1 per PR per poll | 0 (cache) | variable |
| prwatcher ListIssues (unblock deps) | 1 per merge | 0 (cache) | variable |
| prwatcher GetIssue (dep check) | N per blocked issue | 0 (cache) | variable |
| Full reconciliation (15min) | unchanged | unchanged | 0 |
| Watch (30s) | unchanged | unchanged | 0 |

### Architecture Established

- **Event-driven cache updates**: Watch events write to SQLite cache
  immediately, replacing poll-based incremental sync.
- **Body in cache**: enables frontmatter parsing without API calls,
  unlocking future dependency resolution from cache.
- **Graceful degradation**: all cache-based paths fall back to API
  when cache is unavailable (nil issueCache).

### Input to Wave 3

- Event-driven updates from Watch demonstrate the EventBus pattern.
  Wave 3 formalizes this with `issue_events` SQLite table.
- `updateCacheFromEvent` is the natural integration point for an
  `EventBus.Publish()` call when the bus is introduced.
- The `issue_state` computed cache table (Wave 3) can be populated
  from the same events.

## Wave 3 Results (2026-03-30)

### What Shipped (PR 1: Cache-based polling)

**Dispatcher heartbeat: eliminated remaining forge API calls**

- `pollQueued`: replaced `ListIssues(status:queued)` per tracker with
  `ListCachedIssues` SQLite query. Falls back to API when store is nil.
- `recheckCrossProjectDeps`: replaced `ListIssues(status:blocked)` per
  tracker with `ListCachedIssues` SQLite query.
- `checkDependencies`: replaced `GetIssue` per local dependency with
  `GetCachedIssue` SQLite lookup.
- `checkCrossProjectDep`: replaced `GetIssue` on cross-project tracker
  with `GetCachedIssue` SQLite lookup.
- `buildDependencyGraph`: replaced `ListIssues(state:open)` with
  `ListCachedIssues(project, "open", nil)` for cycle detection.
- `unblockDependents`: replaced `ListIssues(status:blocked)` with
  `ListCachedIssues` SQLite query.

Added `isDoneCached` and `cachedToForgeIssue` helpers for cache-to-forge
type conversion. All paths gracefully degrade to forge API when store
is nil.

### API Call Reduction

| Component | Before Wave 3 | After Wave 3 | Savings |
| --------- | ------------- | ------------ | ------- |
| pollQueued (60s tick) | 1 ListIssues per tracker | 0 (cache) | ~3/min |
| recheckCrossProjectDeps (60s tick) | 1 ListIssues per tracker | 0 (cache) | ~3/min |
| checkDependencies (per blocked issue) | 1 GetIssue per dep | 0 (cache) | variable |
| checkCrossProjectDep (per blocked issue) | 1 GetIssue per cross-dep | 0 (cache) | variable |
| buildDependencyGraph (per new issue) | 1 ListIssues | 0 (cache) | variable |
| unblockDependents (per close) | 1 ListIssues per tracker | 0 (cache) | variable |

**Steady-state API calls from heartbeat ticker: 0** (down from ~6/min).
Only Watch polling and full reconciliation (15 min) make forge API calls.

## References

- [ADR-027: Failure Recovery](decisions/ADR-027-failure-recovery.md)
- [ADR-032: Adaptive Worker Scaling](decisions/ADR-032-adaptive-worker-scaling.md)
- [ADR-013: Forge Abstraction](decisions/ADR-013-forge-abstraction.md)
- [ADR-012: Git Issues Protocol](decisions/ADR-012-git-issues-protocol.md)
- [ADR-039: Two-Location Rule](decisions/ADR-039-two-location-centralization-rule.md)
- [Communication Protocol](communication-protocol.md)
- [Adaptive Scaling Plan](adaptive-scaling-plan.md)
- [Plan: Labels as Markers](../../.claude/plans/rustling-riding-sunset.md) -- original OOM fix plan
