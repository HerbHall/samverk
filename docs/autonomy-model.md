# Action Trust Tier Model

## The Problem

Samverk agents work while the user is away. Two failure modes exist:

- **Too many approvals** -- async value is destroyed. The user returns to a queue of yes/no prompts instead of progress. The entire point of Samverk is eliminated.
- **Too few approvals** -- agents make irreversible decisions while the user is at work. Deleted files, bad merges, runaway API spend.

The solution is not a binary switch. It is a configurable trust tier per action type.

## The Three Tiers

### Tier 1 -- Always Autonomous

Agent proceeds immediately. Action is summarized as a comment on the parent task issue.

**Default actions:**

| Action | Rationale |
|--------|-----------|
| Read any file | No side effects |
| Search codebase or web | No side effects |
| Create new files in work directories | Reversible, scoped |
| Open issues | Proposals, not changes |
| Add comments to issues | Communication |
| Add/remove labels on issues | Routing, not decisions |
| Run tests | Read-only result |
| Create new branches | Lightweight, no impact on others |
| Commit to a feature branch | Scoped to branch, reversible |
| Run linters, formatters | Read-only analysis |
| Query APIs (read-only) | No mutation, no cost |

### Tier 2 -- Autonomous With Logging

Agent proceeds. Action is surfaced prominently in the check-in digest for user review and possible override.

**Default actions:**

| Action | Rationale |
|--------|-----------|
| Edit existing files | Changes production code, but reversible via git |
| Close or reopen issues | State change visible to collaborators |
| Create pull requests | Visible to collaborators, triggers CI |
| Push to non-main branches | Publishes work, but scoped |
| Merge feature branch to dev/staging | Integration step, but not production |
| Delete draft/temporary files | Low-risk cleanup |
| Make API calls below cost threshold | Bounded spend |
| Run external services (Docker containers) | Side effects, but scoped to dev env |

### Tier 3 -- Requires Confirmation Before Proceeding

Agent queues the action, labels it `needs-human`, and continues other unblocked work. Does NOT block the entire pipeline -- only this specific action stream waits.

**Default actions:**

| Action | Rationale |
|--------|-----------|
| Merge to main/production | Irreversible deployment path |
| Add or update dependencies | License, security, and bloat risk |
| Modify CI/CD configuration | Pipeline changes can break everything or expose secrets |
| Delete non-temporary files | Potentially irreversible data loss |
| Force push anything | Rewrites shared history |
| API calls above cost threshold | Unbounded spend |
| Create or delete git tags | Semantic versioning, release implications |
| Structural changes (rename, reorganize) | Broad impact across codebase |
| Modify secrets/credentials config | Security-critical |
| Any action explicitly marked irreversible | Catch-all safety net |

## Key Design Principle

**A Tier 3 block never stops the whole system.**

When an agent hits a Tier 3 action, it:

1. Creates a `needs-human` issue with the pending action and context
2. Continues all other work that does not depend on this action
3. User addresses it at next check-in
4. Dependent work resumes immediately after approval

This is the difference between "paused" and "blocked on one thing." The async value is preserved.

## Tier 3 Block Behavior

When an agent encounters a Tier 3 action:

1. The agent creates an issue with label `needs-human` containing:
   - What action is pending
   - Why the agent wants to take this action
   - What context is relevant (parent issue, files affected)
   - What will happen after approval
   - What will happen if rejected (alternative path, if any)
2. The agent marks all dependent work as `status:blocked`
3. The agent continues all independent work streams
4. At the next check-in, the front-end agent surfaces Tier 3 items first
5. User approves or rejects conversationally
6. On approval, the dispatcher unblocks dependent work immediately
7. On rejection, the agent receives the rejection reason and may propose an alternative

## Audit Logging

All tiers are logged as comments on the parent task issue. This keeps the audit trail naturally scoped to the work being done and requires no additional infrastructure.

### Tier 1 Logging

Summarized in a single comment when the agent completes its task. Grouped by category:

```markdown
## Tier 1 Actions (autonomous)

- **Files read**: 12 files in `internal/dispatcher/`
- **Tests run**: `go test ./internal/dispatcher/...` -- 47 passed, 0 failed
- **Branches**: created `feature/issue-42-dispatcher-routing`
- **Commits**: 3 commits to `feature/issue-42-dispatcher-routing`
- **Issues**: opened #55 (sub-task), added labels to #42
```

### Tier 2 Logging

Each action gets its own comment immediately when taken, with enough context for the user to assess and override:

