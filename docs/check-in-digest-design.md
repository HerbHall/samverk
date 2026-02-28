# Check-in Digest Design

## Purpose and Scope

The check-in digest is the primary surface through which users interact with Samverk during their limited availability windows. It answers three questions in order of urgency:

1. **What needs my decision?** (Tier 3 pending actions -- blocking work)
2. **What happened while I was away?** (Tier 2 completed actions -- review and possible override)
3. **What is the overall state of my project?** (Active work, backlog, cost)

The target interaction time is 5-15 minutes. The digest is delivered as a conversational exchange through the front-end agent (Claude + MCP), not as a GUI dashboard. Every line is either actionable (requires a response) or informational (helps the user make decisions). No filler.

### Who This Is For

A hobbyist developer checking in on their phone during a lunch break, or sitting at their desk with 10 minutes before dinner. They need to unblock their agents quickly, spot anything that went wrong, and set direction for the next work cycle.

## Digest Data Model

The front-end agent assembles the digest by querying the `IssueTracker` interface. Each section maps to specific method calls.

### Tier 3 Pending Actions

Issues where agents are blocked waiting for user approval.

**Query:** `ListIssues(ctx, &ListOptions{State: StateOpen, Labels: ["status:needs-human"]})`

**Data extracted per issue:**

| Field | Source | Purpose |
|-------|--------|---------|
| Issue number | `Issue.Number` | Reference ID for quick-action commands |
| Title | `Issue.Title` | One-line description of the pending action |
| Action type | Parsed from `Issue.Body` frontmatter `type: block` | What the agent wants to do |
| Parent issue | Parsed from `Issue.Body` frontmatter `parent_issue` | What work stream this belongs to |
| Priority | Parsed from label `priority:*` on `Issue.Labels` | Sorting order |
| Context | `Issue.Body` Context section | Why the agent needs this, what is affected |
| Blocked dependents | `ListIssues(ctx, &ListOptions{State: StateOpen, Labels: ["status:blocked"]})` filtered by `depends_on` referencing this issue | How many work streams are waiting |
| Time waiting | `time.Since(Issue.CreatedAt)` | Urgency signal |

**Sort order:** `priority:critical` first, then `priority:high`, then by `CreatedAt` ascending (longest-waiting first within the same priority).

### Tier 2 Completed Actions

Actions agents took autonomously since the last check-in, surfaced for awareness and possible override.

**Query:** `ListIssues(ctx, &ListOptions{State: StateClosed})` filtered client-side to issues closed after the last check-in timestamp.

Additionally, Tier 2 actions are logged as individual comments on their parent task issues. The front-end agent queries comments on recently active issues:

**Query per active parent:** `ListComments(ctx, parentNumber)` filtered to comments containing the `## Tier 2 Action:` header and posted after the last check-in.

**Data extracted per action:**

| Field | Source | Purpose |
|-------|--------|---------|
| Action description | Comment body, `## Tier 2 Action:` header | What was done |
| Files affected | Comment body, `Lines affected` field | Scope of change |
| Reversibility | Comment body, `Reversible` field | Whether undo is possible |
| Parent issue | The issue the comment is on | Work stream context |
| Timestamp | `Comment.CreatedAt` | Grouping by time window |

### Active Work

Issues currently being worked on by agents.

**Query:** `ListIssues(ctx, &ListOptions{State: StateOpen, Labels: ["status:in-progress"]})`

### Backlog Depth

Count of issues waiting to be picked up.

**Query:** `ListIssues(ctx, &ListOptions{State: StateOpen, Labels: ["status:queued"]})`

The front-end agent reports `len(result)` as the backlog count.

### Cost Summary

Token usage since the last check-in, extracted from issue frontmatter.

**Source:** Closed issues with `actual_tokens` and `model_used` fields populated in their frontmatter. The front-end agent sums `actual_tokens` across all issues closed since the last check-in and applies the per-model cost rate.

### Blocked Work

Issues waiting on dependencies (not on user input).

**Query:** `ListIssues(ctx, &ListOptions{State: StateOpen, Labels: ["status:blocked"]})`

These are reported separately from Tier 3 items because they are blocked on other work, not on user decisions.

## Conversational Format

The digest is a structured text message from the front-end agent. Sections appear in priority order, and each section uses a consistent format that supports quick-action responses.

### Section 1 -- Tier 3 Decisions (Blocking)

Presented first because these are directly blocking agent work.

Format per item:

