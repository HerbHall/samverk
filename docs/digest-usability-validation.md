# Check-in Digest Usability Validation

Research findings for issue #64. Validates the check-in digest concept through structured user interviews before investing further in implementation.

## Verdict: GO (with caveats)

The check-in digest concept is fundamentally sound. The async check-in model, conversational format, and tiered autonomy approach all hold up under scrutiny. However, three design constraints emerged that must be addressed in implementation or the product risks UX failure:

1. **Notification routing** -- configurable per action type per project (not one-size-fits-all)
2. **Tier 3 volume control** -- max 3-4 items per day, with autonomy tuning guidance
3. **Direction fidelity** -- structured options for known decisions, free text for new direction

## Findings by Research Question

### 1. Cognitive Load -- Can a User Absorb State in 5-15 Minutes?

**Answer: Yes, but the time window varies wildly.**

The user's available time ranges from 2 minutes (phone glance during a break) to 30 minutes (dedicated desktop session). The 5-15 minute target is a midpoint, not a universal window.

**Design implication:** The digest must support two distinct interaction modes:

| Mode | Device | Time | Content |
|------|--------|------|---------|
| Quick glance | Phone | 2-5 min | Tier 3 items only + cost one-liner. Approve/reject. Done. |
| Full session | Desktop | 10-30 min | All sections, diffs on request, direction-setting, priority changes |

The current design's compact phone format (Tier 3 + cost + counts) is validated for the quick glance mode. Desktop gets the full digest as designed.

**Key constraint:** These are truly different modes, not a collapsed version of the same thing. The phone mode should not even present Tier 2 information unless explicitly requested.

### 2. Decision Quality -- Are Compressed Decisions Good Decisions?

**Answer: Yes, with structured options.**

Two modes for user direction:

- **Structured options** for known decisions: agent proposes 2-3 approaches, user picks one. Reduces ambiguity, prevents misinterpretation, faster on mobile.
- **Free text** for new direction: "focus on dispatcher routing" or "hold off on new features." Agents use issue context + history to fill gaps.

**Design implication:** The digest should default to "agent proposes, user disposes" rather than open-ended "what would you like to do?" Every Tier 3 item should include the agent's recommendation. Direction-setting should offer suggested priorities when possible.

Example evolution:

```text
# Current design (open-ended)
What would you like to do?

# Validated design (agent proposes)
Suggested focus: #51 dispatcher routing (70% complete, 2 blockers resolved).
Alternative: #54 test harness (3 issues queued, all unblocked).
> "focus 51" | "focus 54" | free text direction
```

**Complexity-dependent detail:** Simple reprioritization needs only a one-liner. Architecture changes or direction shifts warrant a brief paragraph. The system should not demand equal detail for unequal decisions.

### 3. Check-in Frequency and Missed Check-ins

**Answer: Configurable decay per project.**

No single frequency works. The user needs per-project control:

| Project State | Inactivity Threshold | Behavior After Threshold |
|--------------|---------------------|------------------------|
| Active (primary project) | 24h | Pause new task intake, finish in-progress only |
| Moderate | 48h | Same behavior, longer runway |
| Low-priority / maintenance | 7 days | Run for a week before pausing |

**Design implication:** Add `check_in_decay` to project configuration:

```yaml
# .samverk/server.yaml per-project
projects:
  samverk:
    check_in_decay:
      pause_new_after: 24h    # stop picking up new issues
      pause_all_after: 72h    # stop all work (except in-progress commit)
      notify_after: 12h       # first "you haven't checked in" notification
```

When the user returns after a long absence, the digest compresses older days into summaries (current design handles this well). The validated design adds an explicit "X work was paused due to inactivity -- resume?" prompt.

### 4. Tier 3 Blocking Volume

**Answer: Max 3-4 per day. More indicates misconfigured autonomy.**

If 8-10 Tier 3 items accumulate overnight, the check-in becomes a chore and the async value is destroyed. The autonomy config is wrong, not the check-in design.

**Design implications:**

