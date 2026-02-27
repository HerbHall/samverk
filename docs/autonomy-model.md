# Action Trust Tier Model

## The Problem

Samverk agents work while the user is away. Two failure modes exist:

- **Too many approvals** -- async value is destroyed. The user returns to a queue of yes/no prompts instead of progress. The entire point of Samverk is eliminated.
- **Too few approvals** -- agents make irreversible decisions while the user is at work. Deleted files, bad merges, runaway API spend.

The solution is not a binary switch. It is a configurable trust tier per action type.

## The Three Tiers

### Tier 1 -- Always Autonomous

Agent proceeds immediately. The action is logged for review at the next check-in.

Examples:

- Read any file
- Search codebase or web
- Create new files in designated draft/work directories
- Open issues, add comments to issues
- Add labels to issues
- Run tests (read-only result)
- Install packages to a dev environment
- Create new branches
- Commit to a feature branch

### Tier 2 -- Autonomous With Logging

Agent proceeds, but the action is surfaced prominently in the check-in digest so the user can review and override if needed.

Examples:

- Edit existing files
- Close or reopen issues
- Merge feature branch to dev/staging
- Make API calls below a cost threshold (configurable)
- Delete draft/temporary files
- Push to non-main branches

### Tier 3 -- Requires Confirmation Before Proceeding

Agent queues the action, labels it `needs-human`, and continues other unblocked work. Does NOT block the entire pipeline -- only this specific action stream waits.

Examples:

- Merge to main
- Delete non-temporary files
- Force push anything
- API calls above cost threshold
- Any action explicitly marked irreversible in agent config
- Structural changes to project (rename, reorganize)

## Key Design Principle

**A Tier 3 block never stops the whole system.**

When an agent hits a Tier 3 action, it:

1. Creates a `needs-human` issue with the pending action and context
2. Continues all other work that does not depend on this action
3. User addresses it at next check-in
4. Dependent work resumes immediately after approval

This is the difference between "paused" and "blocked on one thing." The async value is preserved.

## Configuration

Trust tiers are configurable per project and per user. The defaults above are conservative. Power users can promote actions to lower tiers. Cautious users can demote actions to higher tiers.

```yaml
# .samverk/autonomy.yaml
tier_overrides:
  # Promote merge-to-main to Tier 2 for this project (I trust my QC setup)
  merge_main: tier2

  # Demote file deletion to Tier 3 even for temp files (I'm paranoid about this)
  delete_temp: tier3

  # Cost threshold for autonomous API calls
  api_cost_threshold_usd: 5.00
```

## Tier 3 Block Behavior

When an agent encounters a Tier 3 action:

1. The agent creates an issue with label `needs-human` containing:
   - What action is pending
   - Why the agent wants to take this action
   - What context is relevant
   - What will happen after approval
2. The agent marks all dependent work as `status:blocked`
3. The agent continues all independent work streams
4. At the next check-in, the front-end agent surfaces Tier 3 items first
5. User approves or rejects conversationally
6. On approval, the dispatcher unblocks dependent work immediately

## Check-in Digest Integration

The check-in digest presents autonomy tiers in priority order:

1. **Tier 3 pending** (highest priority) -- work is blocked on these decisions
2. **Tier 2 completed** (awareness) -- user can review and override
3. **Tier 1 summary** (audit trail) -- available on request, not shown by default

## Relation to Claude Code Settings

The immediate Claude Code fix for this project uses broad permissions:

```json
{
  "permissions": {
    "allow": ["Bash(*)"]
  }
}
```

This is essentially Tier 1 for everything -- appropriate for a sandboxed dev project where the developer has reviewed the instructions. Samverk's production autonomy model is more nuanced but follows the same principle: match autonomy level to trust level and reversibility of the action.

## Related Decisions

- [ADR-015: Three-Tier Autonomy Model](decisions/ADR-015-three-tier-autonomy.md)
- [ADR-006: Async-First Architecture](decisions/ADR-006-async-first.md)
