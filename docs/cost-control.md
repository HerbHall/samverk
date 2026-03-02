# Cost Control Design

Hard budget caps, runaway detection, and local fallback for an async agent system where the user is not watching.

## Problem Statement

Samverk agents work while the user is away -- sleeping, at work, on vacation. This is the core value proposition but also the core risk. A retry loop at 3:00 AM can burn $50 in 20 minutes. A QC disagreement cycle between two agents can double cost with no user-visible progress. A provider failover cascade can multiply a $2 task into a $15 task across three providers before anyone notices.

The existing autonomy model (Tier 2/3 cost thresholds) provides a per-API-call gate. This document designs the system-wide budget enforcement, runaway detection, and graceful degradation that wraps around individual API calls to protect the user's wallet at every granularity.

## API Pricing Reference (March 2026)

All cost calculations in this document use these rates.

### Cloud Providers

| Provider | Model | Input ($/MTok) | Output ($/MTok) | Notes |
|----------|-------|----------------|-----------------|-------|
| Anthropic | Claude Opus 4.6 | $5.00 | $25.00 | Most capable, architecture/QC |
| Anthropic | Claude Sonnet 4.6 | $3.00 | $15.00 | Balanced, primary workhorse |
| Anthropic | Claude Haiku 4.5 | $1.00 | $5.00 | Fast, classification/routing |
| OpenAI | GPT-4o | $2.50 | $10.00 | Cross-model validation |
| OpenAI | GPT-4o mini | $0.15 | $0.60 | Cheap tasks, formatting |
| Google | Gemini 2.5 Flash | $0.15 | $0.60 | Budget alternative |
| Google | Gemini 3.1 Pro | $1.25 | $10.00 | Mid-tier reasoning |

Batch API discounts (50% off) apply to non-urgent work. Prompt caching reduces input cost to 10% of base for repeated context.

### Local Models (Ollama, $0 per token)

| Model | Size | VRAM | Tokens/sec (RTX 3060) | Quality Tier |
|-------|------|------|----------------------|-------------|
| Qwen 3 8B (Q4) | 5 GB | 6 GB | ~40 tok/s | Code gen, tests |
| Codestral (Q4) | 5 GB | 6 GB | ~60 tok/s | Code gen, formatting |
| Llama 3.3 8B (Q4) | 5 GB | 6 GB | ~50 tok/s | General tasks |
| Mistral Small 24B (Q4) | 14 GB | 16 GB | ~30 tok/s | Mid-tier reasoning |

Local models cost electricity only (~$0.10-0.30/day for continuous GPU use). Their cost is effectively zero in budget calculations.

## Token Consumption by Task Type

Research shows agentic coding tasks are heavily input-token driven (input tokens dominate cost even with caching), and token usage exhibits up to 10x variance across runs for similar task complexity. These estimates are conservative medians.

### Typical Task Token Budgets

| Task Type | Input Tokens | Output Tokens | Estimated Cost (Sonnet) | Estimated Cost (Haiku) |
|-----------|-------------|--------------|------------------------|----------------------|
| Code generation (single file, 100-300 LOC) | 15,000 | 5,000 | $0.12 | $0.04 |
| Code generation (multi-file feature) | 40,000 | 15,000 | $0.35 | $0.12 |
| Code review / QC pass | 25,000 | 3,000 | $0.12 | $0.04 |
| Test generation (one module) | 20,000 | 8,000 | $0.18 | $0.06 |
| Architecture/planning decision | 30,000 | 10,000 | $0.24 | $0.08 |
| Bug fix (diagnosis + patch) | 35,000 | 8,000 | $0.23 | $0.07 |
| Documentation (one doc) | 10,000 | 5,000 | $0.11 | $0.04 |
| Dispatcher routing (per issue) | 3,000 | 500 | $0.02 | $0.01 |

### Compound Task Costs

A typical feature issue flows through multiple agent types:

```text
Issue lifecycle for "Add Gitea webhook handler":
  1. Dispatcher routing:           $0.02  (Haiku)
  2. Code generation:              $0.35  (Sonnet)
  3. Test generation:              $0.18  (Sonnet)
  4. QC review (pass 1):           $0.12  (Sonnet)
  5. QC feedback → code fix:       $0.23  (Sonnet)
  6. QC review (pass 2):           $0.12  (Sonnet)
  7. Documentation:                $0.11  (Haiku)
                                  -------
  Total:                           $1.13

  With local models handling steps 2, 3, 7:  $0.49
  With prompt caching (60% cache hit):       $0.71
  Best case (local + caching):               $0.31
```

## Cost Runaway Scenarios

Five realistic scenarios where costs spiral beyond expectations, with estimated burn rates.

### Scenario 1: Retry Loop on Compilation Error

An agent generates code that fails to compile. It reads the error, modifies the code, recompiles, fails again. The error is a misunderstanding of the API, not a typo -- each retry produces a different wrong approach.

```text
Iteration 1: 40K input + 15K output (Sonnet) = $0.35
Iteration 2: 45K input + 12K output           = $0.32  (growing context)
Iteration 3: 50K input + 15K output           = $0.38
Iteration 4: 55K input + 12K output           = $0.36
Iteration 5: 60K input + 15K output           = $0.42
...
After 10 iterations: ~$3.80 in 8 minutes
After 20 iterations: ~$9.50 in 18 minutes (context window growing)

Without intervention: $25+ per hour
```

**Detection signal:** Same issue, same agent, repeated commit-fail-retry pattern. Cost per iteration stays constant or grows (context accumulation).

### Scenario 2: QC Disagreement Cycle

