# Digest Data Schema

Structured Go types for assembling the check-in digest. Companion to [check-in-digest-design.md](check-in-digest-design.md), which covers the conversational UX format and interaction flow.

## Go Types

These types live in `internal/digest/` (future). They reference existing enums from `internal/autonomy/` and `pkg/models/`.

```go
// DigestData is the top-level container returned by BuildDigest.
// The front-end agent receives this as a single MCP tool response
// and formats it conversationally.
type DigestData struct {
    PendingActions  []PendingAction  // Tier 3: blocked on user decision
    CompletedActions []CompletedAction // Tier 2: done autonomously since last check-in
    ActiveWork      []ActiveWork     // Currently in progress
    QueuedCount     int              // Backlog depth
    BlockedCount    int              // Blocked on dependencies (not user)
    Cost            CostSummary      // Token spend since last check-in
    LastCheckIn     time.Time        // When user last signed off
}

// PendingAction is a Tier 3 item awaiting user approval or rejection.
type PendingAction struct {
    IssueNumber     int                // Forge issue number (quick-action reference)
    Title           string             // One-line description
    ActionType      autonomy.ActionType // e.g. merge_main, add_dependency
    Priority        models.Priority    // Sorting key: critical > high > normal > low
    Context         string             // Why the agent needs this (from issue body)
    BlockedCount    int                // How many issues depend on this decision
    RequestedAt     time.Time          // When the agent requested approval
}

// CompletedAction is a Tier 2 item the agent executed autonomously.
type CompletedAction struct {
    IssueNumber     int                // Parent issue number
    Title           string             // Action description (one line)
    ActionType      autonomy.ActionType // e.g. edit_file, create_pr, close_issue
    ResultSummary   string             // What changed (files, lines, outcome)
    TokensUsed      int                // Tokens consumed for this action
    ModelUsed       string             // Model identifier (e.g. claude-sonnet-4)
    CompletedAt     time.Time          // When the action finished
}

// ActiveWork is an issue currently being worked by an agent.
type ActiveWork struct {
    IssueNumber     int                // Forge issue number
    Title           string             // Issue title
    AgentType       models.AgentType   // Which agent pool owns it
    ClaimedAt       time.Time          // When work started
    LastHeartbeat   time.Time          // Most recent agent activity
    ProgressPct     int                // 0-100, estimated from subtask completion
}

// CostSummary aggregates token spend since the last check-in.
type CostSummary struct {
    TokensUsed      int     // Total tokens consumed
    EstimatedCostUSD float64 // Calculated from per-model rates
    BudgetRemainingUSD float64 // BudgetTotal - BudgetUsed
    BurnRatePerHour float64 // Tokens/hour averaged over the window
}
```

## Builder Function

`BuildDigest` queries the `IssueTracker` to populate `DigestData` in a single server-side call, avoiding multiple round trips from the front-end agent.

```go
func BuildDigest(ctx context.Context, tracker forge.IssueTracker, store Store, lastCheckIn time.Time) (DigestData, error) {
    var d DigestData
    d.LastCheckIn = lastCheckIn

    // 1. Tier 3 pending actions: status:needs-human, sorted by priority then age
    needsHuman, _ := tracker.ListIssues(ctx, &forge.ListOptions{
        State: forge.StateOpen, Labels: []string{"status:needs-human"},
    })
    for _, iss := range needsHuman {
        fm := parseFrontmatter(iss.Body)
        blocked := countDependents(ctx, tracker, iss.Number)
        d.PendingActions = append(d.PendingActions, PendingAction{
            IssueNumber: iss.Number,
            Title:       iss.Title,
            ActionType:  autonomy.ActionType(fm.Type),
            Priority:    fm.Priority,
            Context:     extractSection(iss.Body, "Context"),
            BlockedCount: blocked,
            RequestedAt: iss.CreatedAt,
        })
    }
    sortByPriorityThenAge(d.PendingActions)

    // 2. Tier 2 completed actions: closed since last check-in
    closed, _ := tracker.ListIssues(ctx, &forge.ListOptions{State: forge.StateClosed})
    for _, iss := range closed {
        if iss.ClosedAt != nil && iss.ClosedAt.After(lastCheckIn) {
            fm := parseFrontmatter(iss.Body)
            d.CompletedActions = append(d.CompletedActions, CompletedAction{
                IssueNumber:   iss.Number,
                Title:         iss.Title,
                ActionType:    autonomy.ActionType(fm.Type),
                ResultSummary: extractSection(iss.Body, "Result"),
                TokensUsed:    fm.ActualTokens,
                ModelUsed:     fm.ModelUsed,
                CompletedAt:   *iss.ClosedAt,
            })
        }
    }

    // 3. Active work: status:in-progress
    active, _ := tracker.ListIssues(ctx, &forge.ListOptions{
        State: forge.StateOpen, Labels: []string{"status:in-progress"},
    })
    for _, iss := range active {
        fm := parseFrontmatter(iss.Body)
        d.ActiveWork = append(d.ActiveWork, ActiveWork{
            IssueNumber:   iss.Number,
            Title:         iss.Title,
            AgentType:     fm.AgentType,
            ClaimedAt:     iss.UpdatedAt, // approximation until heartbeat store exists
            LastHeartbeat: iss.UpdatedAt,
            ProgressPct:   estimateProgress(ctx, tracker, iss.Number),
        })
    }

    // 4. Counts
    queued, _ := tracker.ListIssues(ctx, &forge.ListOptions{
        State: forge.StateOpen, Labels: []string{"status:queued"},
    })
    d.QueuedCount = len(queued)

    blocked, _ := tracker.ListIssues(ctx, &forge.ListOptions{
        State: forge.StateOpen, Labels: []string{"status:blocked"},
    })
    d.BlockedCount = len(blocked)

    // 5. Cost summary from the store layer
    d.Cost = store.ComputeCostSince(ctx, lastCheckIn)

    return d, nil
}
```

Helper functions referenced above are internal to the digest package:

- `parseFrontmatter` -- decodes YAML frontmatter into `models.IssueFrontmatter`
- `extractSection` -- pulls a named markdown section from the issue body
- `countDependents` -- counts open issues whose `depends_on` includes this number
- `estimateProgress` -- ratio of closed sub-issues to total sub-issues under a parent
- `sortByPriorityThenAge` -- critical first, then oldest within the same priority

## Rendering

The front-end agent converts `DigestData` into the conversational format defined in [check-in-digest-design.md](check-in-digest-design.md). The mapping is direct:

| DigestData field | Digest section | Format |
|------------------|---------------|--------|
| `PendingActions` | "NEEDS YOUR DECISION" | Numbered items with approve/reject quick-actions |
| `CompletedActions` | "COMPLETED AUTONOMOUSLY" | One-line per action, grouped by day if gap > 24h |
| `ActiveWork` | "STATUS" active line | Count with issue numbers in parentheses |
| `QueuedCount` | "STATUS" queued line | Plain count |
| `BlockedCount` | "STATUS" blocked line | Count with "(dependency, not user)" note |
| `Cost` | "STATUS" cost line | `~$X.XX (Nk tokens) since last check-in` with budget |

The agent omits empty sections. If `PendingActions` is empty, the digest opens with the Tier 2 review. If both are empty, it opens with the status summary.

Device adaptation (compact vs full) is handled by the front-end agent based on user profile preferences, not by the schema. The same `DigestData` is served regardless of device -- the agent decides what to show.