1. **Autonomy tuning guidance:** After a check-in with 5+ Tier 3 items, the system should suggest: "You approved 5 dependency additions today. Consider promoting `add_dependency` to Tier 2 for this project."
2. **Auto-learning:** Track approval patterns. If the user approves a specific action type 10 times without rejection, suggest promotion to Tier 2.
3. **Volume alert:** If 4+ Tier 3 items accumulate between check-ins, warn the user that autonomy config may need adjustment.
4. **Default tier review:** The current defaults may be too conservative. Consider promoting `add_dependency` and `modify_ci` to Tier 2 by default (they are logged and reversible).

### 5. Device Constraints

**Answer: Phone works for approvals. Desktop needed for direction.**

The "mixed equally" usage pattern validates the two-mode design. Phone is the approval device. Desktop is the direction-setting device.

**Design implication:** The phone mode should be optimized for speed:

- Numbered items for quick reference (`1`, `2`, `3`)
- Single-character commands (`1` = approve, `1r` = reject, `1?` = details)
- No scrolling through Tier 2 unless requested
- Cost displayed as one line, not a breakdown

The desktop mode can afford full detail and supports the direction-setting conversation.

### 6. Direction-Giving Fidelity

**Answer: Agent proposes, user disposes. Both structured and free text.**

This is the biggest product risk. The mitigation is dual-mode direction:

1. **Structured options** (default): Agent analyzes current state and proposes 2-3 prioritization options with rationale. User picks one. Minimizes misinterpretation.
2. **Free text** (when needed): For new direction, architectural shifts, or context the agent can't anticipate. Agent confirms understanding before acting: "I understand you want to focus on X because Y. I'll reprioritize Z and W. Correct?"

**Intent verification integration:** The Intent Verification Protocol (ADR-021) applies here. Before executing a direction change, the agent should verify its understanding of the user's intent, especially for ambiguous free-text direction.

## New Requirements Discovered

### Notification System

**Requirement:** Pluggable notification interface with email as the default adapter.

```go
// Notifier sends alerts to the user through a configured channel.
type Notifier interface {
    // Send dispatches a notification. The implementation determines
    // the channel (email, push, webhook, messenger).
    Send(ctx context.Context, notification Notification) error
}

type Notification struct {
    Project   string
    Severity  NotifySeverity // info, warning, urgent, emergency
    Title     string
    Body      string
    Actions   []NotifyAction // approve/reject links if applicable
}
```

Per-project notification routing:

```yaml
# .samverk/notifications.yaml
defaults:
  channel: email
  address: user@example.com

projects:
  samverk:
    rules:
      - action: merge_main
        severity: urgent
        channel: push     # immediate push notification
      - action: add_dependency
        severity: info
        channel: email    # daily digest email
      - action: budget_exceeded
        severity: emergency
        channel: push
  runbooks:
    rules:
      - action: "*"
        severity: info
        channel: email    # low-priority project, email only
```

Ship with: email adapter. Provide interface for community adapters (ntfy, Pushover, Telegram, Signal, webhooks).

### Visual Dashboard

**Requirement:** Web dashboard AND conversational interface, both equally supported.

The current design is chat-only. User feedback validates adding a visual dashboard as a peer interface, not a secondary one.

Dashboard scope (visual, not conversational):

- Project status overview (active, queued, blocked counts)
- Cost trends and budget gauge
- Tier 3 pending items with approve/reject buttons
- Tier 2 action timeline
- Multi-project unified view
- Historical trends (cost, completion rate, Tier 3 volume)

Chat scope (conversational, not visual):

- Direction-setting and nuanced priority changes
- Tier 3 context exploration (`N?`)
- Free-text direction with intent verification
- Complex override scenarios (`undo N because...`)

**Design implication:** The web dashboard (ADR-020) should be a first-class check-in surface, not just an ops view. Both the MCP chat and the dashboard should be able to resolve Tier 3 items.

### Multi-Project Unified Digest

**Requirement:** Unified summary across projects, with per-project deep dive.