The code-gen agent and QC agent disagree. Code-gen produces implementation A, QC rejects with feedback. Code-gen produces implementation B based on feedback. QC rejects B for reasons that lead back to A. The cycle repeats.

```text
Round 1: code-gen ($0.35) + QC ($0.12) = $0.47
Round 2: code-gen ($0.38) + QC ($0.12) = $0.50  (growing context)
Round 3: code-gen ($0.42) + QC ($0.14) = $0.56
...
After 5 rounds: ~$2.75 in 25 minutes
After 10 rounds: ~$6.50 in 50 minutes

Without intervention: $8-15/hour
```

**Detection signal:** Same issue bouncing between `status:in-progress` and `status:needs-qc` more than N times. Cost accumulating without the issue reaching `status:done`.

### Scenario 3: Provider Failover Cascade

Primary provider (Claude Sonnet) returns rate-limit errors. System falls back to secondary (GPT-4o, $2.50/$10.00). GPT-4o also rate-limited. Falls back to Gemini 3.1 Pro ($1.25/$10.00). Each failover retries the full context because the previous provider's partial output is lost.

```text
Attempt 1: Sonnet 40K input, rate-limited after 2K output  = $0.15 (wasted)
Attempt 2: GPT-4o 40K input + 15K output                   = $0.25
  -- GPT-4o output fails QC (different style) --
Attempt 3: Re-do on Gemini 40K input + 15K output          = $0.20
  -- Gemini output also fails QC --
Attempt 4: Back to Sonnet (rate limit cleared) 50K in + 15K = $0.38

Total for one task: $0.98 (vs $0.35 normal)
Multiply across 5 concurrent tasks: $4.90 for one batch
```

**Detection signal:** Multiple provider switches on the same task within a short window. Total task cost exceeding 3x the estimated cost.

### Scenario 4: Dependency Deadlock Spawning

Task A depends on Task B. Task B fails and gets re-queued. The orchestrator, seeing B failed, creates sub-tasks C and D to address the root cause. C depends on D. D fails and spawns E and F. Each spawned task consumes tokens for context loading and initial analysis.

```text
Task B: $0.35 (fails)
Task C: $0.24 (spawned, analysis)
Task D: $0.24 (spawned, analysis, fails)
Task E: $0.24 (spawned from D)
Task F: $0.24 (spawned from D)
Task E fails, spawns G and H...

Exponential growth: 2^n tasks at ~$0.24 each
After 4 levels: 16 tasks = $3.84, none completed
After 6 levels: 64 tasks = $15.36, none completed
```

**Detection signal:** Issue spawn rate exceeding close rate. Dependency graph depth growing without any leaf task completing. Total open-issue count climbing.

### Scenario 5: Context Window Exhaustion Loop

An agent working on a large codebase hits the context window limit. It summarizes and restarts, losing important context. The new attempt makes the same mistakes the original context would have prevented. The cycle of "load context, work, hit limit, summarize, restart" burns tokens on repeated context loading.

```text
Attempt 1: 180K input (near limit) + 10K output  = $0.69 (Sonnet)
  -- hits context limit, summarizes --
Attempt 2: 120K input (summary + code) + 10K out  = $0.51
  -- makes mistake from lost context, fails QC --
Attempt 3: 150K input + 12K output                = $0.63
  -- same mistake pattern --

After 5 attempts: ~$3.20 in 30 minutes
Each attempt loads ~130K tokens of context that was already paid for
```

**Detection signal:** Same agent-issue pair with repeated large input token counts. Agent summary/restart pattern detected via comment parsing. Cost per attempt not decreasing despite re-attempts.

## Budget Enforcement Design

### Budget Granularity

Budgets are enforced at four levels, from narrowest to broadest. A violation at any level triggers enforcement.

| Level | Scope | Default | Purpose |
|-------|-------|---------|---------|
| **Per-call** | Single API call | $5.00 | Catch expensive individual calls (Tier 2/3 boundary) |
| **Per-task** | Single issue lifecycle | $10.00 | Catch runaway single issues |
| **Per-day** | All tasks in 24h window | $25.00 | Catch daily overruns |
| **Per-month** | Calendar month total | $50.00 | Hard cap matching Tier 2 target |

### Configuration

```yaml
# .samverk/cost.yaml

budgets:
  # Per-API-call threshold (Tier 2 → Tier 3 boundary)
  per_call_usd: 5.00

  # Per-task ceiling (entire issue lifecycle)
  per_task_usd: 10.00

  # Daily budget (rolling 24-hour window)
  per_day_usd: 25.00

  # Monthly budget (calendar month)
  per_month_usd: 50.00

# Cap behavior when budget is hit
enforcement:
  # "hard" = stop all work immediately
  # "soft" = degrade to local-only, notify user
  # "notify" = continue but alert at next check-in
  per_call: hard      # always hard -- prevents single expensive calls
  per_task: soft      # fall back to local, continue other tasks
  per_day: soft       # fall back to local for remainder of day
  per_month: hard     # stop all cloud work, local continues

# Local fallback behavior
local_fallback:
  # When cloud budget exhausted, route to local if available
  enabled: true

  # Tasks above this complexity skip local fallback and pause instead
  # (local models cannot handle architecture/QC at this level)
  max_local_complexity: medium

  # Queue cloud-complexity tasks for next budget window
  queue_expensive: true

# Alert thresholds (percentage of budget consumed)
alerts:
  warning_pct: 70     # "You've used 70% of your daily budget"
  critical_pct: 90    # "You're about to hit your daily limit"

# Cost attribution
attribution:
  # Track cost per: issue, agent_type, provider, model
  dimensions:
    - issue
    - agent_type
    - provider
    - model
```

### Cap Types: Hard vs Soft