```text
[N] ACTION_TYPE: title
    Why: one-line rationale
    Blocks: N dependent issues
    Waiting: Xh
    > approve | reject | ? (more context)
```

### Section 2 -- Tier 2 Review (Awareness)

Grouped by time window (today, yesterday, earlier) when the check-in gap is long.

Format per item:

```text
- EDIT internal/dispatcher/router.go (L45-89): refactored routeTask() to label-based matching
```

One line per action. Expandable on request.

### Section 3 -- Status Summary

```text
Active: N issues in progress
Queued: N issues waiting
Blocked: N issues (dependency, not user)
Cost: ~$X.XX (Nk tokens) since last check-in | $Y.YY / $Z.ZZ budget
```

### Full Example Conversation

The following shows a realistic check-in with 3 Tier 3 items and 5 Tier 2 actions.

```text
SAMVERK: Welcome back. You've been away 14h. Here's where things stand.

--- NEEDS YOUR DECISION (3 items, blocking work) ---

[1] merge_main: Merge PR #52 (Gitea IssueTracker) to main
    Why: QC passed, all 14 tests green, code-gen agent considers it ready.
    Blocks: 2 issues (#55 webhook setup, #56 Gitea polling)
    Waiting: 6h
    > 1 approve | 1r reject | 1? more context

[2] add_dependency: Add code.gitea.io/sdk/gitea v0.19.0
    Why: Required for Gitea IssueTracker implementation in #50.
    Blocks: 1 issue (#50)
    Waiting: 8h
    > 2 approve | 2r reject | 2? more context

[3] modify_ci: Add Gitea integration test job to CI workflow
    Why: New adapter needs CI coverage. Adds a Gitea container service to the
    test matrix.
    Blocks: 0 issues (QC will require this before closing #50)
    Waiting: 3h
    > 3 approve | 3r reject | 3? more context

--- COMPLETED AUTONOMOUSLY (5 actions since last check-in) ---

Today:
- EDIT internal/forge/github/github.go (L120-145): added retry logic for rate-limited API calls
- CREATE_PR #53: "feat: optimistic locking for task claiming"
- CLOSE #47: sub-task "research Gitea webhook signature validation" -- result in comments
- PUSH feature/issue-50-gitea-tracker: 3 commits (Gitea adapter core methods)

Yesterday:
- EDIT internal/dispatcher/router.go (L45-89): refactored routeTask() to label-based matching

--- STATUS ---

Active: 3 issues in progress (#50 Gitea adapter, #51 dispatcher routing, #54 test harness)
Queued: 7 issues waiting
Blocked: 2 issues (waiting on #50 and #52)
Cost: ~$1.42 (28k tokens) since last check-in | $12.80 / $50.00 budget

What would you like to do?
```

```text
USER: 1 2

SAMVERK: Approved:
  [1] Merging PR #52 to main -- dispatcher notified, #55 and #56 unblocked.
  [2] Adding gitea SDK v0.19.0 -- agent resuming #50.

2 decisions remain. Anything else?
```

```text
USER: 3?

SAMVERK: [3] modify_ci: Add Gitea integration test job

The code-gen agent wants to add a new job to .github/workflows/ci.yml:

  gitea-integration:
    runs-on: ubuntu-latest
    services:
      gitea:
        image: gitea/gitea:1.22
        ports: ["3000:3000"]
    steps:
      - uses: actions/checkout@v4
      - run: go test -tags=integration ./internal/forge/gitea/...

This adds ~2min to CI runtime. No secrets are exposed -- the Gitea container
uses default credentials for testing only.

> 3 approve | 3r reject
```

```text
USER: 3 approve. Also, focus on getting the dispatcher routing solid before
starting new features.

SAMVERK: Approved:
  [3] CI modification approved -- agent will update workflow.

Direction noted: prioritizing dispatcher routing (#51) over new feature work.
I've bumped #51 to priority:high and moved 3 queued feature issues to
priority:low. The code-gen agent will finish #50 (Gitea, in progress) and
then focus on #51.

All Tier 3 items resolved. Anything else, or should I carry on?
```

```text
USER: undo 1

SAMVERK: Creating revert task for Tier 2 action 1 (retry logic in
github.go L120-145). I'll open an issue to revert that change and
re-run affected tests. The revert will go through normal Tier 2
flow -- you'll see it in your next digest.

Anything else?
```

```text
USER: That's it. Carry on.

SAMVERK: Signed off. 3 Tier 3 approvals applied, 1 revert queued,
dispatcher priority adjusted. Next check-in will cover progress on
#50 and #51. See you later.
```

