# Dispatch Memory Leak Investigation -- 2026-03-31

## Symptoms

- `samverk dispatch` process: 37.8 GB RSS, 90.1% of 40 GB CT 202 RAM
- Only 1 active pool worker, doing almost nothing
- Process uptime: ~10 hours (started 12:37, checked 22:28)
- Related to issue #516 (OOM)

## pprof Data (port 6060)

### Runtime MemStats

| Metric | Value | Meaning |
|--------|-------|---------|
| HeapAlloc | 38.9 GB | Live reachable objects on heap |
| HeapInuse | 39.4 GB | Heap spans in use |
| HeapIdle | 428 MB | Free heap spans |
| HeapReleased | 2.9 MB | Returned to OS |
| Sys | 40.8 GB | Total requested from OS |
| HeapObjects | 178.7 million | Live object count |
| TotalAlloc | 464 GB | Total allocated over process lifetime |
| NextGC | 69.3 GB | Next GC trigger point |
| NumGC | 1,112 | GC cycles completed |

### Key Observation

**This is not fragmentation or GC lag.** `HeapAlloc` is 38.9 GB -- that's
178 million live, reachable objects. The GC has run 1,112 times and cannot
collect them because something still holds references.

`NextGC` is 69 GB (GOGC default doubles heap target). On a 40 GB machine,
the process will OOM before the next GC cycle fires.

### Top Allocation Paths (from pprof stacks)

All major allocators flow through the same paths:

1. `dispatcher.Run` -> `issue_sync.go` -> `fetchAllIssues` ->
   Gitea SDK `ListRepoIssues` -> `encoding/json.Unmarshal`
2. `dispatcher.Run` -> `prwatcher.Watcher.poll` ->
   Gitea SDK `ListRepoPullRequests` -> `encoding/json.Unmarshal`
3. `dispatcher.recheckCrossProjectDeps` -> `store.ListCachedIssues`

These are JSON responses from Gitea API calls, parsed into Go structs.
The question is: what holds references to old responses across poll cycles?

## Investigation Areas

### Examined -- No Leak Found

| Data Structure | Location | Why Not Leaking |
|---------------|----------|-----------------|
| `claimed map[string]*claimedIssue` | dispatcher.go | Properly cleaned on close/timeout |
| `issueFailures map[string]int` | dispatcher.go | Cleared on success/close |
| `circuitBreaker` | circuit_breaker.go | Bounded (one per provider) |
| `metrics.pollLatencies` | metrics.go | Fixed ring buffer (size 50) |
| `labelCache` | gitea.go | Rebuilt on every sync |
| `lastSyncTimes` | issue_sync.go | Bounded (one per tracker) |
| `syncAllIssuesFull` locals | issue_sync.go | Local vars freed on return |
| `deps.go` graph locals | deps.go | Local vars freed on return |
| PR watcher state | watcher.go | No accumulation |
| `eventBus` | eventbus | Bounded channels (64 buffer) |
| SQLite store | issues.go | Returns result sets, no caching |

### Primary Suspect: `known` Map in Watch Functions

**Files:**

- `internal/forge/gitea/gitea.go` -- `diffAndEmit()` / `Watch()` (~line 392-510)
- `internal/forge/github/github.go` -- `diffAndEmit()` / `Watch()` (~line 299-420)

The Watch goroutine maintains a `known map[int]*forge.Issue` that tracks
all open issues. On each poll cycle, it compares current API results against
`known` to detect new, updated, and closed issues.

**Why it's suspicious:**

- Contains full `*forge.Issue` objects (including Body -- 1KB-100KB each)
- Populated on every 30-second poll cycle
- With 4 projects polled, each with 100+ issues, over 10 hours (1,200 cycles)
- If closed issues aren't properly removed, the map grows without bound

**CAUTION:** An initial analysis claimed `delete()` during `range` iteration
is broken in Go. **This is incorrect.** Go explicitly allows and correctly
handles map deletion during range iteration. The actual leak mechanism in
the `known` map needs further investigation in a fresh session.

### Secondary Suspect: Issue Body Retention

Even if `known` map entries are deleted correctly for closed issues, the
open issues are re-stored on every poll cycle with fresh `*forge.Issue`
pointers. If old pointers are retained somewhere (e.g., in event handlers,
in the dispatcher's issue list, or in goroutine closures), they won't be
collected.

### Needs Investigation (Fresh Session)

1. Read `gitea.go` Watch/diffAndEmit carefully -- trace exactly where
   `known` map entries go after being emitted as events
2. Check if event handlers in `dispatcher.go` hold references to emitted
   `forge.Issue` objects beyond one poll cycle
3. Check if the `syncAllIssuesFull` path stores issues in any long-lived
   structure besides SQLite
4. Consider: 178 million objects / 1,200 poll cycles = ~148K objects per
   cycle. Each cycle fetches issues from 4 repos. That's ~37K objects per
   repo per cycle. A 100-issue repo with full JSON decode would produce
   ~300-400 objects per issue (fields, slices, strings). So 100 issues x
   400 objects = 40K objects per cycle per repo. That matches if NONE are
   being freed across cycles.
5. Run with `GODEBUG=gctrace=1` after restart to watch heap growth rate
6. Take two pprof snapshots 5 minutes apart and diff them to see what's
   growing

## Root Cause (confirmed 2026-03-31)

**The `ListComments` pagination loop was the wrong abstraction for the
Gitea API.**

Gitea's per-issue comments endpoint (`/repos/{owner}/{repo}/issues/{index}/comments`)
does not support page-based pagination. This is by design, not a bug --
the endpoint uses time-based filtering (`since`/`before`) instead,
matching GitHub's design for the same endpoint. Confirmed:

- [gitea/gitea#6132](https://github.com/go-gitea/gitea/issues/6132) (open since 2019)
- Gitea 1.25.5 (latest): all pagination params (`limit`, `per_page`, `pagesize`) ignored
- The issues endpoint paginates correctly; the comments endpoint does not
- [gitea/gitea#18082](https://github.com/go-gitea/gitea/issues/18082) tracks unpaginated endpoints

Our `ListComments` wrapped this endpoint in a `for { page++ }` loop that
broke on `len(batch) < 50`. The API always returns all comments (e.g., 77),
so `77 >= 50` was always true. The loop never terminated. Each iteration
appended 77 duplicate `forge.Comment` objects. Over hours: 35 GB, OOM.

pprof confirmed the accumulation was in `cloneString` <- `convertComment`
<- `ListComments` <- `tryApplyEdits` <- `handleTaskComplete` <- `Pool.worker`.
A single goroutine was stuck inside the infinite loop for issue #255
(77 comments), holding the entire `result` slice on its stack.

**All 8 callers of `ListComments` go through one function via the
`forge.IssueTracker` interface.** The fix was a single function change.

## Resolution

Removed the pagination loop from `gitea.Client.ListComments` and
`gitea.Client.ListCommentsSince`. Single API call per invocation --
working with the endpoint's design instead of against it.

- PR: (pending)
- Files changed: `internal/forge/gitea/gitea.go`, `internal/forge/gitea/gitea_test.go`
- Regression test: `TestListComments_NoInfiniteLoop` verifies exactly 1 HTTP
  request and correct comment count

## Previous Band-Aids (issue #516)

These mitigations were merged before the root cause was found:

- `maxCommentBody = 4096` truncation in `convertComment`
- `cloneString` to break JSON decoder backing array references
- `ListCommentsSince` (never wired into callers)
- `GOMEMLIMIT` warning on `backfill-labels` command

These reduced the per-iteration cost but did not stop the infinite loop.
