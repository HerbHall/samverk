# Optimistic Locking for Task Claiming

## Problem

Git forges (GitHub, Gitea, GitLab) do not provide atomic "claim this issue" operations. When multiple agents poll simultaneously and find the same `status:queued` issue, they could both begin working on it, wasting resources and creating conflicting results.

The forge API offers:

- `AddComment` -- append a comment
- `AddLabel` / `RemoveLabel` -- modify labels
- `Assign` -- set assignee

None of these are conditional. There is no "update if unchanged" or compare-and-swap primitive.

## Design

### Comment-Based Optimistic Lock

The lock uses issue comments as an append-only log. Comments are timestamped by the forge, creating a natural ordering that all agents can observe.

#### Claim Protocol

```text
1. Agent reads issue -- sees status:queued, no claim comments
2. Agent posts claim comment:
     "🔒 CLAIM [agent-id] [iso-timestamp]"
3. Agent waits the CLAIM_WINDOW (default: 10 seconds)
4. Agent re-reads comments on the issue
5. Agent checks: is my claim the FIRST claim comment after the
   last release (or in the entire history if no release)?
   - YES → claim succeeded, proceed to step 6
   - NO  → claim lost, proceed to step 8
6. Agent adds label status:claimed, removes status:queued
7. Agent begins work
8. Agent backs off and selects next available issue
```

#### Release Protocol

When an agent finishes, times out, or crashes:

```text
1. Agent posts release comment:
     "🔓 RELEASE [agent-id] [iso-timestamp] [reason]"
2. Agent removes status:claimed
3. Agent adds status:queued (if work incomplete) or
   status:needs-qc (if work complete)
```

### Claim Comment Format

```text
🔒 CLAIM [agent-id] [iso-timestamp]
🔓 RELEASE [agent-id] [iso-timestamp] [reason]
```

- `agent-id`: Unique identifier for the agent instance (e.g., `code-gen-a1b2c3`)
- `iso-timestamp`: RFC 3339 timestamp from the agent's clock
- `reason`: One of `completed`, `timeout`, `error`, `preempted`

The emoji prefix makes claim comments visually distinct and easy to filter programmatically.

### Why Comments, Not Labels or Assignees

| Mechanism | Atomic? | Ordered? | Audit trail? | Multi-agent safe? |
|-----------|---------|----------|-------------|-------------------|
| Labels | No | No | No | No -- two agents can add `claimed` simultaneously |
| Assignees | No | No | Partial | No -- overwrite, not append |
| Comments | No | Yes (by timestamp) | Yes | Yes -- append-only, first writer wins |

Labels and assignees are **overwrite** operations -- the second agent silently replaces the first. Comments are **append** operations -- both claims are visible, and the earliest one wins.

## Race Condition Analysis

### Race 1: Two Agents Claim Simultaneously

```text
t=0.0s  Agent A reads issue #42 (status:queued)
t=0.1s  Agent B reads issue #42 (status:queued)
t=0.2s  Agent A posts CLAIM comment
t=0.3s  Agent B posts CLAIM comment
t=10.2s Agent A re-reads comments -- sees own claim is first → succeeds
t=10.3s Agent B re-reads comments -- sees A's claim is first → backs off
```

The 10-second window ensures both agents have time to post before either checks. The forge's comment ordering (by creation timestamp) is the arbitration mechanism.

### Race 2: Claim During Release

```text
t=0.0s  Agent A posts RELEASE (completed work)
t=0.1s  Agent B reads issue, sees RELEASE, posts CLAIM
t=0.2s  Agent C reads issue, doesn't see RELEASE yet (eventual consistency), posts CLAIM
t=10.1s Agent B re-reads -- sees own claim is first after the RELEASE → succeeds
t=10.2s Agent C re-reads -- sees B's claim is first after the RELEASE → backs off
```

The claim check looks for the first claim **after the most recent release**. Prior claims before a release are invalidated.

### Race 3: Agent Crashes Before Release

```text
t=0.0s  Agent A claims issue #42
t=5m    Agent A crashes -- no RELEASE posted
t=10m   Dispatcher notices: issue has status:claimed but no progress for 10 minutes
t=10m   Dispatcher posts RELEASE on behalf of A (reason: timeout)
t=10m   Dispatcher sets status:queued -- issue available for re-claiming
```

The dispatcher monitors claimed issues and enforces a timeout. The timeout value is configurable per agent type (research agents get more time than code-gen agents).

### Race 4: Forge API Latency