**Hard caps** stop all cloud API work immediately. In-progress API calls complete (we cannot cancel a streaming response), but no new calls are initiated. The agent commits any partial work, posts a cost-limit comment on the issue, and transitions the issue to `status:paused`.

**Soft caps** degrade gracefully. Cloud API calls stop, but local models continue. Tasks that are within local model capability are re-routed. Tasks that require cloud capability are queued with `status:budget-hold` and resume when the budget resets or the user increases it.

### Enforcement Decision Matrix

| Budget Level | Amount Hit | Cap Type | Action |
|-------------|-----------|----------|--------|
| Per-call | Single call would exceed $5 | Hard | Block call, escalate to Tier 3 (existing autonomy model) |
| Per-task | Issue lifetime cost exceeds $10 | Soft | Pause cloud calls for this issue, try local, flag for user |
| Per-day | Daily spend reaches $25 | Soft | All new cloud calls degrade to local, queue cloud-only tasks |
| Per-month | Monthly spend reaches $50 | Hard | All cloud work stops, local continues, user notified |

### What Happens to In-Progress Work

When a budget cap triggers, in-progress work follows these rules:

1. **Current streaming API call:** Completes. Cannot cancel mid-stream without losing tokens already billed.
2. **Current task (issue):** Agent commits whatever partial work exists to the feature branch. Posts a structured comment (see format below).
3. **Queued tasks:** Remain queued. Not affected by budget enforcement.
4. **Dependent tasks:** Remain blocked (dependency not met). Not double-penalized.

Budget comment format posted by the agent on pause:

```text
BUDGET [agent-id] [timestamp]
trigger: per_day_limit
spent: $24.82 / $25.00
partial_work: committed to branch feature/issue-42 (3 files modified)
resume_action: will continue when budget resets or user increases limit
```

## Budget Enforcement State Machine

The cost controller tracks the system's budget state and enforces transitions.

```text
                    ┌────────────────┐
                    │    NORMAL      │ All budgets within limits
                    │  (cloud + local│ Cloud routing active
                    │   both active) │
                    └───────┬────────┘
                            │
               budget >= warning_pct (70%)
                            │
                            ▼
                    ┌────────────────┐
                    │    WARNING     │ Alert queued for next check-in
                    │  (still active,│ Digest shows projected exhaustion
                    │   alert queued)│ time
                    └───────┬────────┘
                            │
               budget >= critical_pct (90%)
                            │
                            ▼
                    ┌────────────────┐
                    │   CRITICAL     │ Aggressive cost reduction:
                    │  (throttled,   │ - Downgrade models (Sonnet→Haiku)
                    │   model        │ - Prefer local for all eligible
                    │   downgrade)   │   tasks
                    └───────┬────────┘
                            │
               budget >= 100% (soft cap)
                            │
                            ▼
                    ┌────────────────┐
                    │  LOCAL_ONLY    │ Cloud calls blocked
                    │  (cloud paused,│ Local agents continue
                    │   local active)│ Cloud tasks queued with
                    │                │ status:budget-hold
                    └───────┬────────┘
                            │
               budget >= 100% (hard cap) OR
               monthly limit hit
                            │
                            ▼
                    ┌────────────────┐
                    │    STOPPED     │ All new work paused
                    │  (all paused,  │ In-progress completes + commits
                    │   user must    │ User must resume explicitly
                    │   resume)      │
                    └────────────────┘

        Recovery transitions (any state → NORMAL):
        - User increases budget
        - Budget window resets (daily/monthly)
        - User types "resume" at check-in
```

### State Machine Implementation

```go
// BudgetState represents the current cost enforcement level.
type BudgetState int

const (
    BudgetNormal   BudgetState = iota // All systems active
    BudgetWarning                     // Alert queued, still operating
    BudgetCritical                    // Model downgrade, prefer local
    BudgetLocalOnly                   // Cloud calls blocked
    BudgetStopped                     // All new work paused
)

// CostController manages budget enforcement across all granularities.
type CostController struct {
    mu           sync.RWMutex
    state        BudgetState
    config       CostConfig
    store        CostStore
    clock        func() time.Time

    // Running totals (also persisted to SQLite)
    taskCosts    map[int]float64      // issue number → cumulative cost
    dailySpend   float64
    monthlySpend float64

    // Listeners notified on state transitions
    listeners    []BudgetStateListener
}

// PreApprove checks whether a proposed API call should proceed.
// Called BEFORE every cloud provider API call.
func (cc *CostController) PreApprove(ctx context.Context, req CostRequest) (CostDecision, error) {
    cc.mu.RLock()
    defer cc.mu.RUnlock()

    estimated := req.EstimatedInputTokens*req.InputRate +
                 req.EstimatedOutputTokens*req.OutputRate

    // Check per-call limit
    if estimated > cc.config.PerCallUSD {
        return CostDecision{
            Approved: false,
            Reason:   "per-call limit exceeded",
            Action:   ActionEscalateTier3,
        }, nil
    }

    // Check per-task limit
    taskTotal := cc.taskCosts[req.IssueNumber] + estimated
    if taskTotal > cc.config.PerTaskUSD {
        return CostDecision{
            Approved: false,
            Reason:   "per-task limit exceeded",
            Action:   ActionFallbackLocal,
        }, nil
    }

    // Check per-day limit
    if cc.dailySpend+estimated > cc.config.PerDayUSD {
        return CostDecision{
            Approved: false,
            Reason:   "daily budget exhausted",
            Action:   ActionFallbackLocal,
        }, nil
    }

    // Check per-month limit
    if cc.monthlySpend+estimated > cc.config.PerMonthUSD {
        return CostDecision{
            Approved: false,
            Reason:   "monthly budget exhausted",
            Action:   ActionStopAll,
        }, nil
    }

    return CostDecision{Approved: true}, nil
}

// Record logs actual cost after an API call completes.
func (cc *CostController) Record(ctx context.Context, usage CostUsage) error {
    cc.mu.Lock()
    defer cc.mu.Unlock()

    cost := float64(usage.InputTokens)*usage.InputRate +
            float64(usage.OutputTokens)*usage.OutputRate

    cc.taskCosts[usage.IssueNumber] += cost
    cc.dailySpend += cost
    cc.monthlySpend += cost

    // Persist to SQLite
    if err := cc.store.RecordUsage(ctx, usage); err != nil {
        return err
    }

    // Evaluate state transitions
    cc.evaluateState()

    return nil
}
```

