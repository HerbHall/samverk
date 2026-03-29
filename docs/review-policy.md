# Review Policy

## Overview

This document outlines code review requirements and policies for Samverk across different Git forges (GitHub and Gitea).

## GitHub Review Policy

### Copilot Code Review (GitHub-only)

**Status:** Informational only (deprecated for primary review gate)

The `scripts/copilot-review-setup.sh` script configures GitHub Copilot code review via GitHub Rulesets API. **This feature is GitHub-specific and does not exist on Gitea.**

#### Key Constraints

- Copilot can only **comment** on PRs; it cannot **approve**
- Setting `required_approving_review_count: 1` creates an unsatisfiable merge gate
- Copilot review is informational only; it does not satisfy approval requirements

#### Configuration

1. Set `required_approving_review_count: 0` in GitHub branch protection
2. Enable `copilot_code_review` with `review_on_push: true` for comments
3. Use GitHub Rulesets (not branch protection alone) for review policies
4. CI gates are the only enforced merge requirement

See `project-templates/copilot-ruleset.json` for a reference ruleset configuration.

#### Post-Merge Review Followup

Copilot may add review comments to already-merged PRs days after merge. Monitor and address these via:

```bash
gh api repos/{owner}/{repo}/pulls/{number}/comments \
  --jq '.[] | select(.created_at > "MERGE_TIME")'
```

Create a followup branch with fixes, referencing the original PR.

### CI as Merge Gate

- CI/CD pipelines (GitHub Actions) are the only enforced merge requirement
- Use `--admin` flag only for critical CI infrastructure failures
- Human approval is encouraged but not enforced at the API level

## Gitea Review Policy

### Review Requirements

- **Copilot integration:** Not available on Gitea
- **Code review:** Handled via pull request reviews (standard Gitea feature)
- **Approval:** Manual review and approval via PR interface
- **CI gates:** Via Gitea Actions (see `.gitea/workflows/`)

## Forge Abstraction

Review policies are abstracted behind the `IssueTracker` interface in `internal/forge/`. Implementation-specific review behavior is isolated to:

- `internal/forge/github/` — GitHub Rulesets and Copilot integration
- `internal/forge/gitea/` — Standard Gitea PR review

See [ADR-013: Forge Abstraction](decisions/ADR-013-forge-abstraction.md) for architectural details.

## Deprecated Scripts

| Script | Reason | Replacement |
|--------|--------|------------|
| `scripts/copilot-review-setup.sh` | GitHub-specific; Copilot does not exist on Gitea | Manual GitHub Rulesets configuration or use template at `project-templates/copilot-ruleset.json` |

## See Also

- [Communication Protocol](communication-protocol.md) — Issue schema and PR workflow
- [ADR-013: Forge Abstraction](decisions/ADR-013-forge-abstraction.md) — Implementation details
- `project-templates/copilot-ruleset.json` — Reference GitHub Ruleset configuration