```text
SAMVERK: Welcome back (8h away). 3 projects active.

--- CROSS-PROJECT SUMMARY ---
Tier 3 pending: 2 (samverk: 1, subnetree: 1)
Active work: 5 issues across 2 projects
Cost today: $3.40 / $25.00 daily budget

--- SAMVERK (1 decision needed) ---
[1] merge_main: PR #52 Gitea IssueTracker -> main
    > 1 approve | 1r reject | 1?

--- SUBNETREE (1 decision needed) ---
[2] add_dependency: Add paho.mqtt.golang v1.5.0
    > 2 approve | 2r reject | 2?

--- RUNBOOKS (no decisions, running) ---
Active: 1 issue (#82 README update)
Cost: $0.12

Type "samverk details" or "subnetree details" for full project digest.
```

### Batch Approval Safety

**Requirement:** Batch approve only for low-risk action types. Never batch approve merges.

```text
# These can be batch-approved:
> approve deps        # all pending add_dependency items
> approve ci          # all pending modify_ci items

# These cannot be batch-approved:
> approve merges      # ERROR: merge_main requires individual review
> approve all         # ERROR: contains 1 merge_main item. Approve non-merge items? (y/n)
```

Configuration:

```yaml
# .samverk/autonomy.yaml
batch_approve:
  allowed:
    - add_dependency
    - modify_ci
    - create_tag
  blocked:
    - merge_main
    - delete_file
    - force_push
    - modify_secrets
```

## Risks That Could Kill the Product

### Risk 1: Notification Fatigue (Medium)

If notifications fire too often or for low-value events, the user will disable them entirely and miss critical Tier 3 blocks.

**Mitigation:** Conservative defaults (email only, daily summary). Urgent push only for `merge_main` and `budget_exceeded`. Let users opt in to more, not opt out of less.

### Risk 2: Stale Context on Long Absence (Medium)

After 3+ days away, Tier 3 items may have stale context. The agent's analysis from 3 days ago may no longer reflect the current codebase state.

**Mitigation:** Already in the current design (48h staleness warning). Add automatic context refresh when the user requests details on a stale item.

### Risk 3: Direction Misinterpretation (High)

Free-text direction like "make the tests better" is ambiguous. Agents may waste cycles building the wrong thing.

**Mitigation:** Intent verification before acting. Agent summarizes its interpretation and waits for confirmation before executing direction changes that affect multiple issues.

### Risk 4: Autonomy Config Complexity (Medium)

Per-project, per-agent, per-branch autonomy config is powerful but complex. Users may never tune it, leaving conservative defaults that generate too many Tier 3 items.

**Mitigation:** Auto-tuning suggestions based on approval patterns. "You've approved 8 `add_dependency` items this month without rejection. Promote to Tier 2?"

## UX Constraints for Implementation

These constraints are non-negotiable based on the validation findings:

1. **Two distinct modes:** Phone quick-glance (Tier 3 + cost only) and desktop full session (all sections). Not a responsive version of the same thing.
2. **Agent proposes, user disposes:** Every Tier 3 item includes the agent's recommendation. Direction-setting offers structured options with a free-text escape hatch.
3. **Max 3-4 Tier 3 items per check-in:** System warns and suggests autonomy tuning when volume exceeds this.
4. **Configurable decay per project:** Inactivity thresholds control how long agents run without check-in.
5. **Pluggable notifications:** Interface + email default. Per-action-type, per-project routing.
6. **Batch approve safety:** Blocked list prevents batch approval of irreversible actions.
7. **Visual dashboard as peer interface:** Not secondary to chat. Both can resolve Tier 3 items.
8. **Unified multi-project digest:** Cross-project summary with per-project drill-down.
9. **Intent verification on direction changes:** Agent confirms understanding before executing ambiguous direction.

## Related Documents

- [Check-in Digest Design](check-in-digest-design.md) -- existing design, validated with modifications above
- [User Interface](user-interface.md) -- device flexibility spec
- [Autonomy Model](autonomy-model.md) -- tier definitions and configuration
- [Intent Verification Protocol](intent-verification.md) -- pre-execution understanding verification
- [MCP Server](mcp-server.md) -- MCP tool definitions
- [Cost Model](cost-model.md) -- budget and cost transparency
- ADR-006 (async-first), ADR-009 (device flexibility), ADR-020 (web dashboard), ADR-021 (intent verification)
