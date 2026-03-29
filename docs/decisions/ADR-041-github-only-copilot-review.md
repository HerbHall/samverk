# ADR-041: GitHub-Only Copilot Review Deprecation

**Status:** Accepted
**Date:** 2026-03-28
**Context:** Forge cutover to Gitea requires deprecating GitHub-specific automation

## Problem

The `scripts/copilot-review-setup.sh` script configures GitHub Copilot code review via the GitHub Rulesets API. With the planned cutover from GitHub to Gitea as the primary forge, this GitHub-specific feature is no longer applicable to all deployments.

Additionally, Copilot code review has fundamental limitations:

- Copilot cannot **approve** PRs; it can only **comment**
- Setting `required_approving_review_count: 1` creates an unsatisfiable merge gate
- Copilot review was always informational, never a merge blocker

## Decision

1. **Mark `scripts/copilot-review-setup.sh` as deprecated**
   - Remove from conformance audit
   - Update script with deprecation warning and pointer to manual configuration
   - Document in `docs/review-policy.md` as GitHub-only

2. **Clarify review policy**
   - CI/CD (GitHub Actions / Gitea Actions) is the only enforced merge gate
   - Code review (Copilot on GitHub, manual on Gitea) is informational
   - No approval requirement enforced at git forge API level

3. **Abstract review behavior behind forge interface**
   - Review policies remain in `IssueTracker` implementation
   - GitHub: Continue informational Copilot review with manual setup
   - Gitea: Use standard Gitea PR review interface

## Rationale

- **Forge independence:** Supporting both GitHub and Gitea requires features that work on both platforms
- **Honest capability:** Copilot was always informational; formalizing this reduces confusion
- **Reduced maintenance:** Removes the burden of automation for non-portable features
- **Manual control:** Operators retain choice on GitHub; template provided for reference

## Consequences

### Positive

- Single source of truth for merge requirements (CI gates only)
- No silent failures on Gitea when Copilot-expecting configs are applied
- Reduced automation surface area

### Negative

- GitHub deployments lose automatic Copilot review setup
- Operators must manually configure GitHub Rulesets (though template is provided)
- Requires education on informational vs. enforced review

## Alternatives Considered

1. **Implement Copilot mock on Gitea** — Not feasible; Gitea has no equivalent
2. **Keep auto-setup, fail silently on Gitea** — Poor UX; operator confusion
3. **Conditional script based on forge** — Adds complexity; manual approach is clearer

## Implementation

1. Add deprecation notice to `scripts/copilot-review-setup.sh`
2. Update `docs/review-policy.md` to document GitHub-only status
3. Remove from all conformance checklists
4. Point to `project-templates/copilot-ruleset.json` for manual setup reference

## References

- [ADR-013: Forge Abstraction](ADR-013-forge-abstraction.md)
- `docs/review-policy.md` — Review policies and configuration
- `project-templates/copilot-ruleset.json` — Reference GitHub Ruleset template