## Interaction Flow

### Phase 1 -- Tier 3 Decisions

The digest always starts here when Tier 3 items exist.

1. Front-end agent presents all `status:needs-human` issues in priority order
2. User responds with quick-actions (numbers, approve/reject) or asks for more context
3. For each approval, front-end agent calls forge operations (see Response-to-Action Translation)
4. For each rejection, front-end agent records the reason and creates an alternative path issue
5. Phase completes when all Tier 3 items are addressed or user explicitly defers (`skip`)

### Phase 2 -- Tier 2 Review

After Tier 3 items are resolved (or if none exist), the digest shifts to awareness.

1. Front-end agent presents Tier 2 actions grouped by time window
2. User can request detail on any item by number
3. User can undo any item by number (`undo N`)
4. If user has no concerns, a simple "looks good" or "next" advances to Phase 3

### Phase 3 -- Status and Direction

1. Front-end agent presents active/queued/blocked/cost summary
2. User can reprioritize ("focus on X"), add constraints ("hold off on new features"), or ask questions about specific issues
3. Front-end agent translates direction into label changes and priority adjustments

### Phase 4 -- Sign-off

1. Front-end agent summarizes all actions taken during this check-in
2. Confirms what will happen next (which agents will resume, what work starts)
3. User dismisses with "carry on", "that's it", or similar

The user can skip any phase or address phases out of order. The flow is a default, not a constraint. Saying "approve all" in Phase 1 immediately advances to Phase 2.

## Device Adaptations

The digest is plain text, so it works on any device. Adaptations are about information density, not rendering.

### Desktop (Full Digest)

All sections shown by default. Tier 2 items include file paths and line numbers. Code diffs available inline when user requests detail (`N?`). Full status summary with per-agent breakdown if requested.

### Phone (Compact Digest)

The front-end agent detects the device context (screen width hint from the client, or user preference in profile).

**Default view:**

- Tier 3 items (always shown -- these block work)
- Cost summary (one line)
- Count-only status: "3 active, 7 queued, 2 blocked"

**Omitted by default (available on request):**

- Tier 2 action list ("show tier 2" or "show changes")
- File paths and line numbers
- Per-agent status breakdown

**Interaction:**

- Same quick-action syntax (`1`, `1r`, `1?`)
- "show more" expands Tier 2 section
- "details N" shows full context for item N

### Tablet (Intermediate)

Full digest shown, but Tier 2 section is collapsed to count-only by default:

```text
--- COMPLETED AUTONOMOUSLY (5 actions since last check-in) ---
[expand] 3 file edits, 1 PR created, 1 issue closed
```

User types "expand" or "show changes" to see the full list.

## Response-to-Action Translation

User responses in the chat are translated by the front-end agent into specific `IssueTracker` method calls.

### Tier 3 Approval Actions

| User Input | Forge Operations |
|------------|-----------------|
| `N` or `N approve` or `approve N` | `AddComment(ctx, issueNum, "Approved by user at check-in.")` then `RemoveLabel(ctx, issueNum, "status:needs-human")` then `AddLabel(ctx, issueNum, "status:queued")` |
| `N reject` or `Nr` | `AddComment(ctx, issueNum, "Rejected by user: [reason].")` then `RemoveLabel(ctx, issueNum, "status:needs-human")` then `AddLabel(ctx, issueNum, "status:blocked")` |
| `N?` or `details N` | `GetIssue(ctx, issueNum)` then `ListComments(ctx, issueNum)` -- present full context |
| `approve all` | Loops over all Tier 3 items, applies approval sequence to each |
| `skip N` or `defer N` | `AddComment(ctx, issueNum, "Deferred by user at check-in.")` -- no label change, stays `needs-human` |

### Tier 2 Override Actions

| User Input | Forge Operations |
|------------|-----------------|
| `undo N` | `CreateIssue(ctx, &CreateIssueRequest{Title: "Revert: [original action]", Body: revertContext, Labels: ["status:queued", "agent:code-gen", "priority:high"]})` |
| `undo N because [reason]` | Same as above, with reason included in issue body Context section |

### Direction-Setting Actions

| User Input | Forge Operations |
|------------|-----------------|
| `focus on X` or `prioritize X` | For each issue matching X: `RemoveLabel(ctx, num, "priority:normal")` then `AddLabel(ctx, num, "priority:high")`. For unrelated queued issues: demote to `priority:low` |
| `hold off on [feature]` | `AddComment(ctx, num, "User requested hold.")` then `RemoveLabel(ctx, num, "status:queued")` then `AddLabel(ctx, num, "status:blocked")` |
| `pause everything` | For each `status:in-progress` issue: `AddComment(ctx, num, "Paused by user.")` then label swap to `status:blocked` |
| `resume` | Reverses hold/pause: restore `status:queued` labels |

