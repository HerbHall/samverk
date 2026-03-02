# ADR-027: Failure Recovery and State Reconciliation

## Status

Proposed

## Context

Samverk runs unattended for hours or days. The dispatcher polls for issues, agents execute tasks in containers or via cloud APIs, and QC validates output. Multiple failure modes exist -- process crashes, network partitions, external service outages, and agent deaths mid-task -- each with different blast radii and recovery requirements.

The current implementation has a critical gap: the dispatcher holds its primary coordination state (the `claimed` map tracking agent assignments, heartbeat timestamps, and failure counts) entirely in memory. A process restart loses all tracking state, leaving in-progress issues unmonitored until they happen to be rediscovered.

Additionally, the dispatcher's `Run()` method terminates when the forge `Watch` function returns an error. A transient GitHub API outage kills the entire routing engine, requiring manual restart.

The target user (hobbyist developer checking in every 1-2 days) may not notice system degradation for extended periods. Silent failures -- where the system appears healthy but is not making progress -- are the worst possible outcome for this audience.

Three architectural questions must be answered:

1. **What is the source of truth?** The forge (GitHub/Gitea issues, labels, comments) or the local database (SQLite)?
2. **Should the dispatcher persist its own state?** Or reconstruct from the forge on every restart?
3. **How aggressively should the system self-heal vs. escalate to the user?**

### Options Considered

**Option A: Stateless Dispatcher (Forge as Source of Truth)**

The dispatcher persists nothing locally. On startup, it reconstructs its `claimed` map by querying open issues with `status:in-progress` or `status:claimed` labels, reading assignees, and parsing comments for heartbeat timestamps. A forge circuit breaker prevents transient outages from terminating the dispatcher.

Pros: No local state to corrupt or drift. The forge is already the canonical record. Simple mental model.

Cons: Reconstruction requires O(n) API calls per restart. Failure counts must be re-derived from comment parsing. Slightly slower startup.

**Option B: Persistent Dispatcher State (SQLite Cache)**

The dispatcher writes its `claimed` map, circuit breaker state, and failure counts to SQLite. On startup, it reads from SQLite first, then reconciles against the forge.

Pros: Faster restarts. Failure counts survive without comment parsing. Circuit breaker state preserved across restarts during extended outages.

Cons: Two sources of truth that can drift. More code to maintain. SQLite corruption now affects routing, not just cost tracking.

**Option C: Hybrid (Selected)**

The forge remains the authoritative source of truth. SQLite serves as a performance cache for frequently-accessed state (failure counts, last poll timestamp, circuit breaker state). On startup, the dispatcher reconstructs from the forge and updates the cache. During normal operation, the cache reduces API calls. If the cache is lost, the forge provides full recovery.

## Decision

Adopt Option C: hybrid approach with forge as source of truth and SQLite as performance cache. Additionally:

1. **Forge circuit breaker**: Replace the current fail-fast behavior in `dispatcher.Run()` with a circuit breaker that retries with exponential backoff. The dispatcher never terminates due to transient forge errors.

2. **Startup reconstruction**: On startup, the dispatcher queries the forge for all in-progress and claimed issues, reconstructs its tracking state, and also checks for blocked issues whose dependencies may have resolved during downtime.

3. **Periodic reconciliation**: Every 15 minutes, the dispatcher verifies its in-memory state against the forge, catching manual changes, missed events, and drift.

4. **Graduated heartbeat timeouts**: Before unclaiming an agent, post a PING comment to give slow-but-alive agents a chance to respond. Research and orchestrator agents get longer timeouts than code-gen agents.

5. **Health reporting**: Write a health status file that the web dashboard and check-in digest can read. Distinguish "no progress because idle" from "no progress because broken."

6. **Branch continuity**: When re-assigning a previously-failed task, check for existing branch progress before starting from scratch.

## Consequences

- The dispatcher survives forge outages without termination (circuit breaker)
- Restart recovery takes 10-30 seconds (forge queries) instead of being instant
- Manual forge changes are detected within 15 minutes (reconciliation)
- SQLite gains two new tables (`dispatcher_state`, `issue_failure_counts`) as caches
- Agents must implement reconnection logic for network partition recovery
- The health file provides a monitoring surface for external tools
- Graduated timeouts reduce false positives for research tasks at the cost of slower detection for truly dead agents
- The user sees actionable health information at every check-in, even when the system is degraded

## References

- [Failure Recovery Design](../failure-recovery.md) -- full failure mode catalog and recovery designs
- [Dispatcher Design](../dispatcher-design.md) -- core routing loop and heartbeat protocol
- [Communication Protocol](../communication-protocol.md) -- issue schema and state machine
- [Autonomy Model](../autonomy-model.md) -- trust tiers governing agent actions
- [ADR-014: Dispatcher Agent](ADR-014-dispatcher-agent.md) -- why a dedicated dispatcher
