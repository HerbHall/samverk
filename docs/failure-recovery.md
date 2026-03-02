# Failure Recovery

Samverk runs unattended for hours or days. The dispatcher polls for issues, agents execute tasks, QC validates output. This document designs what happens when things go wrong -- crash recovery, external dependency failures, state reconciliation, and user notification.

**ADR**: [ADR-027](decisions/ADR-027-failure-recovery.md)

## Design Principles

Three principles govern all recovery behavior:

1. **The forge is the source of truth.** Issue labels, comments, and assignments on the git forge (GitHub/Gitea) are the canonical state. In-memory state is a cache, not the master record.
2. **Err toward re-doing, not skipping.** If recovery cannot determine whether a task completed, re-queue it. Wasted tokens are cheaper than missed work.
3. **Silent failures are worse than loud halts.** Every unrecoverable error must surface to the user within one check-in cycle. A system that appears healthy but is not working is the worst outcome for a hobbyist developer who checks in every 1-2 days.

## Failure Mode Catalog

### Component Inventory

| Component | Process | Persisted State | In-Memory State |
|-----------|---------|-----------------|-----------------|
| Dispatcher | Long-running goroutine in `samverk serve` | None (reads forge on demand) | `claimed` map: issue -> agent ID, heartbeat times, failure counts |
| Agent (local) | Container managed by Docker | Git branch with commits | Working tree, uncommitted changes, model context |
| Agent (cloud) | API call session | Git branch with commits | API conversation state, partial output |
| Store (SQLite) | Embedded in `samverk serve` | Sessions, cost records | WAL journal, open transactions |
| Forge API | External service (GitHub/Gitea) | Issues, labels, comments, assignments | Connection pool, rate limit state |
| Provider API | External service (Claude/OpenAI/Ollama) | None | Active inference session |

### Failure Modes

#### F1: Dispatcher Process Crash

**Cause**: Unhandled panic, OOM kill, host reboot, power loss.

**Blast radius**: All routing, heartbeat monitoring, and dependency resolution stops. Agents that are already running continue unmonitored. No new work gets assigned. Blocked issues stay blocked even if their dependencies resolve.

**Detection**: The dispatcher process disappears. No new comments or label changes appear on the forge. The system monitor (systemd, Docker restart policy, or user observation) detects the absence.

**Current state at risk**: The `claimed` map in `internal/dispatcher/dispatcher.go` holds agent IDs, heartbeat timestamps, and failure counts for all in-progress issues. This is entirely in-memory. A crash loses:

- Which issues are claimed and by whom
- Last heartbeat timestamp per claimed issue
- Failure count per issue (how many times an agent has failed on it)