### CostRequest and CostDecision Types

```go
// CostRequest describes a proposed API call for pre-approval.
type CostRequest struct {
    IssueNumber          int
    AgentType            string
    Provider             string
    Model                string
    EstimatedInputTokens int
    EstimatedOutputTokens int
    InputRate            float64  // $/token
    OutputRate           float64  // $/token
}

// CostDecision is the result of a pre-approval check.
type CostDecision struct {
    Approved bool
    Reason   string
    Action   CostAction
    // When Action is ActionDowngradeModel, this is the suggested model
    SuggestedModel string
}

// CostAction defines what the caller should do when not approved.
type CostAction int

const (
    ActionApproved       CostAction = iota
    ActionEscalateTier3             // Requires user confirmation
    ActionFallbackLocal             // Try local model instead
    ActionDowngradeModel            // Use cheaper cloud model
    ActionStopAll                   // Pause all work
)

// CostUsage records actual token usage after an API call.
type CostUsage struct {
    IssueNumber  int
    AgentType    string
    Provider     string
    Model        string
    InputTokens  int
    OutputTokens int
    InputRate    float64
    OutputRate   float64
    Timestamp    time.Time
    CacheHit     bool    // Was prompt caching used?
    BatchMode    bool    // Was batch API used?
}
```

## Runaway Detection Algorithm

The runaway detector operates independently from budget enforcement. Budget caps are reactive (triggered when money is spent). Runaway detection is predictive (triggered when spending patterns indicate a problem before the budget is exhausted).

### Detection Signals

| Signal | Description | Threshold | Window |
|--------|-------------|-----------|--------|
| **Retry frequency** | Same issue, same agent, repeated attempts | 3 attempts in 10 min | Per-issue |
| **QC bounce count** | Issue oscillating between in-progress and needs-qc | 3 bounces | Per-issue lifetime |
| **Cost velocity** | Rate of spend acceleration | Spend rate doubling in 5 min | System-wide |
| **Spawn rate** | New issues created faster than issues closed | Open rate > 2x close rate for 15 min | System-wide |
| **Context growth** | Input tokens per call growing without output progress | 20% growth per iteration for 3+ iterations | Per-issue |
| **No progress** | Agent posting heartbeats but no file changes | 3 consecutive heartbeats, 0 file changes | Per-agent |
| **Provider churn** | Rapid provider switching on same task | 3+ provider switches in 10 min | Per-issue |

### Detection Algorithm

```go
// RunawayDetector monitors for cost runaway patterns.
type RunawayDetector struct {
    store        CostStore
    config       RunawayConfig
    costCtrl     *CostController

    // Per-issue tracking
    issueRetries map[int]*retryTracker
    issueBounces map[int]int
    issueContext map[int]*contextTracker

    // System-wide tracking
    spendHistory []spendSample
    spawnHistory []spawnSample
}

// retryTracker tracks retry patterns for a single issue.
type retryTracker struct {
    attempts    []time.Time
    lastTokens  int   // input tokens on last attempt
    providers   []string
}

// Evaluate checks all runaway signals and returns any triggered alerts.
func (rd *RunawayDetector) Evaluate(ctx context.Context, event CostEvent) []RunawayAlert {
    var alerts []RunawayAlert

    // Signal 1: Retry frequency
    if alert := rd.checkRetryFrequency(event); alert != nil {
        alerts = append(alerts, *alert)
    }

    // Signal 2: QC bounce count
    if alert := rd.checkQCBounces(event); alert != nil {
        alerts = append(alerts, *alert)
    }

    // Signal 3: Cost velocity
    if alert := rd.checkCostVelocity(event); alert != nil {
        alerts = append(alerts, *alert)
    }

    // Signal 4: Spawn rate
    if alert := rd.checkSpawnRate(event); alert != nil {
        alerts = append(alerts, *alert)
    }

    // Signal 5: Context growth
    if alert := rd.checkContextGrowth(event); alert != nil {
        alerts = append(alerts, *alert)
    }

    // Signal 6: No progress
    if alert := rd.checkNoProgress(event); alert != nil {
        alerts = append(alerts, *alert)
    }

    // Signal 7: Provider churn
    if alert := rd.checkProviderChurn(event); alert != nil {
        alerts = append(alerts, *alert)
    }

    return alerts
}
```

### Response Actions

When a runaway is detected, the response depends on severity:

| Severity | Condition | Response |
|----------|-----------|----------|
| **Low** | Single signal, first occurrence | Log warning, add to next digest |
| **Medium** | Single signal, persistent (3+ occurrences) OR two signals simultaneously | Pause the specific issue, notify at check-in |
| **High** | Three+ signals simultaneously OR cost velocity critical | Pause the agent pool, escalate as `needs-human` |
| **Critical** | Spawn cascade detected OR monthly budget at 90% with active runaways | Stop all cloud work, emergency notification |

### Runaway Response State Machine

