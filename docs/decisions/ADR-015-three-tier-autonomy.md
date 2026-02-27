# ADR-015: Three-Tier Autonomy Model

## Status

Accepted

## Context

Samverk agents work asynchronously while the user is away. A binary approve/deny model for agent actions creates two failure modes: too many approvals destroy async value (user returns to a prompt queue), and too few approvals risk irreversible damage (deleted files, bad merges, runaway spend).

This tension was directly observed during Samverk development when Claude Code repeatedly stopped to request approval for individual Bash commands because the permission config used an explicit allowlist rather than a wildcard. The user returned to a queue of prompts instead of completed work.

## Decision

Samverk uses a configurable three-tier trust model for agent actions:

- **Tier 1 (Always Autonomous):** Agent proceeds, action logged for audit. Reads, searches, branch creation, commits to feature branches.
- **Tier 2 (Autonomous With Logging):** Agent proceeds, action surfaced prominently in check-in digest. File edits, issue state changes, non-main pushes, API calls under cost threshold.
- **Tier 3 (Requires Confirmation):** Agent queues the action as `needs-human` and continues unblocked work. Merges to main, file deletion, force pushes, over-threshold API calls, structural changes.

A Tier 3 block never halts the full pipeline -- only the dependent work stream pauses.

Trust tiers are configurable per project via `.samverk/autonomy.yaml`. Users can promote or demote actions between tiers.

## Consequences

- Preserves async value by ensuring most work proceeds without interruption
- Protects against irreversible actions via Tier 3 confirmation gate
- Requires careful default mapping of actions to tiers
- Check-in digest must present Tier 3 pending items and Tier 2 completed items clearly
- Configuration schema adds complexity but is essential for user trust

## References

- [Autonomy Model](../autonomy-model.md)
- [ADR-006: Async-First Architecture](ADR-006-async-first.md)