```markdown
## Tier 2 Action: edited `internal/dispatcher/router.go`

**What changed**: Refactored `routeTask()` to use label-based matching instead of regex.
**Lines affected**: 45-89 (44 lines modified)
**Why**: Original regex approach couldn't handle compound labels like `agent:code-gen+complexity:local`.
**Reversible**: Yes -- `git revert abc123` on branch `feature/issue-42-dispatcher-routing`.
```

### Tier 3 Logging

Handled via the `needs-human` issue (see Tier 3 Block Behavior above). The issue itself is the log entry.

## Check-in Digest Integration

The check-in digest presents autonomy tiers in priority order:

1. **Tier 3 pending** (highest priority) -- work is blocked on these decisions
2. **Tier 2 completed** (awareness) -- user can review and override
3. **Tier 1 summary** (audit trail) -- available on request, not shown by default

## Configuration

Trust tiers are configurable per project, per agent type, and per branch. Defaults are conservative. Users promote or demote actions between tiers.

### Schema

```yaml
# .samverk/autonomy.yaml

# Global defaults (apply to all agents, all branches)
defaults:
  api_cost_threshold_usd: 5.00

# Per-action tier overrides (global scope)
tier_overrides:
  # Promote merge-to-dev to Tier 1 for this project
  merge_staging: tier1

  # Demote file deletion to Tier 3 even for temp files
  delete_temp: tier3

# Per-agent-type overrides (takes precedence over global)
agents:
  qc:
    tier_overrides:
      # QC agent can close issues autonomously (Tier 1)
      close_issue: tier1
  code-gen:
    tier_overrides:
      # Code-gen agent cannot push without review
      push_branch: tier3

# Per-branch overrides (takes precedence over agent overrides)
branches:
  main:
    tier_overrides:
      # Everything touching main requires confirmation
      merge: tier3
      push: tier3
      delete_branch: tier3
  "feature/*":
    tier_overrides:
      # Feature branches are relaxed
      merge: tier2
      push: tier1
```

### Override Precedence

Resolution order (highest precedence first):

1. Branch-specific override for the target branch
2. Agent-type override for the acting agent
3. Global project override
4. System default

Example: code-gen agent pushing to `feature/issue-42`:

1. Check `branches.feature/*.push` -- found: `tier1`
2. Result: **Tier 1** (branch override wins)

Example: code-gen agent pushing to `main`:

1. Check `branches.main.push` -- found: `tier3`
2. Result: **Tier 3** (branch override wins)

### Action Keys

Complete set of action keys for `tier_overrides`:

| Key | Default Tier | Description |
|-----|-------------|-------------|
| `read_file` | 1 | Read any file |
| `search` | 1 | Search codebase or web |
| `create_file` | 1 | Create new files |
| `open_issue` | 1 | Create a new issue |
| `comment_issue` | 1 | Add comment to an issue |
| `label_issue` | 1 | Add/remove labels |
| `run_tests` | 1 | Execute test suite |
| `create_branch` | 1 | Create a new branch |
| `commit` | 1 | Commit to feature branch |
| `run_lint` | 1 | Run linters/formatters |
| `query_api` | 1 | Read-only API calls |
| `edit_file` | 2 | Modify existing files |
| `close_issue` | 2 | Close or reopen issues |
| `create_pr` | 2 | Create a pull request |
| `push_branch` | 2 | Push to non-main branches |
| `merge_staging` | 2 | Merge to dev/staging |
| `delete_temp` | 2 | Delete draft/temp files |
| `api_call_paid` | 2 | API calls under cost threshold |
| `run_service` | 2 | Start external services |
| `merge_main` | 3 | Merge to main/production |
| `add_dependency` | 3 | Add or update dependencies |
| `modify_ci` | 3 | Change CI/CD configuration |
| `delete_file` | 3 | Delete non-temporary files |
| `force_push` | 3 | Force push any branch |
| `api_call_expensive` | 3 | API calls over cost threshold |
| `create_tag` | 3 | Create or delete git tags |
| `restructure` | 3 | Rename/reorganize project |
| `modify_secrets` | 3 | Touch secrets/credentials config |
| `irreversible` | 3 | Catch-all for marked actions |

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

## Related Documents

- [Intent Verification Protocol](intent-verification.md) — pre-execution understanding verification (complements this permission model)
- [ADR-015: Three-Tier Autonomy Model](decisions/ADR-015-three-tier-autonomy.md)
- [ADR-021: Intent Verification Protocol](decisions/ADR-021-intent-verification.md)
- [ADR-006: Async-First Architecture](decisions/ADR-006-async-first.md)
