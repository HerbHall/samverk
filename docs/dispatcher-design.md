# Dispatcher Agent Design

## Overview

The dispatcher is Samverk's always-running routing process. It watches the issue tracker for changes, classifies incoming work, resolves dependencies, and assigns tasks to the correct specialist agent pools. It is the single coordination point between the front-end conversational agent and the back-end execution agents.

**What the dispatcher IS:**

- A routing engine that matches issues to agent pools
- A dependency resolver that enforces strict blocking
- A liveness monitor that tracks agent heartbeats
- An escalation gateway that surfaces problems to the user

**What the dispatcher is NOT:**

- An execution agent -- it never writes code, runs tests, or generates docs
- An orchestrator -- it does not decompose tasks (that is the orchestrator agent's job)
- A QC agent -- it does not validate work output

The dispatcher uses only `IssueTracker` operations defined in [`internal/forge/forge.go`](../internal/forge/forge.go). It reads issue frontmatter fields defined in [`pkg/models/issue.go`](../pkg/models/issue.go). It consults the autonomy policy from [`internal/autonomy/policy.go`](../internal/autonomy/policy.go) when evaluating escalation triggers.

## Core Loop

The dispatcher runs a continuous watch-classify-route-monitor cycle:

```text
                     ┌───────────────────────────┐
                     │     Event Source           │
                     │  (webhook + poll hybrid)   │
                     └─────────┬─────────────────┘
                               │
                               ▼
                     ┌───────────────────────────┐
                     │     Event Deduplicator     │
                     └─────────┬─────────────────┘
                               │
                     ┌─────────▼─────────────────┐
                     │     Classify & Validate    │
                     │  (validate-then-trust)     │
                     └─────────┬─────────────────┘
                               │
              ┌────────────────┼────────────────┐
              │                │                │
              ▼                ▼                ▼
     ┌────────────┐   ┌────────────┐   ┌────────────┐
     │ Check Deps │   │ Route to   │   │ Escalate   │
     │ & Block    │   │ Agent Pool │   │ to Human   │
     └─────┬──────┘   └─────┬──────┘   └────────────┘
           │                │
           ▼                ▼
     ┌──────────────────────────────┐
     │     Monitor Heartbeats       │
     │   (timeout → unclaim → re-q) │
     └──────────────────────────────┘
```

### Event Sources

The dispatcher receives events through the hybrid webhook/polling architecture described in [webhook-polling-strategy.md](webhook-polling-strategy.md). Webhooks provide low-latency event delivery; polling provides reliability. The event deduplicator ensures each change is processed exactly once regardless of which source delivers it.

Events that trigger dispatcher action:

| Forge Event | Dispatcher Response |
| --- | --- |
| `EventIssueOpened` | Classify, check deps, route or block |
| `EventIssueLabeled` | Re-evaluate if status label changed |
| `EventIssueClosed` | Check if any blocked issues are now unblocked |
| `EventIssueCommented` | Check for heartbeat, claim, or release comments |
| `EventIssueEdited` | Re-parse frontmatter if body changed |

### Processing Order

When multiple events arrive in the same cycle, the dispatcher processes them in this order:

1. **Closures first** -- closing an issue may unblock others, so process these before routing
2. **New issues** -- classify and route
3. **Label changes** -- re-evaluate routing
4. **Comments** -- heartbeat and claim tracking
5. **Edits** -- frontmatter re-parsing (rare)

## Routing Strategy

The dispatcher uses a **validate-then-trust** model with an escalation ladder. The `agent_type` label set by the issue creator (orchestrator, human, or another agent) is treated as the primary routing signal. The dispatcher validates it before trusting it.

### Step 1: Basic Validation

When an issue arrives with `status:queued`, the dispatcher performs lightweight checks on the `agent_type` label:

| Check | Pass Condition | Failure Meaning |
| --- | --- | --- |
| Label exists | `agent_type` frontmatter field is set and non-empty | Issue is malformed |
| Label is known | Value is one of the `AgentType` constants in `pkg/models/issue.go` | Typo or unsupported type |
| Content pattern match | Issue body structure matches expectations for the agent type | Possible misroute |

Content pattern matching rules by agent type:

| `AgentType` | Expected Patterns |
| --- | --- |
| `code-gen` | References files, has acceptance criteria with testable code conditions |
| `test` | References test files or test scenarios, acceptance criteria about coverage or assertions |
| `docs` | References documentation files or sections, acceptance criteria about docs |
| `research` | Contains a question or analysis request, acceptance criteria about findings |
| `qc` | References a completed task issue, acceptance criteria about validation |
| `orchestrator` | High-level goal, may reference multiple sub-tasks |
| `human` | Contains a question or decision request directed at the user |
| `dispatcher` | Internal routing or dependency management task |

These checks are heuristic, not deterministic. They use keyword presence and structural analysis, not LLM inference. The goal is to catch obvious misroutes cheaply.

### Step 2: Trust the Label

If all validation checks pass, the dispatcher trusts the `agent_type` label and routes directly to the matching agent pool. No further analysis is performed. This is the fast path and should handle the vast majority of issues.

### Step 3: Reclassification

If validation fails, the dispatcher attempts reclassification using its own logic:

1. **Parse the issue body** -- extract the summary, context, and acceptance criteria sections
2. **Score against each agent type** -- count keyword matches, structural indicators, and file references
3. **Select the highest-scoring type** -- if one type scores significantly above others (2x the runner-up), reclassify
4. **Update the issue** -- change the `agent_type` in frontmatter, add a comment explaining the reclassification, and route

Reclassification is a Tier 1 action (`ActionLabelIssue`) -- it proceeds autonomously and is logged.

```text
Example reclassification comment:

🔄 RECLASSIFY [dispatcher] [2026-02-28T10:30:00Z]
Original: agent_type=docs
Reclassified to: agent_type=code-gen
Reason: Issue references internal/forge/github.go and has
acceptance criteria about method implementations. Content
pattern matches code-gen (score: 8) over docs (score: 2).
```

### Step 4: Thorough Examination Escalation

If reclassification produces no clear winner (no type scores 2x the runner-up, or two types are tied), the dispatcher escalates:

1. **Label the issue** with `status:needs-human` and `agent:dispatcher`
2. **Comment** with the scoring breakdown and the dispatcher's best guess
3. **Continue processing other issues** -- this one issue is parked, not the whole system

The user resolves it at the next check-in by confirming or correcting the agent type. Once resolved, the dispatcher routes immediately.

## Dependency Management

### Strict Blocking

Issues with `depends_on` entries in their frontmatter stay `status:queued` (or transition to `status:blocked`) until **all** dependencies reach `status:done`. There is no partial unblocking or optimistic parallel work on dependent issues.

Rules:

- When the dispatcher processes a new `status:queued` issue, it reads `depends_on` from frontmatter
- If any dependency issue is not `status:done`, the dispatcher adds `status:blocked` and comments with the blocking issue numbers
- When a dependency issue closes (`EventIssueClosed`), the dispatcher scans all `status:blocked` issues to check if their full dependency set is now satisfied
- When all dependencies for a blocked issue are `status:done`, the dispatcher transitions it to `status:queued` for routing

### Dependency Check Using IssueTracker

```text
For each issue in status:blocked:
  1. Parse depends_on from frontmatter → [dep1, dep2, dep3]
  2. For each dep in depends_on:
     a. tracker.GetIssue(ctx, dep)
     b. Check if issue state is StateClosed AND has label status:done
  3. If ALL deps are done → RemoveLabel(status:blocked), AddLabel(status:queued)
  4. If any dep is NOT done → no change, add/update comment with remaining blockers
```

IssueTracker methods used:

- `GetIssue` -- check dependency status
- `ListIssues` -- find all `status:blocked` issues
- `AddLabel` / `RemoveLabel` -- transition between `status:blocked` and `status:queued`
- `AddComment` -- document blocking reasons and unblock events

### Cycle Detection

Dependency cycles would cause permanent deadlock. The dispatcher detects them before they cause problems.

Algorithm (runs when a new issue is created or when `depends_on` is edited):

```text
1. Build a directed graph: issue → depends_on issues
2. Run DFS from the new/edited issue
3. If DFS revisits a node in the current path → cycle detected
4. On cycle detection:
   a. Label ALL issues in the cycle with status:needs-human
   b. Comment on each with the full cycle path
   c. Escalate to user (see Escalation Policy)
```

The dependency graph is built from frontmatter, not from a separate data structure. The dispatcher reconstructs it on demand by listing open issues and parsing their `depends_on` fields. This avoids maintaining a separate state that could drift.

### Blocked Issue Tracking

The dispatcher comments on blocked issues with a structured format:

```text
⏸️ BLOCKED [dispatcher] [2026-02-28T10:30:00Z]
Waiting on: #42 (status:in-progress), #43 (status:queued)
Will auto-unblock when all dependencies reach status:done.
```

When a dependency resolves:

```text
✅ UNBLOCKED [dispatcher] [2026-02-28T11:15:00Z]
Dependency #42 completed. All dependencies satisfied.
Transitioning to status:queued for routing.
```

## Heartbeat Protocol

Agents must post heartbeat comments at regular intervals while working on a claimed issue. The heartbeat serves as a liveness signal and provides progress visibility.

### Heartbeat Format

```text
💓 HEARTBEAT [agent-id] [iso-timestamp]
progress: 45%
status: Running test suite (12/47 passed)
```

Fields:

| Field | Required | Description |
| --- | --- | --- |
| `agent-id` | Yes | The agent instance identifier |
| `iso-timestamp` | Yes | RFC 3339 timestamp |
| `progress` | Yes | Percentage estimate (0-100) |
| `status` | Yes | Brief human-readable description of current activity |

### Heartbeat Intervals

Intervals vary by execution environment because local and cloud agents have different latency characteristics:

| Environment | Heartbeat Interval | Timeout Threshold |
| --- | --- | --- |
| Local agent (containerized) | Every 10 minutes | 15 minutes (1.5x interval) |
| Cloud agent (API-based) | Every 30 minutes | 45 minutes (1.5x interval) |

The 1.5x multiplier provides tolerance for network hiccups and API latency without false positives.

### Timeout Handling

When the dispatcher detects a missing heartbeat (no heartbeat within the timeout threshold since the last one or since the claim):

```text
1. Post a RELEASE comment on behalf of the agent:
   "🔓 RELEASE [dispatcher] [iso-timestamp] timeout
    Agent [agent-id] missed heartbeat. Last seen: [last-heartbeat-time].
    Unclaiming issue for re-queue."
2. RemoveLabel(status:claimed) or RemoveLabel(status:in-progress)
3. AddLabel(status:queued)
4. Unassign the agent: Unassign(ctx, number, agent-id)
5. Increment the failure counter for this issue
6. If failure count >= 3 → escalate (see Escalation Policy)
```

The dispatcher does NOT attempt to kill or restart the timed-out agent. Agent lifecycle management is separate from routing. The dispatcher only manages the issue state.

### Heartbeat Monitoring Implementation

The dispatcher maintains an in-memory map of claimed issues and their last heartbeat timestamp:

```text
claimedIssues: map[issueNumber] → {
    agentID:           string
    claimedAt:         time.Time
    lastHeartbeat:     time.Time
    heartbeatInterval: time.Duration
    failureCount:      int
}
```

On every poll cycle (or on `EventIssueCommented`), the dispatcher:

1. Scans comments on claimed issues for heartbeat patterns
2. Updates `lastHeartbeat` if a new heartbeat is found
3. Checks if `time.Since(lastHeartbeat) > heartbeatInterval * 1.5`
4. If exceeded, triggers timeout handling

## Escalation Policy

The dispatcher escalates to the user by creating or labeling issues with `status:needs-human`. Escalation triggers are categorized by severity.

### Escalation Triggers

| Trigger | Severity | Dispatcher Action |
| --- | --- | --- |
| Tier 3 autonomy action requiring confirmation | High | Create `needs-human` issue per autonomy model |
| Budget threshold exceeded | High | Add `status:needs-human` label, comment with cost breakdown |
| 3 consecutive agent failures on the same issue | High | Add `status:needs-human`, comment with failure history |
| Ambiguous priority conflict (two critical items competing for same resource) | Medium | Add `status:needs-human`, comment with conflict details |
| Dependency cycle detected | Critical | Add `status:needs-human` to all cycle members, comment with cycle path |
| Reclassification dispute (no clear agent type) | Medium | Add `status:needs-human`, comment with scoring breakdown |

### Escalation Comment Format

```text
🚨 ESCALATE [dispatcher] [iso-timestamp]
trigger: 3_consecutive_failures
severity: high
issue: #42
details: Agent code-gen-a1b2c3 failed 3 times on this issue.
  Attempt 1: timeout after 60 minutes (no heartbeat)
  Attempt 2: error -- "compilation failed: undefined reference"
  Attempt 3: timeout after 60 minutes (no heartbeat)
action_needed: User should review the issue complexity and
  acceptance criteria. Consider reclassifying or breaking
  into smaller sub-tasks.
```

### Tier 3 Integration

The dispatcher checks the `AutonomyPolicy` before routing certain actions. When an agent requests an action classified as `Tier3` by `policy.TierFor(action)`:

1. The dispatcher does NOT route the action to an agent
2. Instead, it creates a `needs-human` issue with the pending action details
3. The action is queued until the user approves at the next check-in
4. On approval, the dispatcher re-routes with the action pre-approved

The `AutonomyPolicy.RequiresConfirmation(action)` method returns `true` for all Tier 3 actions. The dispatcher calls this for any action that involves `ActionType` values like `ActionMergeMain`, `ActionDeleteFile`, `ActionForcePush`, `ActionAPICallExpense`, etc.

### Budget Threshold

The dispatcher tracks cumulative token spend per issue and per project session. When spend exceeds `AutonomyPolicy.CostThreshold()` (default: `$5.00` per the `DefaultCostThresholdUSD` constant), the dispatcher escalates.

Budget tracking uses the `actual_tokens` and `model_used` frontmatter fields from completed issues to compute cost. The dispatcher does not call provider APIs directly -- it reads cost data from issue metadata.

## Agent Selection

When multiple agents of the same type are available (e.g., two `code-gen` containers running), the dispatcher must choose which one gets the work.

### Selection Strategy: Weighted Score

The dispatcher scores each available agent and assigns to the highest scorer:

| Factor | Weight | Measurement |
| --- | --- | --- |
| Current load | 40% | Number of `status:in-progress` issues assigned to this agent (fewer is better) |
| Recent success rate | 30% | Ratio of completed vs. failed/timed-out tasks in the last 24 hours |
| Capability match | 20% | Does the agent's model match the `complexity` label? (local agent for `complexity:local`, cloud for `complexity:cloud`) |
| Cost efficiency | 10% | Estimated cost per token for this agent's provider |

### Tie-Breaking

If two agents score identically, the dispatcher uses round-robin based on the agent's last assignment timestamp (least-recently-assigned wins).

### Single Agent Fallback

If only one agent of the required type exists, it gets the work regardless of load. The dispatcher logs a warning if a single agent's queue depth exceeds 5 issues.

### No Available Agents

If no agents of the required type are running:

1. **Wait one poll cycle** -- the agent may be starting up
2. **After 3 poll cycles with no agent** -- add `status:blocked` with comment: "No `{agent_type}` agents available"
3. **After 10 minutes** -- escalate to user with `status:needs-human`

## Cost Awareness

The dispatcher factors cost into routing decisions using the `complexity` label and the user's cost tier configuration.

### Routing by Complexity

| `Complexity` Label | Routing Decision |
| --- | --- |
| `ComplexityLocal` | Route to a local agent (Ollama container). Skip cloud agents entirely. |
| `ComplexityCloud` | Route to a cloud agent (Claude, GPT-4, Gemini). Fall back to local if no cloud credits remain. |
| `ComplexityAmbiguous` | Dispatcher evaluates: if estimated_tokens < 2000 and agent_type is code-gen/test/docs, route local. Otherwise route cloud. |

### Cost Failover

When a cloud provider returns a billing/quota error:

1. **Try next cloud provider** in the user's priority order
2. **If all cloud providers exhausted** -- fall back to local agents with a warning
3. **Log the failover** as a comment on the issue
4. **Notify at next check-in** -- include cost status in the digest

### Budget Guards

Before routing an issue, the dispatcher checks:

- `estimated_tokens` against the per-task cost threshold
- Cumulative session spend against the per-session budget
- If either would be exceeded, escalate to user instead of routing

## State Machine

Issue lifecycle from the dispatcher's perspective. Each transition maps to specific `IssueTracker` operations.

```text
    ┌──────────┐
    │  OPENED  │  (new issue created)
    └────┬─────┘
         │ classify & validate
         ▼
    ┌──────────┐  deps not met   ┌───────────┐
    │  QUEUED  │────────────────►│  BLOCKED   │
    └────┬─────┘                 └─────┬──────┘
         │                             │ all deps done
         │ deps met                    │
         │◄────────────────────────────┘
         │
         │ route to agent
         ▼
    ┌──────────┐
    │ CLAIMED  │  (optimistic lock acquired)
    └────┬─────┘
         │ agent starts work
         ▼
    ┌─────────────┐
    │ IN-PROGRESS │◄──────────────────────────┐
    └────┬────────┘                           │
         │                                    │
         ├── agent completes ──►┌───────────┐ │
         │                      │ NEEDS-QC  │ │
         │                      └─────┬─────┘ │
         │                            │       │
         │                  QC pass ──┼──►┌──────┐
         │                            │   │ DONE │
         │                  QC fail ──┘   └──────┘
         │                    │
         │                    └── re-queued ──────┘
         │
         ├── heartbeat timeout ──► RELEASE ──► QUEUED
         │                                      │
         │                          failure++   │
         │                          if >= 3 ────┼──►┌──────────────┐
         │                                      │   │ NEEDS-HUMAN  │
         │                                      │   └──────────────┘
         └── escalation trigger ────────────────┘
```

### State Transitions and IssueTracker Operations

| Transition | Forge Operations |
| --- | --- |
| OPENED -> QUEUED | `AddLabel(status:queued)` |
| QUEUED -> BLOCKED | `RemoveLabel(status:queued)`, `AddLabel(status:blocked)`, `AddComment(blocking details)` |
| BLOCKED -> QUEUED | `RemoveLabel(status:blocked)`, `AddLabel(status:queued)`, `AddComment(unblock notice)` |
| QUEUED -> CLAIMED | Agent posts claim comment, waits 10s, verifies via `ListComments`, then `RemoveLabel(status:queued)`, `AddLabel(status:claimed)`, `Assign(agent-id)` |
| CLAIMED -> IN-PROGRESS | `RemoveLabel(status:claimed)`, `AddLabel(status:in-progress)` |
| IN-PROGRESS -> NEEDS-QC | Agent: `RemoveLabel(status:in-progress)`, `AddLabel(status:needs-qc)`, `AddComment(result)` |
| NEEDS-QC -> DONE | QC agent: `RemoveLabel(status:needs-qc)`, `AddLabel(status:done)`, close issue |
| NEEDS-QC -> QUEUED (retry) | QC agent: `RemoveLabel(status:needs-qc)`, `AddLabel(status:queued)`, `AddComment(failure details)` |
| IN-PROGRESS -> QUEUED (timeout) | Dispatcher: `AddComment(RELEASE)`, `RemoveLabel(status:in-progress)`, `AddLabel(status:queued)`, `Unassign(agent-id)` |
| Any -> NEEDS-HUMAN | `AddLabel(status:needs-human)`, `AddComment(escalation details)` |

## IssueTracker Integration

Every dispatcher action maps to specific methods on the `IssueTracker` interface from [`internal/forge/forge.go`](../internal/forge/forge.go).

### Methods Used by Dispatcher Action

| Dispatcher Action | IssueTracker Methods |
| --- | --- |
| Watch for changes | `Watch(ctx, handler)` |
| Read issue details | `GetIssue(ctx, number)` |
| List queued/blocked issues | `ListIssues(ctx, &ListOptions{State: StateOpen, Labels: ["status:queued"]})` |
| Route to agent | `Assign(ctx, number, agentID)`, `AddLabel(ctx, number, "status:claimed")`, `RemoveLabel(ctx, number, "status:queued")` |
| Block on dependency | `AddLabel(ctx, number, "status:blocked")`, `RemoveLabel(ctx, number, "status:queued")`, `AddComment(ctx, number, blockMsg)` |
| Unblock after dependency resolves | `RemoveLabel(ctx, number, "status:blocked")`, `AddLabel(ctx, number, "status:queued")`, `AddComment(ctx, number, unblockMsg)` |
| Timeout an agent | `AddComment(ctx, number, releaseMsg)`, `Unassign(ctx, number, agentID)`, `RemoveLabel(ctx, number, "status:in-progress")`, `AddLabel(ctx, number, "status:queued")` |
| Escalate to user | `AddLabel(ctx, number, "status:needs-human")`, `AddComment(ctx, number, escalateMsg)` |
| Reclassify | `AddComment(ctx, number, reclassifyMsg)` (frontmatter update via `UpdateIssue` if body edit is needed) |
| Check heartbeats | `ListComments(ctx, number)` |
| Create escalation issue | `CreateIssue(ctx, &CreateIssueRequest{...})` |

### Frontmatter Parsing

The dispatcher parses the YAML frontmatter block from issue bodies using the `IssueFrontmatter` struct from `pkg/models/issue.go`. It extracts:

- `agent_type` (`AgentType`) -- routing target
- `priority` (`Priority`) -- scheduling order
- `depends_on` (`[]int`) -- dependency list
- `complexity` (`Complexity`) -- local vs. cloud routing hint
- `estimated_tokens` (`int`) -- cost estimation
- `schema_version` (`string`) -- for forward compatibility

## Configuration

All dispatcher behavior is configurable via `.samverk/dispatcher.yaml`. Defaults are conservative and work for single-user self-hosted deployments.

### Full Configuration Schema

```yaml
# .samverk/dispatcher.yaml

# Routing validation
routing:
  # Minimum score multiplier for reclassification (winner must score
  # this many times higher than runner-up)
  reclassification_threshold: 2.0

  # Maximum number of reclassification attempts before escalating
  max_reclassification_attempts: 1

# Heartbeat monitoring
heartbeat:
  # Interval at which local agents must post heartbeats
  local_interval_minutes: 10

  # Interval at which cloud agents must post heartbeats
  cloud_interval_minutes: 30

  # Multiplier applied to interval to determine timeout
  # (e.g., 1.5 means timeout = interval * 1.5)
  timeout_multiplier: 1.5

# Agent failure handling
failures:
  # Number of consecutive failures before escalating to user
  max_consecutive: 3

  # Cooldown before re-queuing a failed issue (seconds)
  requeue_cooldown_seconds: 60

# Dependency management
dependencies:
  # How often to re-check blocked issues for unblocked deps (seconds)
  # Only relevant in poll-only mode; webhook mode checks on closure events
  recheck_interval_seconds: 120

# Agent selection
selection:
  # Weights for agent scoring (must sum to 1.0)
  load_weight: 0.4
  success_rate_weight: 0.3
  capability_weight: 0.2
  cost_weight: 0.1

  # Queue depth warning threshold per agent
  queue_depth_warning: 5

  # Poll cycles to wait before marking "no agent" as blocked
  no_agent_wait_cycles: 3

  # Minutes before escalating "no agent" to needs-human
  no_agent_escalate_minutes: 10

# Cost guards
cost:
  # Per-task token budget (overrides estimated_tokens if lower)
  max_tokens_per_task: 50000

  # Per-session budget in USD (0 = unlimited)
  session_budget_usd: 0.0

# Task claiming (also in optimistic-locking.md)
claiming:
  claim_window_seconds: 10
  claim_timeout_minutes: 30
  max_consecutive_failures: 10
  max_backoff_seconds: 60

# Per-agent-type claim timeout overrides (minutes)
agent_timeouts:
  code-gen: 60
  test: 30
  docs: 20
  research: 120
  qc: 15
  orchestrator: 90

# Watch configuration (also in webhook-polling-strategy.md)
watch:
  mode: hybrid
  polling:
    interval_seconds: 60
    backoff_max_seconds: 300
  webhook:
    enabled: true
    listen_addr: ":8081"
    secret: ""
    tls_cert: ""
    tls_key: ""
  dedup:
    ttl_seconds: 300
```

### Configuration Precedence

1. `.samverk/dispatcher.yaml` in the project directory (highest)
2. `~/.samverk/dispatcher.yaml` in the user's home directory (global defaults)
3. Compiled-in defaults (lowest)

## Implementation Phases

### Phase 1: Core Routing (MVP)

Build the minimum viable dispatcher that can route issues to agents.

- Poll-only event source (no webhooks)
- Validate-then-trust routing (steps 1-2 only, no reclassification)
- Strict dependency blocking with cycle detection
- Single-agent-per-type assumption (no agent selection scoring)
- Heartbeat monitoring with timeout and re-queue
- Basic escalation: 3 failures, dependency cycles

IssueTracker methods needed: `ListIssues`, `GetIssue`, `AddComment`, `ListComments`, `AddLabel`, `RemoveLabel`, `Assign`, `Unassign`.

### Phase 2: Intelligence

Add smarter routing and multi-agent support.

- Reclassification logic (steps 3-4 of the escalation ladder)
- Agent selection scoring with weighted factors
- Cost-aware routing using `complexity` labels
- Budget guards and cost threshold escalation
- Configurable heartbeat intervals per environment

### Phase 3: Reliability

Add production-grade reliability features.

- Webhook event source (hybrid mode)
- Event deduplication
- Graceful degradation on forge API failures
- Persistent state recovery after dispatcher restart (rebuild in-memory maps from issue state)
- Metrics and structured logging for operational visibility

### Phase 4: Optimization

Performance and UX improvements.

- Conditional polling with `If-None-Match` (GitHub) to reduce API usage
- Webhook registration via IssueTracker interface (auto-configure forge)
- Priority preemption (critical issue can bump a low-priority agent off a shared resource)
- Batch unblocking (when a parent issue closes, unblock all children in one pass)

## Related Documents

- [Communication Protocol](communication-protocol.md) -- issue schema, label taxonomy, lifecycle
- [Architecture](architecture.md) -- system design and agent hierarchy
- [Optimistic Locking](optimistic-locking.md) -- task claiming protocol
- [Webhook/Polling Strategy](webhook-polling-strategy.md) -- event source design
- [Autonomy Model](autonomy-model.md) -- trust tiers and escalation
- [Cost Model](cost-model.md) -- tiered cost structure
- [ADR-014: Dispatcher Agent](decisions/ADR-014-dispatcher-agent.md) -- why a dedicated dispatcher