### Sign-off Actions

| User Input | Forge Operations |
|------------|-----------------|
| `carry on` / `that's it` / `done` | No forge operations. Front-end agent logs check-in timestamp for next digest window calculation |

## Edge Cases

### No Tier 3 Items

Skip directly to Tier 2 review. The digest opens with:

```text
SAMVERK: Welcome back (8h away). No decisions needed -- agents are unblocked.

--- COMPLETED AUTONOMOUSLY (3 actions since last check-in) ---
...
```

### First Check-in Ever

No history exists. The digest becomes an onboarding flow:

```text
SAMVERK: Welcome to Samverk. This is your first check-in.

No agents have run yet. To get started:
1. Tell me about your project and what you're working on
2. I'll create the first set of task issues
3. Agents will pick them up and start working

Or if you've already set up issues, say "start" and I'll begin
dispatching.
```

### Long Absence (Greater Than 24h)

Tier 2 items are grouped by day to prevent information overload:

```text
--- COMPLETED AUTONOMOUSLY (23 actions over 3 days) ---

Today (4 actions):
- EDIT internal/forge/github/github.go: rate limit retry
- CREATE_PR #53: optimistic locking
...

Yesterday (8 actions):
- [summary] 5 file edits, 2 PRs created, 1 issue closed
  Type "yesterday details" for full list

3 days ago (11 actions):
- [summary] 7 file edits, 3 PRs created, 1 dependency added (Tier 3 pending)
  Type "3d details" for full list
```

For absences longer than 7 days, the digest further compresses:

```text
--- COMPLETED AUTONOMOUSLY (67 actions over 12 days) ---

Most important:
- 3 PRs merged to main (Tier 3 approved by auto-policy)
- 2 new dependencies added
- 12 issues closed

Type "full history" for day-by-day breakdown.
```

### Conflicting Actions (Two Agents Touched the Same File)

When the front-end agent detects multiple Tier 2 edits to the same file, it flags the conflict:

```text
- EDIT internal/dispatcher/router.go (L45-89): label-based matching [agent:code-gen]
- EDIT internal/dispatcher/router.go (L102-115): timeout handling [agent:code-gen]
  WARNING: 2 agents edited the same file. Changes may conflict.
  Type "conflict 1" to see both diffs side by side.
```

If the agents created separate commits on the same branch, the front-end agent checks whether a merge conflict exists in git and reports it:

```text
  CONFLICT DETECTED: both edits touch router.go. Manual resolution needed.
  Type "resolve" for suggested merge, or "revert N" to undo one edit.
```

### Budget Exceeded

When token spend since the last check-in exceeds the configured `api_cost_threshold_usd`:

```text
--- BUDGET ALERT ---

Cost since last check-in: $7.82 (156k tokens)
Budget: $5.00 threshold exceeded by $2.82

Agents have been paused. Approve to continue or adjust budget.
> "resume" to continue with current budget
> "budget $10" to raise threshold
> "pause" to keep agents stopped until next check-in
```

The cost threshold is read from `AutonomyPolicy.CostThreshold()` (default `$5.00` from `DefaultCostThresholdUSD`). When exceeded, the agent treats further API calls as `api_call_expensive` (Tier 3), which requires confirmation.

### Tier 3 Item With Expired Context

If a Tier 3 item has been waiting longer than 48h, the front-end agent warns that the context may be stale:

```text
[2] add_dependency: Add gitea SDK v0.19.0
    Why: Required for Gitea IssueTracker implementation in #50.
    Blocks: 1 issue (#50)
    Waiting: 52h
    STALE: Context may be outdated. Type "2?" for fresh analysis.
```

The `2?` command re-reads the issue and its comments to provide current context rather than the original request.

## Implementation Considerations

### Front-end Agent Requirements

The front-end agent is a Claude instance with MCP tools that connect to the Samverk server. It needs:

1. **IssueTracker access** -- All 12 methods from the `IssueTracker` interface, exposed as MCP tools. The agent calls `ListIssues`, `GetIssue`, `ListComments`, `AddComment`, `SetLabels`, `AddLabel`, `RemoveLabel`, and `CreateIssue` during a typical check-in.