**Recovery**: See [Dispatcher Restart Recovery](#dispatcher-restart-recovery).

#### F2: Agent Dies Mid-Task

**Cause**: Container killed (OOM, Docker restart, host reboot), cloud API timeout, network loss during execution.

**Blast radius**: One task stalls. The git branch may have partial commits (committed but not pushed), uncommitted changes (lost), or no progress at all.

**Git branch state possibilities**:

| Scenario | Branch State | Recovery Action |
|----------|-------------|-----------------|
| Agent committed and pushed | Remote branch has partial work | New agent continues from branch HEAD |
| Agent committed but not pushed | Local commits exist in container volume | Recover if volume survives; otherwise re-start from last pushed commit |
| Agent has uncommitted changes | Changes exist only in container filesystem | Lost unless container volume is recoverable |
| Agent made no progress | Branch is clean at starting point | Re-assign with no data loss |

**Detection**: Heartbeat timeout. The dispatcher's `checkTimeouts` function (in `internal/dispatcher/heartbeat.go`) detects missing heartbeats when `time.Since(lastHeartbeat) > heartbeatInterval * 1.5`.

**Recovery**: The existing timeout handler (`releaseTimedOut`) already handles this correctly:

1. Posts a RELEASE comment on the issue
2. Removes `status:in-progress` label
3. Adds `status:queued` label
4. Unassigns the dead agent
5. Increments failure counter
6. Escalates after 3 consecutive failures

**Gap**: No mechanism to inspect the git branch for partial progress before re-assignment. A new agent starting from scratch wastes the dead agent's pushed commits. See [Branch Continuity Protocol](#branch-continuity-protocol).

#### F3: Forge API Downtime

**Cause**: GitHub outage, Gitea server unreachable, network partition between Samverk and the forge.

**Blast radius**: The dispatcher cannot read issues, post comments, change labels, or assign agents. All routing halts. Agents that are already running can continue local work but cannot push branches or report results.

**Detection**: `IssueTracker` method calls return errors. The `Watch` function in the event loop reports an error via the `errCh` channel in `dispatcher.Run()`.

**Current behavior**: The dispatcher's `Run` method returns an error when `Watch` stops, which terminates the entire dispatcher. This is too aggressive -- a transient network blip kills the routing engine.

**Recovery**: See [Forge Circuit Breaker](#forge-circuit-breaker).

#### F4: Provider API Outage

**Cause**: Anthropic/OpenAI/Ollama service unavailable, rate limits exceeded, billing issues, model deprecation.

**Blast radius**: Agents using the affected provider cannot make inference calls. Tasks assigned to those agents stall. Other provider pools are unaffected.

**Detection**: Provider client returns specific error types (HTTP 429, 500, 503, billing errors). The agent reports the error as a heartbeat status or fails to produce a heartbeat at all.

**Recovery**: See [Provider Failover Protocol](#provider-failover-protocol).

#### F5: SQLite Database Corruption

**Cause**: Write during power loss (unlikely with WAL mode), disk failure, filesystem corruption.

**Blast radius**: Session history and cost tracking are lost. The dispatcher itself is unaffected because it reads state from the forge, not from SQLite. Budget enforcement breaks because `GetBudgetStatus` cannot query cost records.

**Detection**: SQLite returns `SQLITE_CORRUPT` or `SQLITE_NOTADB` errors on any query.

**Current mitigation**: WAL mode (enabled in `internal/store/store.go:New()`) provides crash-safe writes. The database survives most unexpected process terminations without corruption.

**Recovery**: See [Database Recovery](#database-recovery).

#### F6: Agent Produces Invalid Output

**Cause**: Model hallucination, context window overflow, wrong model for task complexity, ambiguous acceptance criteria.

**Blast radius**: One task produces bad output. If QC catches it, the task is re-queued. If QC misses it and the task has dependents, bad output propagates downstream.

**Detection**: QC agent validates against acceptance criteria. The QC flow (described in `docs/communication-protocol.md`) catches most invalid output. After 3 QC rejections, the issue escalates to `status:needs-human`.

**Recovery**: Existing QC loop handles this. No additional recovery mechanism needed beyond ensuring QC agents are robust. The 3-strike escalation is the safety net.

#### F7: Network Partition -- Agent Has Work but Cannot Push

**Cause**: Agent's network connection to the forge drops while the agent is mid-task. The agent can continue local work (writing code, running tests) but cannot push commits or post heartbeats.

**Blast radius**: The dispatcher sees a missing heartbeat and will eventually time out the agent. The agent is doing useful work that will be discarded if it cannot reconnect before timeout.

**Current timeout**: 15 minutes for local agents (10 min interval * 1.5 multiplier). This is too short for network partitions that may last 30-60 minutes.

**Recovery**: See [Network Partition Handling](#network-partition-handling).

#### F8: Stale Dispatcher State After Manual Forge Changes

**Cause**: User manually changes issue labels, closes issues, or reassigns agents through the forge UI while the dispatcher is running. The dispatcher's `claimed` map diverges from reality.

**Blast radius**: The dispatcher may attempt to manage an issue that is already closed, or miss an issue that was manually re-opened.

**Detection**: On each poll cycle, the dispatcher should reconcile its `claimed` map against actual forge state. Currently, the dispatcher only processes events -- it does not do full reconciliation.

**Recovery**: See [State Reconciliation Protocol](#state-reconciliation-protocol).

## Recovery Designs

### Dispatcher Restart Recovery

When the dispatcher process starts (or restarts after a crash), it must reconstruct its `claimed` map from forge state. The forge is the source of truth -- the in-memory map is a performance cache.

**Reconstruction algorithm**:

```text
On Dispatcher.Run() start, before entering the event loop:

1. List all open issues with label "status:in-progress" or "status:claimed"
   issues = tracker.ListIssues(ctx, {State: open, Labels: ["status:in-progress"]})
   issues += tracker.ListIssues(ctx, {State: open, Labels: ["status:claimed"]})

2. For each issue:
   a. Read assignee → agentID
   b. Read comments (most recent first) → find last HEARTBEAT comment
   c. Parse heartbeat timestamp → lastHeartbeat
   d. Count RELEASE comments → failureCount
   e. Insert into claimed map:
      claimed[issue.Number] = {
          AgentID: assignee,
          ClaimedAt: issue.UpdatedAt (approximation),
          LastHeartbeat: heartbeat timestamp or issue.UpdatedAt,
          FailureCount: count of RELEASE comments
      }

3. List all open issues with label "status:blocked"
   For each, verify dependencies are still unresolved.
   If all deps are now done (resolved during downtime), unblock.

4. List all open issues with label "status:queued"
   For each, attempt routing (some may have been queued during downtime).

5. Enter the normal event loop.
```

**Persistence requirement**: None. The forge holds all the data needed for reconstruction. This is intentional -- the dispatcher is stateless by design. Reconstruction takes O(n) forge API calls where n is the number of active issues, which is acceptable for the expected scale (tens to low hundreds of issues).

**Implementation note**: The reconstruction should happen in a new `Dispatcher.reconstruct(ctx)` method called at the top of `Run()`, before starting the watch loop. If reconstruction fails (forge unreachable), the dispatcher should retry with exponential backoff before entering the event loop.

### Branch Continuity Protocol

When an agent dies mid-task and the issue is re-queued, the new agent should check for prior work before starting from scratch.

**Protocol**:

```text
When an agent claims a re-queued issue (failureCount > 0):

1. Check if a feature branch exists for this issue:
   branch name pattern: feature/issue-{number}-*

2. If branch exists with commits ahead of main:
   a. Read the last commit message and diff
   b. Assess whether the partial work is usable
   c. If usable: continue from branch HEAD
   d. If not usable (conflicts, wrong approach): reset branch to main

3. If no branch exists:
   Start fresh (normal flow)

4. Post a CONTINUITY comment on the issue:
   "CONTINUITY [agent-id] [timestamp]
    Prior work found on branch feature/issue-{number}-desc.
    Commits ahead of main: {count}
    Decision: continuing from HEAD / starting fresh
    Reason: {assessment}"
```

**Cost savings**: For tasks that are 50-80% complete when the agent dies, this avoids re-doing the completed portion. For a hobbyist developer paying per token, this is meaningful.

### Forge Circuit Breaker

The dispatcher must survive transient forge outages without terminating.

**States**:

```text
CLOSED (normal) ──── error ────► OPEN (failing)
       ▲                              │
       │                              │ cooldown expires
       │                              ▼
       └──── success ◄──── HALF-OPEN (probing)
```

**Configuration** (in `dispatcher.yaml`):

```yaml
circuit_breaker:
  # Consecutive failures before opening the circuit
  failure_threshold: 5

  # How long to wait before probing (seconds)
  cooldown_seconds: 30

  # Maximum cooldown after repeated failures (seconds)
  max_cooldown_seconds: 300

  # Backoff multiplier for cooldown
  backoff_multiplier: 2.0
```

**Behavior**:

| State | Dispatcher Behavior |
|-------|-------------------|
| CLOSED | Normal operation. All forge calls proceed. |
| OPEN | All forge calls are skipped. Heartbeat checks continue against cached timestamps. No new routing. A timer counts down to HALF-OPEN. |
| HALF-OPEN | One probe call (e.g., `ListIssues` with a single-item page). If it succeeds, transition to CLOSED and run full reconciliation. If it fails, transition back to OPEN with doubled cooldown. |

**Key design point**: During OPEN state, the dispatcher does NOT terminate. It logs the outage, continues ticking, and retries automatically. The user sees degraded behavior (no new routing) but the system self-heals when the forge returns.

**Escalation**: If the circuit stays OPEN for longer than `escalation_after_minutes` (default: 30), the dispatcher writes a local health file (`.samverk/health.json`) that the web dashboard can read. The next user check-in surfaces this degradation.

### Provider Failover Protocol

When an AI provider is unreachable, the system fails over to alternatives following the user's configured priority.

**Failover chain** (configured in `.samverk/providers.yaml`):

```yaml
providers:
  cloud:
    - name: claude
      priority: 1
      max_retries: 3
      retry_delay_seconds: 10
    - name: openai
      priority: 2
      max_retries: 3
      retry_delay_seconds: 10
    - name: gemini
      priority: 3
      max_retries: 3
      retry_delay_seconds: 10
  local:
    - name: ollama
      priority: 1
      max_retries: 2
      retry_delay_seconds: 5
```

**Failover behavior**:

```text
1. Agent attempts provider[0] (claude)
2. On failure, retry up to max_retries with exponential backoff
3. If all retries exhausted:
   a. Log the failure as an issue comment
   b. Try provider[1] (openai)
   c. Repeat retry logic
4. If all cloud providers exhausted:
   a. Fall back to local provider if task complexity allows
   b. If complexity:cloud and no cloud available:
      - Post a PROVIDER_UNAVAILABLE comment on the issue
      - Transition issue to status:blocked
      - Set a retry timer (check again in 15 minutes)
5. If all providers (cloud + local) exhausted:
   a. Escalate to status:needs-human
   b. Comment: "All configured providers are unreachable"
```

**Session continuity**: If a provider fails mid-inference (partial response), the agent should discard the partial output and retry with the same or fallback provider. Partial inference results are unreliable and should never be committed.

### Network Partition Handling

An agent that loses network connectivity should buffer heartbeats and work locally, then reconcile when connectivity returns.

**Agent-side behavior**:

```text
1. Agent detects network failure (push fails, heartbeat post fails)
2. Agent enters PARTITIONED mode:
   a. Continue local work (write code, run tests)
   b. Buffer heartbeat data locally (progress %, status)
   c. Attempt reconnection every 60 seconds
3. On reconnection:
   a. Post a RECONNECT comment with buffered heartbeat history
   b. Push accumulated commits
   c. Resume normal heartbeat cadence
4. If partitioned for longer than max_partition_minutes (default: 60):
   a. Save all local work to a recovery branch: recovery/issue-{number}-{timestamp}
   b. Stop execution
   c. On next reconnection, post a PARTITION_TIMEOUT comment
```

**Dispatcher-side behavior**:

The dispatcher will time out the agent after the heartbeat timeout (15 minutes for local agents). This is correct behavior -- the dispatcher cannot distinguish "network partition" from "agent crash." The agent's reconnection protocol handles the reconciliation.

**Race condition**: If the dispatcher re-queues the issue AND a new agent claims it BEFORE the partitioned agent reconnects, two agents work on the same issue. Resolution:

```text
When a reconnecting agent detects its issue was re-assigned:
1. Stop work immediately
2. Push accumulated work to recovery/issue-{number}-{agent-id}
3. Post a PARTITION_RECOVERY comment listing the recovery branch
4. The new agent (or dispatcher) decides whether to merge recovery work
```

### State Reconciliation Protocol

The dispatcher must periodically verify that its in-memory `claimed` map matches forge reality. This catches manual changes, missed events, and post-crash drift.

**Reconciliation runs**:

- On dispatcher startup (see [Dispatcher Restart Recovery](#dispatcher-restart-recovery))
- Every `reconciliation_interval_minutes` (default: 15) during normal operation
- After a circuit breaker transitions from OPEN to CLOSED

**Algorithm**:

```text
reconcile(ctx):

1. claimed_on_forge = ListIssues(ctx, {State: open, Labels: ["status:in-progress", "status:claimed"]})
2. claimed_in_memory = d.claimed

3. For each issue in claimed_in_memory but NOT in claimed_on_forge:
   // Issue was closed, manually un-assigned, or label removed while we weren't watching
   → Remove from claimed map
   → Log: "Reconciliation: removed stale claim on #{number}"

4. For each issue in claimed_on_forge but NOT in claimed_in_memory:
   // Issue was claimed by an agent that started before the dispatcher
   // or was manually assigned through the forge UI
   → Add to claimed map with current time as lastHeartbeat
   → Log: "Reconciliation: tracking externally claimed #{number}"

5. For each issue in BOTH:
   // Verify assignee matches
   → If assignee differs, update claimed map to match forge
   → Log discrepancy

6. Check for orphaned states:
   // Issues labeled "status:claimed" with no assignee
   → Transition to status:queued
   // Issues labeled "status:in-progress" with no recent heartbeat (and not in claimed map)
   → Apply heartbeat timeout logic
```

**Dependent task recovery**: When reconciliation discovers a completed dependency that was missed during downtime, it triggers the same `unblockDependents` logic that `handleClosed` uses. This ensures blocked issues are eventually unblocked even if the closure event was lost.

### Database Recovery

SQLite with WAL mode is already crash-safe for normal process termination. Additional protections for edge cases:

**Backup strategy**:

```text
1. Daily backup: copy the SQLite database file to .samverk/backups/
   Retention: 7 daily backups
   Triggered by: a cron-style ticker in the server process

2. Pre-migration backup: before any schema migration, copy the database
   Stored alongside the migration script for rollback

3. Integrity check: run PRAGMA integrity_check on startup
   If corruption detected:
   a. Log the error
   b. Attempt to recover from most recent backup
   c. If no backup: create a fresh database (sessions/cost data is lost
      but the system remains functional -- forge state is unaffected)
```

**What SQLite stores vs. what the forge stores**:

| Data | Location | Loss Impact |
|------|----------|-------------|
| Task state (status, assignment, deps) | Forge (issues, labels) | System cannot function |
| Session history | SQLite | Audit trail lost, but system continues |
| Cost records | SQLite | Budget enforcement temporarily disabled |
| User profile | SQLite | Preferences reset to defaults |
| Agent heartbeats | In-memory + forge comments | Reconstructable from forge |

The forge is the critical data store. SQLite loss is inconvenient but not catastrophic.

## State Persistence Requirements

### What Must Survive a Full Restart

| State | Persistence Mechanism | Survives Restart | Reconstruction Method |
|-------|----------------------|------------------|-----------------------|
| Issue lifecycle (status, labels, assignments) | Git forge (GitHub/Gitea) | Yes (external service) | Direct read via `IssueTracker` |
| Dependency graph | Forge issue frontmatter (`depends_on`) | Yes | Rebuilt from `ListIssues` + `ParseFrontmatter` |
| Heartbeat timestamps | Forge comments | Yes | Parse comments for HEARTBEAT pattern |
| Failure counts per issue | Forge comments | Yes | Count RELEASE comments per issue |
| Agent session history | SQLite `sessions` table | Yes (WAL mode) | Direct query |
| Cost tracking | SQLite `cost_records` table | Yes (WAL mode) | Direct query |
| Claimed issue map | In-memory only | No | Reconstructed from forge on startup |
| Circuit breaker state | In-memory only | No | Resets to CLOSED on startup (safe default) |
| Event deduplication cache | In-memory only | No | Resets on startup; duplicate events are idempotent |

### What Should Be Added to SQLite

Currently, the dispatcher holds no persistent state. For production reliability, the following should be persisted:

**New table: `dispatcher_state`**

```sql
CREATE TABLE IF NOT EXISTS dispatcher_state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

Records to persist:

| Key | Value | Purpose |
|-----|-------|---------|
| `last_poll_timestamp` | RFC3339 timestamp | Resume polling from where we left off |
| `circuit_breaker_state` | `closed`, `open`, `half-open` | Avoid resetting circuit breaker on restart during extended outage |
| `circuit_breaker_since` | RFC3339 timestamp | Track how long the circuit has been open |

**New table: `issue_failure_counts`**

```sql
CREATE TABLE IF NOT EXISTS issue_failure_counts (
    issue_number INTEGER PRIMARY KEY,
    failure_count INTEGER NOT NULL DEFAULT 0,
    last_failure_at TEXT,
    last_agent_id TEXT
);
```

This provides a persistent failure count that survives restarts without needing to re-parse forge comments every time. The forge comments remain the authoritative source, but this table is a performance cache.

## Heartbeat and Liveness Protocol

### Current Design

The heartbeat protocol is defined in `docs/dispatcher-design.md` and implemented in `internal/dispatcher/heartbeat.go`. Key parameters:

| Parameter | Local Agent | Cloud Agent |
|-----------|-------------|-------------|
| Heartbeat interval | 10 minutes | 30 minutes |
| Timeout threshold | 15 minutes (1.5x) | 45 minutes (1.5x) |

### Enhancement: Graduated Timeout Response

The current design treats all timeouts identically: unclaim and re-queue. A graduated response provides better outcomes:

```text
1. SOFT TIMEOUT (1.5x interval):
   - Post a PING comment on the issue:
     "PING [dispatcher] [timestamp]
      Agent {agent-id} has not sent a heartbeat in {elapsed}.
      If still active, please post a heartbeat."
   - Do NOT unclaim yet
   - Start a grace period (0.5x interval)

2. HARD TIMEOUT (2.0x interval):
   - No response to PING within grace period
   - Execute full timeout: unclaim, re-queue
   - This is the current releaseTimedOut behavior

3. EXTENDED TIMEOUT (for agent types with long tasks):
   - research agents: 3.0x interval before hard timeout
   - orchestrator agents: 2.5x interval before hard timeout
   - code-gen agents: 2.0x interval (default)
   - Configurable per agent type in dispatcher.yaml
```

**Rationale**: Research agents may spend extended periods reading and analyzing without producing intermediate output. A premature timeout wastes significant work. The PING step gives a working-but-slow agent a chance to respond.

### Agent Liveness Categories

| Category | Meaning | Dispatcher Response |
|----------|---------|-------------------|
| ALIVE | Heartbeat received within interval | No action |
| SLOW | No heartbeat, but within soft timeout | Post PING comment |
| UNRESPONSIVE | Past soft timeout, within hard timeout | Grace period running |
| DEAD | Past hard timeout | Unclaim, re-queue, increment failure count |
| PARTITIONED | Agent reconnects after being marked DEAD | Reconciliation protocol |

## Idempotency Guarantees

### Why Idempotency Matters

When the dispatcher re-queues a task after an agent failure, the new agent may redo work the first agent already completed. Certain operations are not naturally idempotent:

| Operation | Naturally Idempotent | Risk |
|-----------|---------------------|------|
| Write a file | Yes (overwrite) | None |
| Create a git commit | Yes (same content = same hash) | None |
| Create an issue | No (duplicates) | Duplicate issues |
| Post a comment | No (duplicates) | Noisy issue thread |
| Add a label | Yes (already present = no-op) | None |
| Merge a branch | Partially (already merged = no-op, but conflicts possible) | Merge conflicts |
| API call with side effects | No | Duplicate external actions |

### Idempotency Strategy

Samverk does not enforce strict idempotency at the framework level. Instead, it uses **at-least-once delivery with deduplication hints**:

1. **Issue comments** include structured headers (`HEARTBEAT [agent-id] [timestamp]`, `CLAIM [agent-id] [timestamp]`). The dispatcher can detect and ignore duplicate comments by matching the pattern and timestamp.

2. **Label operations** are idempotent by nature in both GitHub and Gitea APIs. Adding a label that already exists is a no-op.

3. **Agent work** follows a branch-per-issue pattern. If a new agent starts on the same issue, it works on the same branch. Git handles the merge/overwrite naturally.

4. **Issue creation** by agents should always check for existing issues before creating new ones. The issue title and parent issue number serve as the deduplication key.

## User Notification for System Health

### Health Status Model

The system maintains a health status with four levels:

| Level | Meaning | User Impact |
|-------|---------|-------------|
| HEALTHY | All components operational | Normal check-in digest |
| DEGRADED | One or more non-critical components failing | Warning banner in digest |
| IMPAIRED | Core routing or execution affected | Prominent alert in digest |
| DOWN | No useful work possible | Emergency notification |

### Health Check Components

```text
System Health = aggregate of:

1. Forge connectivity:
   HEALTHY = last successful API call < 5 minutes ago
   DEGRADED = last successful call 5-30 minutes ago
   DOWN = last successful call > 30 minutes ago

2. Provider availability:
   HEALTHY = at least one cloud + one local provider responsive
   DEGRADED = only local or only cloud available
   IMPAIRED = one provider type available but failing intermittently
   DOWN = no providers responsive

3. Agent pool health:
   HEALTHY = all expected agent types have at least one instance
   DEGRADED = one or more agent types have no instances
   IMPAIRED = agents are running but all tasks are failing

4. Database health:
   HEALTHY = reads and writes succeed
   DEGRADED = reads succeed but writes fail (WAL corruption)
   DOWN = all queries fail (rebuild from backup)

5. Task throughput:
   HEALTHY = tasks completing at expected rate
   DEGRADED = task completion rate dropped > 50% from rolling average
   IMPAIRED = no tasks completed in last 2 hours despite queued work
```

### Health File

The dispatcher writes a health snapshot to `.samverk/health.json` every 5 minutes:

```json
{
  "timestamp": "2026-03-01T10:30:00Z",
  "overall": "healthy",
  "components": {
    "forge": {"status": "healthy", "last_success": "2026-03-01T10:29:45Z"},
    "providers": {"status": "healthy", "available": ["claude", "ollama"]},
    "agents": {"status": "healthy", "active": 3, "types": ["code-gen", "test", "docs"]},
    "database": {"status": "healthy", "last_write": "2026-03-01T10:28:00Z"},
    "throughput": {"status": "healthy", "completed_1h": 4, "queued": 7}
  },
  "issues": []
}
```

When health is not HEALTHY, the `issues` array contains specific problems:

```json
{
  "issues": [
    {
      "component": "forge",
      "severity": "degraded",
      "message": "GitHub API returning 503 since 2026-03-01T10:15:00Z",
      "since": "2026-03-01T10:15:00Z",
      "action": "Circuit breaker OPEN. Auto-retrying every 60s."
    }
  ]
}
```

### Distinguishing "No Progress" Reasons

A critical UX requirement: the user must be able to distinguish "nothing to do" from "something is broken."

**Diagnosis in the check-in digest**:

```text
If queued_count == 0 AND in_progress_count == 0:
  → "All caught up. No work queued."

If queued_count > 0 AND in_progress_count == 0 AND agent_count > 0:
  → "⚠️ {N} tasks queued but nothing in progress.
     Possible routing issue. Check dispatcher logs."

If queued_count > 0 AND agent_count == 0:
  → "⚠️ {N} tasks queued but no agents are running.
     Start agents with: samverk agent start"

If in_progress_count > 0 AND last_heartbeat > 2x interval:
  → "⚠️ {N} tasks in progress but agents appear stalled.
     Last heartbeat: {time}. Dispatcher will auto-recover."

If circuit_breaker == OPEN:
  → "🔴 Cannot reach {forge}. Work is paused.
     Auto-retrying. Last successful contact: {time}."

If all_providers_down:
  → "🔴 No AI providers available. Cannot execute tasks.
     Check API keys and provider status."
```

### Notification Channels

Phase 1 (current): Health information surfaces at check-in only. The user must actively ask "how's my project doing?" to learn about issues.

Phase 2 (planned): Proactive notifications for IMPAIRED and DOWN states:

| Channel | When | Content |
|---------|------|---------|
| Web dashboard banner | Always visible when health is not HEALTHY | Component status with timestamps |
| Health file | Written every 5 minutes | Full health snapshot for external monitoring |
| Forge issue | When IMPAIRED or DOWN persists for > 30 minutes | System health issue with `status:needs-human` |

Phase 3 (future): External integrations for urgent notifications:

- Email digest when DOWN state persists for > 1 hour
- Webhook to user-configured endpoint (Slack, Discord, Pushover)

## Recovery Procedures

### Automatic Recovery (No User Action Required)

| Failure | Recovery | Time to Recover |
|---------|----------|-----------------|
| Dispatcher restart | Reconstruct claimed map from forge | 10-30 seconds |
| Agent heartbeat timeout | Unclaim, re-queue, new agent picks up | 15-45 minutes |
| Transient forge API error | Circuit breaker retry | 30 seconds - 5 minutes |
| Provider API rate limit | Exponential backoff + failover | 10 seconds - 5 minutes |
| Missed dependency resolution event | Periodic reconciliation catches it | Up to 15 minutes |

### User-Required Recovery

| Failure | User Action | Surfaced Via |
|---------|-------------|-------------|
| 3 consecutive agent failures on same task | Review task complexity, break into sub-tasks | `status:needs-human` issue |
| Dependency cycle detected | Break the cycle by editing `depends_on` | `status:needs-human` on all cycle members |
| All providers down | Check API keys, provider status, network | Health file + check-in digest |
| Database corruption with no backup | Accept data loss, restart with fresh database | Startup error log |
| Extended forge outage (> 1 hour) | Verify forge is accessible, check network | Health file + check-in digest |
| Budget exhaustion | Add credits or raise budget threshold | `status:needs-human` issue |

### Manual Recovery Commands

```bash
# Check system health
samverk health

# Force reconciliation of dispatcher state against forge
samverk dispatch reconcile

# List and recover from a partitioned agent's work
samverk agent recover --issue 42

# Rebuild the SQLite database from forge data
samverk store rebuild --from-forge

# Reset the circuit breaker (force closed)
samverk dispatch reset-circuit

# Show recent failures with context
samverk dispatch failures --since 24h
```

## Configuration Reference

All recovery-related configuration in `.samverk/dispatcher.yaml`:

```yaml
# Recovery and resilience settings
recovery:
  # Run state reconciliation every N minutes
  reconciliation_interval_minutes: 15

  # Maximum time an agent can be partitioned before timeout
  max_partition_minutes: 60

  # SQLite backup settings
  database_backup:
    enabled: true
    interval_hours: 24
    retention_count: 7

circuit_breaker:
  failure_threshold: 5
  cooldown_seconds: 30
  max_cooldown_seconds: 300
  backoff_multiplier: 2.0
  escalation_after_minutes: 30

health:
  # Write health file every N minutes
  write_interval_minutes: 5

  # Create a needs-human issue after N minutes of IMPAIRED/DOWN
  escalation_threshold_minutes: 30

  # Throughput monitoring
  stall_detection_hours: 2
```

## Implementation Phases

### Phase 1: Core Recovery (Implement Now)

- Dispatcher restart reconstruction from forge state
- Forge circuit breaker with exponential backoff
- State reconciliation on startup
- Health file writing
- Check-in digest health awareness

### Phase 2: Enhanced Resilience

- Periodic reconciliation during normal operation
- Graduated heartbeat timeout (PING before RELEASE)
- Branch continuity protocol for re-queued tasks
- Provider failover chain
- SQLite backup and integrity checks

### Phase 3: Proactive Monitoring

- Throughput-based stall detection
- Proactive notifications (forge issue for extended outages)
- External notification channels (webhooks)
- Recovery CLI commands (`samverk health`, `samverk dispatch reconcile`)

## Related Documents

- [Dispatcher Design](dispatcher-design.md) -- core routing loop and heartbeat protocol
- [Communication Protocol](communication-protocol.md) -- issue schema and state machine
- [Autonomy Model](autonomy-model.md) -- trust tiers governing agent actions
- [Architecture](architecture.md) -- system components and hierarchy
- [Multi-Session Safety](multi-session-safety.md) -- concurrent access guardrails
- [Cost Control](cost-control.md) -- budget enforcement and runaway prevention
- [Security Model](security-model.md) -- authentication and authorization
- [ADR-027: Failure Recovery Approach](decisions/ADR-027-failure-recovery.md)
