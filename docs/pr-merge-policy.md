# PR Merge Policy

Samverk enforces a tier-based merge policy for pull requests across all managed
projects. Every PR is classified into a tier based on its linked issue labels
and the files it touches. The tier determines whether the PR can be auto-merged
or requires human review.

## Tier Definitions

| Tier | Description | Merge Rule |
|------|-------------|------------|
| Tier 1 | Docs, autolearn, small fixes, config-only | Auto-merge immediately on CI green |
| Tier 2 | Feature code, test additions, refactoring | Surface in digest, auto-merge after delay (default 1h) |
| Tier 3 | Architecture, breaking changes, deployments, security | Never auto-merge; label `status:needs-human` |

## Tier Classification Rules

A PR's tier is derived from its linked issue labels and title prefix. The
highest-matching tier wins (Tier 3 > Tier 2 > Tier 1).

### Tier 3 (human review required)

- Issue has label `priority:critical` or `complexity:high`
- Issue has label `type:architecture` or `type:breaking`
- PR title contains "breaking", "security", or "deploy"
- PR modifies files in `internal/autonomy/`, `internal/store/` schema, or CI workflows

### Tier 2 (delayed auto-merge)

- Issue has label `agent:code-gen` or `agent:test`
- PR title starts with `feat:`, `feat(`, `fix:`, `fix(`, `refactor:`
- PR touches Go source files in `internal/` or `cmd/`

### Tier 1 (immediate auto-merge)

- Issue has label `agent:docs` or `type:chore`
- PR title starts with `docs:`, `chore:`, `ci:`, `style:`
- PR only modifies `.md`, `.yml`, `.yaml`, or `.json` files
- Release-please PRs (label `autorelease: pending`)

## CI Failure Handling

When CI fails on a PR:

1. PR watcher comments the failure details on the PR
2. The linked issue is re-queued (`status:queued`)
3. The PR is not merged regardless of tier

## No-CI Projects

Projects without CI configured (e.g., new Gitea projects) treat PRs as
Tier 2 by default. The delay allows the user to review during their next
check-in window.

## Merge Conflict Handling

When a PR has merge conflicts:

1. PR watcher labels the PR with `status:blocked`
2. A comment is posted explaining the conflict
3. The linked issue is not re-queued (the branch needs manual rebase)

## MCP Tools

The following MCP tools support PR review and merge workflows:

| Tool | Description |
|------|-------------|
| `list_prs` | List PRs for the active project |
| `list_open_prs` | Aggregate open PRs across all registered projects |
| `get_pr` | Get full PR details by number |
| `review_pr` | Summarize PR: diff stats, CI status, tier assessment |
| `merge_pr` | Merge a single PR (respects tier confirmation) |
| `bulk_merge` | Merge all Tier 1 PRs with green CI |

## Digest Integration

The check-in digest includes a "PRs Awaiting Review" section showing:

- Open PRs grouped by project
- Each PR's tier, CI status, and age
- Count of auto-merged PRs since last check-in

## Configuration

Tier delay and classification can be tuned in `server.yaml` under the
`autonomy.merge` section:

```yaml
autonomy:
  merge:
    auto_merge_on_ci_pass: true
    tier2_delay_minutes: 60
    trusted_authors:
      - samverk-bot
    exclude_labels:
      - status:needs-human
      - do-not-merge
```