```text
            MONITORING
                │
        signal detected
                │
                ▼
            ALERTED ──── signal clears ──── MONITORING
                │
        signal persists (3+ occurrences)
        OR 2+ signals simultaneously
                │
                ▼
         ISSUE_PAUSED ──── user resumes ──── MONITORING
                │
        3+ signals OR cost velocity critical
                │
                ▼
         POOL_PAUSED ──── user resumes ──── MONITORING
                │
        spawn cascade OR budget critical
                │
                ▼
         EMERGENCY_STOP ── user resumes ──── MONITORING
```

### Circuit Breaker Integration

The runaway detector uses a three-state circuit breaker per issue and per agent pool, inspired by the ralph-claude-code pattern:

```go
// CircuitBreaker tracks anomaly state for a scope (issue or agent pool).
type CircuitBreaker struct {
    state         CBState
    failureCount  int
    lastFailure   time.Time
    halfOpenUntil time.Time
}

type CBState int

const (
    CBClosed   CBState = iota // Normal operation
    CBOpen                    // Tripped, all calls blocked
    CBHalfOpen                // Testing recovery
)

// Trip opens the circuit breaker.
func (cb *CircuitBreaker) Trip(reason string) {
    cb.state = CBOpen
    cb.failureCount++
    cb.lastFailure = time.Now()
}

// AttemptReset moves to half-open state after cooldown.
func (cb *CircuitBreaker) AttemptReset(cooldown time.Duration) bool {
    if cb.state != CBOpen {
        return false
    }
    if time.Since(cb.lastFailure) >= cooldown {
        cb.state = CBHalfOpen
        cb.halfOpenUntil = time.Now().Add(cooldown / 2)
        return true
    }
    return false
}

// RecordSuccess in half-open state closes the breaker.
func (cb *CircuitBreaker) RecordSuccess() {
    if cb.state == CBHalfOpen {
        cb.state = CBClosed
        cb.failureCount = 0
    }
}
```

Configuration for circuit breaker thresholds:

```yaml
# .samverk/cost.yaml (continued)

runaway_detection:
  # Per-issue thresholds
  max_retries_per_window: 3
  retry_window_minutes: 10
  max_qc_bounces: 3
  max_context_growth_pct: 20
  max_no_progress_heartbeats: 3
  max_provider_switches: 3
  provider_switch_window_minutes: 10

  # System-wide thresholds
  cost_velocity_doubling_minutes: 5
  spawn_rate_multiplier: 2.0
  spawn_rate_window_minutes: 15

  # Circuit breaker cooldown
  cooldown_minutes: 15

  # Emergency stop threshold (% of monthly budget)
  emergency_stop_pct: 90
```

## User Notification Design

### Notification Channels

Samverk uses the check-in digest as the primary notification channel. Additional channels are available for urgent alerts.

| Priority | Channel | Latency | Use Case |
|----------|---------|---------|----------|
| Normal | Check-in digest | Next check-in (hours) | Budget warnings, daily summaries |
| High | Push notification | Minutes | Budget critical, runaway detected |
| Emergency | Push + digest header | Minutes | Emergency stop, monthly cap hit |

Push notifications are implemented via the platform the user accesses Samverk from (web push for browser, OS notification for desktop app, webhook for integrations like Slack/Discord).

### Digest Cost Section (Enhanced)

The existing digest cost line (`Cost: ~$X.XX (Nk tokens) since last check-in | $Y.YY / $Z.ZZ budget`) is expanded:

```text
--- COST STATUS ---

Spend since last check-in: $4.82 (96k tokens, 14h window)
  Breakdown: code-gen $2.90 | QC $1.12 | routing $0.40 | docs $0.40
  Providers: Sonnet $3.20 | Haiku $0.80 | local $0.00 (handled 12 tasks)

Budget status:
  Today:   $18.20 / $25.00 (72%) -- on track
  Month:   $34.50 / $50.00 (69%) -- 11 days remaining, $1.41/day pace

Projections:
  At current pace: $48.70 by month end (within budget)
  If backlog cleared: ~$62.00 (over budget by $12)
  Recommended: defer 3 low-priority tasks to stay within $50

Cost events:
  [!] Issue #67 hit per-task limit ($10.02) -- paused, needs review
  [i] Model downgrade active: Sonnet → Haiku for complexity:local tasks

> "budget raise day 30" to increase daily limit
> "budget raise month 75" to increase monthly limit
> "67 resume" to continue #67 with increased task limit
> "cost details" for per-issue breakdown
```

### Budget Alert Notification

For out-of-band notifications (push), the message is concise:

```text
[Samverk] Budget alert: Daily spend at 90% ($22.50/$25.00).
3 tasks paused. Local agents continuing.
Check in to review or adjust: samverk.dev/checkin
```

### Emergency Stop Notification

```text
[Samverk] EMERGENCY: Monthly budget hit ($50.02/$50.00).
All cloud work stopped. 2 tasks committed partial results.
Local agents paused (no cloud QC available).
Resume at: samverk.dev/checkin
```

## Realistic Cost Projections

### Tier 2 User: One Cloud Provider + Local ($50/month)

Assume 20 working days per month, 8 hours of agent work per day while user is away.

**Workload:** 3-5 feature issues per week, each requiring code gen + test + QC + docs.