2. **Last check-in timestamp** -- Stored in the user profile (SQLite `store` layer). Used to filter "since last check-in" for Tier 2 actions and cost calculations.

3. **Issue body parser** -- The agent must parse YAML frontmatter from issue bodies to extract `type`, `parent_issue`, `depends_on`, `actual_tokens`, `model_used`, and other structured fields.

4. **Cost calculation** -- Map `model_used` to per-token cost rates. Sum `actual_tokens * rate` across all closed issues since the last check-in. Compare against `AutonomyPolicy.CostThreshold()`.

5. **Device context** -- A hint from the client about screen size or device type, used to select compact vs full digest format. Falls back to full format if unknown.

### Required API Surface (MCP Tools)

The front-end agent needs these MCP tools to execute the digest flow:

| MCP Tool | Maps To | Used For |
|----------|---------|----------|
| `samverk_list_issues` | `IssueTracker.ListIssues` | Querying Tier 3, Tier 2, active, queued, blocked issues |
| `samverk_get_issue` | `IssueTracker.GetIssue` | Fetching full context for `N?` detail requests |
| `samverk_list_comments` | `IssueTracker.ListComments` | Reading Tier 2 action logs and discussion history |
| `samverk_add_comment` | `IssueTracker.AddComment` | Recording approvals, rejections, deferrals |
| `samverk_add_label` | `IssueTracker.AddLabel` | Setting status labels after user decisions |
| `samverk_remove_label` | `IssueTracker.RemoveLabel` | Clearing `needs-human` on approval |
| `samverk_set_labels` | `IssueTracker.SetLabels` | Bulk label updates for reprioritization |
| `samverk_create_issue` | `IssueTracker.CreateIssue` | Creating revert tasks from `undo` commands |
| `samverk_get_profile` | User profile store | Last check-in timestamp, device preferences |
| `samverk_update_profile` | User profile store | Recording current check-in timestamp on sign-off |
| `samverk_get_cost_summary` | Cost store | Token usage and budget status |

### Data Structures

The digest assembly requires an intermediate structure to hold the compiled data before the front-end agent formats it conversationally:

```go
// DigestData holds the compiled check-in digest for the front-end agent.
type DigestData struct {
    Tier3Pending    []PendingAction
    Tier2Completed  []CompletedAction
    ActiveWork      []ActiveIssue
    QueuedCount     int
    BlockedCount    int
    CostSummary     CostSummary
    LastCheckIn     time.Time
    TimeSinceLastCI time.Duration
}

// PendingAction represents a Tier 3 item awaiting user decision.
type PendingAction struct {
    IssueNumber      int
    Title            string
    ActionType       ActionType
    Priority         Priority
    ParentIssue      int
    Context          string
    BlockedCount     int
    WaitingDuration  time.Duration
}

// CompletedAction represents a Tier 2 action taken autonomously.
type CompletedAction struct {
    Description  string
    FilesChanged []string
    LinesAffected string
    Reversible   bool
    ParentIssue  int
    Timestamp    time.Time
    AgentType    AgentType
}

// ActiveIssue represents an issue currently being worked on.
type ActiveIssue struct {
    IssueNumber int
    Title       string
    AgentType   AgentType
}

// CostSummary holds token usage and budget information.
type CostSummary struct {
    TokensUsed    int
    EstimatedCost float64
    BudgetTotal   float64
    BudgetUsed    float64
}
```

This structure is assembled server-side by the digest service and served to the front-end agent via the `samverk_get_digest` MCP tool, reducing the number of individual forge API calls the agent needs to make.

### Digest Assembly Optimization

Rather than having the front-end agent make 5-10 separate `ListIssues` calls with different filters, the server should provide a single `samverk_get_digest` endpoint that assembles the `DigestData` in one call. This reduces latency and API rate limit consumption, especially important for GitHub's rate limits.

The individual MCP tools (`samverk_list_issues`, etc.) remain available for follow-up queries during the conversation (e.g., `N?` detail requests, `undo` commands).

## Related Documents

- [Autonomy Model](autonomy-model.md) -- Tier definitions, audit logging, configuration
- [User Interface](user-interface.md) -- Check-in model, device flexibility requirements
- [Communication Protocol](communication-protocol.md) -- Issue schema, label taxonomy, dispatcher behavior
- [User Profile](user-profile.md) -- Persistent preferences including check-in history
- [MCP Server](mcp-server.md) -- MCP tool definitions and transport
- [Cost Model](cost-model.md) -- Token pricing and budget management