```text
t=0.0s  Agent A posts CLAIM (API call takes 3 seconds due to latency)
t=0.1s  Agent B posts CLAIM (API call takes 0.5 seconds)
t=0.6s  Agent B's comment appears on the forge
t=3.0s  Agent A's comment appears on the forge
```

Even though Agent A initiated first, Agent B's comment has an earlier forge timestamp. **The forge timestamp is authoritative, not the agent's local clock.** Agent B wins.

This is correct behavior -- the forge's ordering reflects when each comment was actually received and persisted.

## Backoff Strategy

When an agent loses a claim race:

1. **Immediate retry on next issue** -- scan `status:queued` issues by priority, skip the contested one
2. **Jitter** -- add random delay (0-2 seconds) before the next claim attempt to avoid synchronized retries
3. **Exponential backoff on repeated failures** -- if an agent loses 3+ consecutive claim races, back off with `min(2^n * 1s, 60s)` before the next attempt
4. **Circuit breaker** -- if an agent loses 10 consecutive claims, log an alert and wait for the dispatcher to investigate (likely indicates a configuration issue or too many agents for the available work)

```go
type ClaimBackoff struct {
    consecutiveFailures int
    maxBackoff          time.Duration // default: 60s
}

func (b *ClaimBackoff) Next() time.Duration {
    if b.consecutiveFailures == 0 {
        return 0
    }
    jitter := time.Duration(rand.Int63n(int64(2 * time.Second)))
    base := time.Duration(1<<min(b.consecutiveFailures, 6)) * time.Second
    if base > b.maxBackoff {
        base = b.maxBackoff
    }
    return base + jitter
}

func (b *ClaimBackoff) RecordSuccess() { b.consecutiveFailures = 0 }
func (b *ClaimBackoff) RecordFailure() { b.consecutiveFailures++ }
```

## Forge Compatibility

The protocol uses only three IssueTracker operations:

| Operation | GitHub | Gitea | GitLab |
|-----------|--------|-------|--------|
| `AddComment` | Yes | Yes | Yes |
| `ListComments` | Yes | Yes | Yes |
| `SetLabels` | Yes | Yes | Yes |

No forge-specific extensions are required. The protocol works with any forge that supports issue comments and labels.

## Configuration

```yaml
# .samverk/dispatcher.yaml (or within autonomy.yaml)
claiming:
  claim_window_seconds: 10     # Time to wait before checking claim result
  claim_timeout_minutes: 30    # Max time an agent can hold a claim
  max_consecutive_failures: 10 # Circuit breaker threshold
  max_backoff_seconds: 60      # Maximum backoff duration
```

Timeouts can be overridden per agent type:

```yaml
agent_timeouts:
  code-gen: 60    # minutes -- code generation can be slow
  test: 30        # minutes
  docs: 20        # minutes
  research: 120   # minutes -- research tasks can take a while
  qc: 15          # minutes
```

## Implementation Notes

### Claim Verification Function

```go
// IsClaimHolder checks if agentID holds the active claim on an issue.
// It scans comments for the most recent RELEASE, then finds the first
// CLAIM after that RELEASE (or the first CLAIM ever if no RELEASE exists).
func IsClaimHolder(comments []*forge.Comment, agentID string) bool {
    lastReleaseIdx := -1
    for i, c := range comments {
        if isRelease(c.Body) {
            lastReleaseIdx = i
        }
    }

    for i := lastReleaseIdx + 1; i < len(comments); i++ {
        if claimID := parseClaimAgent(comments[i].Body); claimID != "" {
            return claimID == agentID
        }
    }
    return false
}
```

### Why Not Use Assignees as a Lock

GitHub and Gitea `Assign` operations are idempotent but not conditional. If two agents both call `Assign(issue, self)`, both succeed -- the second overwrites the first. There is no "assign if unassigned" operation. The overwritten agent has no way to detect that it lost.

### Why Not Use a Separate Database

A centralized database (SQLite, Redis) could provide true atomic locks. However:

- It adds an infrastructure dependency to what is otherwise a stateless system
- It doesn't survive across deployments without migration
- The forge already has comments -- adding a database duplicates state
- Comments are human-readable -- a database lock row is not

The comment-based approach trades a 10-second claim window for zero additional infrastructure.

## Related Decisions

- [ADR-012: Git Issues as Agent Communication](decisions/ADR-012-git-issues-protocol.md)
- [ADR-013: Forge Abstraction](decisions/ADR-013-forge-abstraction.md)
- [ADR-014: Dedicated Dispatcher Agent](decisions/ADR-014-dispatcher-agent.md)
- [Communication Protocol](communication-protocol.md) -- see "Optimistic Locking for Task Claiming" section