| Activity | Tasks/Month | Avg Cost/Task | Monthly Cost |
|----------|-------------|--------------|-------------|
| Feature implementation (Sonnet) | 15 | $0.35 | $5.25 |
| Test generation (Sonnet) | 15 | $0.18 | $2.70 |
| QC review (Sonnet, 1.5 passes avg) | 22 | $0.12 | $2.64 |
| Bug fixes (Sonnet) | 8 | $0.23 | $1.84 |
| Architecture/planning (Sonnet) | 4 | $0.24 | $0.96 |
| Dispatcher routing (Haiku) | 60 | $0.02 | $1.20 |
| Documentation (Haiku) | 10 | $0.04 | $0.40 |
| **Cloud subtotal** | | | **$14.99** |
| Local model tasks (code gen, tests, formatting) | 40 | $0.00 | $0.00 |
| **Total** | | | **$14.99** |

**Finding:** The $50/month budget is generous for Tier 2 with local offloading. A user would need to push 3x this workload to approach the limit. The budget provides a comfortable safety margin for runaway scenarios.

**With no local models (cloud only):**

| Activity | Tasks/Month | Avg Cost/Task | Monthly Cost |
|----------|-------------|--------------|-------------|
| All tasks on Sonnet | 134 | ~$0.22 avg | $29.48 |
| Dispatcher on Haiku | 60 | $0.02 | $1.20 |
| **Total** | | | **$30.68** |

Still within $50. The buffer shrinks to $19.32 for runaways.

### Tier 3 User: Multiple Cloud Providers ($50-150/month)

Cross-model validation doubles QC costs but improves quality.

| Activity | Tasks/Month | Avg Cost/Task | Monthly Cost |
|----------|-------------|--------------|-------------|
| Feature implementation (Sonnet) | 20 | $0.35 | $7.00 |
| Cross-model QC (Sonnet + GPT-4o) | 30 | $0.22 | $6.60 |
| Architecture (Opus) | 6 | $0.48 | $2.88 |
| Bug fixes (Sonnet) | 12 | $0.23 | $2.76 |
| Test generation (local) | 20 | $0.00 | $0.00 |
| Dispatcher (Haiku) | 80 | $0.02 | $1.60 |
| Documentation (local) | 15 | $0.00 | $0.00 |
| **Total** | | | **$20.84** |

Tier 3 users running more aggressively with Opus for architecture decisions: ~$40-60/month.

### Worst-Case Runaway Scenario (Unprotected)

Without cost control, a single bad night:

```text
21:00 - User goes to sleep. 5 tasks queued.
21:15 - Task #42 enters retry loop (compilation error). Burns $3.80 in 8 min.
21:25 - Task #43 enters QC disagreement cycle. Burns $2.75 in 25 min.
21:50 - Task #42 spawns 4 sub-tasks. Each starts analysis. $0.96.
22:15 - Sub-tasks of #42 also fail, spawn more. $3.84 (16 tasks).
22:45 - Provider rate limit. Failover cascade on 3 tasks. $2.94.
23:30 - Spawn cascade continues. 64 open tasks. $15.36.
01:00 - Context exhaustion loops on 5 parallel tasks. $16.00.

Total by 07:00: ~$82 in one night
```

**With cost control (this design):**

```text
21:00 - User goes to sleep. 5 tasks queued.
21:15 - Task #42 retry detected at attempt 3. Circuit breaker trips.
         Cost so far: $1.05. Task paused. Other tasks continue.
21:25 - Task #43 QC bounce detected at bounce 3. Paused. $1.41.
21:50 - Spawn rate alert: 4 tasks opened, 0 closed in 15 min. Pool throttled.
22:00 - Daily spend at $4.50. 3 tasks paused, 2 healthy tasks continue on local.
07:00 - User check-in. Digest shows 3 paused tasks, $6.20 spent.
         User reviews, adjusts approach, resumes.

Total by 07:00: $6.20 (vs $82 unprotected)
```

## Local Fallback Strategy

When cloud budget is exhausted, local models continue working. Not all tasks can degrade gracefully.

### Task Compatibility Matrix

| Task Type | Local Capable? | Quality Impact | Fallback Behavior |
|-----------|---------------|---------------|-------------------|
| Code generation (simple) | Yes | Minor -- may need more iterations | Route to Qwen 3 8B or Codestral |
| Code generation (complex) | Partial | Significant -- architecture may be poor | Queue for cloud, continue simpler tasks |
| Test generation | Yes | Minor -- tests may be less creative | Route to local |
| QC review | Partial | Moderate -- fewer subtle issues caught | Local pre-screen, queue for cloud final review |
| Bug fix (simple) | Yes | Minor | Route to local |
| Bug fix (complex) | No | Major -- may misdiagnose | Queue for cloud |
| Architecture/planning | No | Critical -- local models lack reasoning depth | Queue for cloud, do not attempt locally |
| Documentation | Yes | Minimal | Route to local |
| Dispatcher routing | Yes (Haiku-level) | Minimal | Route to local classifier |
| Formatting/linting | Yes | None | Always local anyway |

### Fallback Decision Logic

```go
// CanFallbackLocal determines if a task can run on local models.
func CanFallbackLocal(task TaskMeta) FallbackDecision {
    // Architecture and planning: never local
    if task.AgentType == AgentTypeOrchestrator {
        return FallbackDecision{CanFallback: false, Reason: "orchestration requires cloud reasoning"}
    }

    // Complex tasks: only if user has capable local hardware
    if task.Complexity == ComplexityCloud {
        return FallbackDecision{CanFallback: false, Reason: "task complexity exceeds local capability"}
    }

    // QC: partial -- local can pre-screen
    if task.AgentType == AgentTypeQC {
        return FallbackDecision{
            CanFallback:  true,
            Degraded:     true,
            Reason:       "local QC pre-screen only; cloud review queued",
        }
    }

    // Everything else: fallback OK
    return FallbackDecision{CanFallback: true, Degraded: false}
}
```

### Budget Recovery Flow

When the budget window resets (daily at midnight UTC, monthly on the 1st):

1. Cost controller transitions from `LOCAL_ONLY` or `STOPPED` to `NORMAL`
2. All `status:budget-hold` issues transition to `status:queued`
3. Dispatcher picks up queued issues in priority order
4. Cloud-queued tasks (architecture, complex QC) get priority since they waited longest
5. Digest at next check-in notes: "Budget reset. N tasks resumed from budget hold."

If the user explicitly resumes before the window reset:

```text
USER: resume

SAMVERK: Resuming cloud work. Current month spend: $50.02.
This will exceed the $50.00 monthly budget.
> "budget raise month 75" to increase limit
> "resume anyway" to continue without raising (budget warnings disabled until reset)
```

The "resume anyway" option disables budget enforcement for the current window. This is intentional -- the user explicitly chose to exceed their budget, and Samverk respects user agency. Runaway detection remains active regardless of budget state.

## SQLite Schema for Cost Tracking

```sql
-- Per-API-call cost records
CREATE TABLE IF NOT EXISTS cost_usage (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_number  INTEGER NOT NULL,
    agent_type    TEXT NOT NULL,
    provider      TEXT NOT NULL,
    model         TEXT NOT NULL,
    input_tokens  INTEGER NOT NULL,
    output_tokens INTEGER NOT NULL,
    input_rate    REAL NOT NULL,      -- $/token at time of call
    output_rate   REAL NOT NULL,      -- $/token at time of call
    total_cost    REAL NOT NULL,      -- pre-computed for query speed
    cache_hit     BOOLEAN DEFAULT 0,
    batch_mode    BOOLEAN DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Indexes for budget queries
CREATE INDEX IF NOT EXISTS idx_cost_usage_created ON cost_usage(created_at);
CREATE INDEX IF NOT EXISTS idx_cost_usage_issue ON cost_usage(issue_number);
CREATE INDEX IF NOT EXISTS idx_cost_usage_provider ON cost_usage(provider);

-- Runaway detection events
CREATE TABLE IF NOT EXISTS runaway_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_number  INTEGER,            -- NULL for system-wide events
    signal_type   TEXT NOT NULL,       -- retry_loop, qc_bounce, cost_velocity, etc.
    severity      TEXT NOT NULL,       -- low, medium, high, critical
    details       TEXT NOT NULL,       -- JSON blob with signal-specific data
    action_taken  TEXT NOT NULL,       -- logged, paused, stopped, etc.
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Budget state transitions (audit log)
CREATE TABLE IF NOT EXISTS budget_transitions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    from_state    TEXT NOT NULL,
    to_state      TEXT NOT NULL,
    trigger       TEXT NOT NULL,       -- per_day_limit, user_resume, window_reset, etc.
    details       TEXT,                -- JSON with context
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
```

### Cost Query Helpers

```go
// DailySpend returns total cost for the rolling 24-hour window.
func (s *CostStore) DailySpend(ctx context.Context) (float64, error) {
    var total float64
    err := s.db.QueryRowContext(ctx,
        `SELECT COALESCE(SUM(total_cost), 0)
         FROM cost_usage
         WHERE created_at >= strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '-24 hours')`,
    ).Scan(&total)
    return total, err
}

// MonthlySpend returns total cost for the current calendar month.
func (s *CostStore) MonthlySpend(ctx context.Context) (float64, error) {
    var total float64
    err := s.db.QueryRowContext(ctx,
        `SELECT COALESCE(SUM(total_cost), 0)
         FROM cost_usage
         WHERE created_at >= strftime('%Y-%m-01T00:00:00Z', 'now')`,
    ).Scan(&total)
    return total, err
}

// TaskSpend returns total cost for a specific issue.
func (s *CostStore) TaskSpend(ctx context.Context, issueNumber int) (float64, error) {
    var total float64
    err := s.db.QueryRowContext(ctx,
        `SELECT COALESCE(SUM(total_cost), 0)
         FROM cost_usage
         WHERE issue_number = ?`,
        issueNumber,
    ).Scan(&total)
    return total, err
}

// BurnRate returns average hourly spend over a window.
func (s *CostStore) BurnRate(ctx context.Context, window time.Duration) (float64, error) {
    windowStr := fmt.Sprintf("-%d seconds", int(window.Seconds()))
    var total float64
    err := s.db.QueryRowContext(ctx,
        `SELECT COALESCE(SUM(total_cost), 0)
         FROM cost_usage
         WHERE created_at >= strftime('%Y-%m-%dT%H:%M:%fZ', 'now', ?)`,
        windowStr,
    ).Scan(&total)
    if err != nil {
        return 0, err
    }
    hours := window.Hours()
    if hours == 0 {
        return 0, nil
    }
    return total / hours, nil
}
```

## Integration Points

### With Autonomy Model

The per-call threshold (`per_call_usd: $5.00`) is the existing `api_cost_threshold_usd` from `autonomy.yaml`. When a single API call would exceed this, it becomes a Tier 3 action requiring user confirmation. Cost control does not replace the autonomy model -- it wraps around it with additional budget-level enforcement.

### With Dispatcher

The dispatcher calls `CostController.PreApprove()` before routing any task to a cloud agent. The decision flow:

```text
Dispatcher receives queued issue
  → Parse estimated_tokens from frontmatter
  → CostController.PreApprove(estimatedCost)
     → Approved: route to cloud agent
     → FallbackLocal: route to local agent (if capable)
     → StopAll: leave issue queued, add status:budget-hold
```

### With Check-in Digest

`DigestData.Cost` is replaced with an expanded `CostReport`:

```go
type CostReport struct {
    // Spend since last check-in
    SinceLastCheckIn    float64
    TokensSinceLastCI   int
    WindowDuration      time.Duration

    // Breakdown by dimension
    ByAgentType         map[string]float64
    ByProvider          map[string]float64
    LocalTasksHandled   int

    // Budget status
    DailySpend          float64
    DailyBudget         float64
    MonthlySpend        float64
    MonthlyBudget       float64

    // Projections
    ProjectedMonthEnd   float64
    DaysRemaining       int
    PacePerDay          float64

    // Events requiring attention
    PausedTasks         []PausedTask
    RunawayAlerts       []RunawayAlert
    BudgetState         BudgetState
    ModelDowngradeActive bool
}
```

### With Provider Layer

Every provider client (`internal/provider/`) wraps API calls with cost tracking:

```go
// Before call
decision, _ := costCtrl.PreApprove(ctx, CostRequest{
    IssueNumber:           issueNum,
    Provider:              "anthropic",
    Model:                 "claude-sonnet-4-6",
    EstimatedInputTokens:  len(prompt) / 4, // rough estimate
    EstimatedOutputTokens: maxTokens,
    InputRate:             3.0 / 1_000_000,
    OutputRate:            15.0 / 1_000_000,
})
if !decision.Approved {
    return decision, nil
}

// Make API call
response, err := client.Complete(ctx, prompt, maxTokens)

// After call
costCtrl.Record(ctx, CostUsage{
    IssueNumber:  issueNum,
    InputTokens:  response.Usage.InputTokens,
    OutputTokens: response.Usage.OutputTokens,
    InputRate:    3.0 / 1_000_000,
    OutputRate:   15.0 / 1_000_000,
    CacheHit:     response.Usage.CacheCreationInputTokens > 0,
})
```

## Design Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Monthly cap type | Hard | Users set this as their absolute maximum. Respecting it builds trust. |
| Daily cap type | Soft (local fallback) | Daily fluctuations are normal. Hard-stopping every evening is disruptive. |
| Per-task cap type | Soft (local fallback) | One expensive task should not stop all other tasks. |
| Runaway detection | Predictive (pattern-based) | Waiting for budget exhaustion is too late for async systems. |
| Circuit breaker | Per-issue + per-pool | Issue-level prevents one bad task from affecting others. Pool-level catches systemic issues. |
| Local fallback | Automatic for compatible tasks | Maximizes work done per dollar. User sees more progress. |
| Budget recovery | Explicit resume OR window reset | Prevents surprising the user with unexpected charges after a cap. |
| Cost attribution | 4 dimensions (issue, agent, provider, model) | Enables user to understand WHERE money goes and make informed trade-offs. |

## Recommended ADR

### ADR-025: Cost Control -- Hard Caps with Graceful Degradation

**Status:** Proposed

**Date:** 2026-03-01

**Context:**

Samverk agents work autonomously while users are away. The async-first architecture (ADR-006) means cost overruns happen without user awareness. The autonomy model (ADR-015) gates individual expensive API calls at Tier 3, but does not address cumulative budget enforcement, runaway detection, or graceful degradation when budgets are exhausted.

Users need confidence that they will not wake up to a $200 bill. This confidence is foundational to the check-in model -- if users feel they must monitor agents in real-time to control costs, the async value proposition collapses.

**Decision:**

Implement four-level budget enforcement (per-call, per-task, per-day, per-month) with mixed hard/soft caps. Monthly budgets are hard caps (all cloud work stops). Daily and per-task budgets are soft caps (degrade to local-only). Per-call budgets integrate with the existing Tier 2/3 autonomy boundary.

Implement predictive runaway detection using seven signals (retry frequency, QC bounces, cost velocity, spawn rate, context growth, no progress, provider churn) with circuit breakers per issue and per agent pool.

When cloud budgets are exhausted, automatically fall back to local models for compatible tasks. Queue incompatible tasks for the next budget window.

**Consequences:**

- Users can set a monthly budget and trust it will not be exceeded
- Agents continue productive work on local models when cloud budget is exhausted
- Runaway scenarios are detected and stopped before they exhaust the budget
- Cost is tracked at four dimensions enabling fine-grained analysis in the digest
- The CostController becomes a dependency of every provider client, adding one function call per API request
- Local model quality determines how much useful work happens during budget-constrained periods
- Users who want to exceed their budget can do so explicitly ("resume anyway")

**Alternatives Considered:**

1. **Soft caps only (notify but never stop):** Rejected. Users explicitly setting a $50 budget expect it to be respected. "We told you" is not acceptable.
2. **Hard caps only (stop everything):** Rejected. Stopping all work including local models wastes available compute. Local models are free -- there is no reason to idle them.
3. **Cloud-only cost model (no local fallback):** Rejected. Contradicts the tiered cost model (ADR cost-model) where local handles volume.
4. **Budget enforcement in the autonomy model only:** Rejected. The autonomy model gates individual actions. Budget enforcement is cumulative and requires its own state machine.

## Related Documents

- [Cost Model](cost-model.md) -- Tiered cost structure and work distribution
- [Autonomy Model](autonomy-model.md) -- Trust tiers and per-action cost thresholds
- [Check-in Digest Design](check-in-digest-design.md) -- How cost information is presented to users
- [Digest Data Schema](digest-data-schema.md) -- Go types for cost summary in digest
- [Dispatcher Design](dispatcher-design.md) -- Budget guards in routing decisions
- [System Requirements](system-requirements.md) -- Hardware tiers affecting local fallback capability
- [Ollama Orchestration](ollama-orchestration.md) -- Local model deployment
- [ADR-006: Async-First Architecture](decisions/ADR-006-async-first.md)
- [ADR-015: Three-Tier Autonomy Model](decisions/ADR-015-three-tier-autonomy.md)
