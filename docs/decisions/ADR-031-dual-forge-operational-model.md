# ADR-031: Dual-Forge Operational Model

**Status:** Accepted
**Date:** 2026-03-08
**Supersedes:** None

## Context

Samverk originally targeted GitHub as the only issue tracker and forge.
Three operational concerns drove the need for a second forge:

1. **Autonomy risk** — a dispatcher running autonomously against GitHub has
   write access to a public-facing service. A Gitea instance on the LAN gives
   the same issue-tracking capability with a smaller blast radius for
   misconfigured automation.

2. **Self-hosted alignment** — Samverk's design philosophy is self-hosted-first
   (ADR-019). Operating against an external SaaS for core dispatcher state is
   inconsistent with that principle.

3. **Privacy** — development notes, draft frontmatter, and intermediate agent
   comments that live in issues may not be suitable for a public repository
   until they are ready for review.

The goal is a dual-forge model where:

- GitHub remains the public-facing, developer-visible tracker.
- Gitea (CT 200, `gitea.herbhall.net`) is the primary runtime forge for
  the dispatcher and agent communication during active development.

## Decision

Samverk supports two forges simultaneously. Key points:

### Forge abstraction is the interface

`forge.IssueTracker`, `forge.RepoReader`, and `forge.PullRequestManager`
(ADR-013) remain the single interface layer. Both GitHub and Gitea are
equal-status implementations. The dispatcher is unaware of which forge it
is talking to.

### Projects can be dual-registered

`server.yaml` supports a `forge` field per project entry (`github` or `gitea`).
A single logical repository can appear twice — once per forge — so the
dispatcher watches both. New issues created by agents on Gitea flow back to
GitHub via the migration script when ready.

### GitHub is the public source of truth for releases

`release-please` runs only on GitHub. Gitea receives tags mirrored from GitHub
after releases are cut (B21). This keeps the changelog and semantic versioning
tied to the public repository where external contributors can see it.

### Gitea is the runtime forge for autonomous work

The dispatcher preferentially operates against Gitea during the autonomous
development phase. GitHub issues are migrated to Gitea via `migrate-issues.py`
so the full backlog is available. Finished work is merged to GitHub via the
normal PR flow (dual-push remote, configured in B24).

### Authentication

Gitea uses Bearer token auth. The token is stored in `GITEA_TOKEN` environment
variable or per-project `gitea_token` field in `server.yaml`. The token for
`samverk-admin` on the self-hosted instance is provisioned separately and not
committed to the repository.

## Consequences

**Positive:**

- Dispatcher blast radius is contained to the LAN Gitea instance.
- GitHub remains clean and human-readable.
- Infrastructure is portable — replacing Gitea with another forge only
  requires implementing a new `IssueTracker` adapter.
- `server.yaml` dual-entry model generalises to N-forge without code changes.

**Negative:**

- Issue state must be kept in sync across forges (manual migration or scripted).
  Divergence is possible if sync is missed.
- Two forges means two sets of labels, milestones, and webhooks to maintain.
- The Gitea instance is a single point of failure for autonomous operation
  (mitigated by fallback to GitHub mode via config change).

## Related

- [ADR-013: Forge Abstraction](ADR-013-forge-abstraction.md)
- [ADR-019: Self-Hosted First](ADR-019-self-hosted-first.md)
- [docs/gitea-migration-plan.md](../gitea-migration-plan.md)
- [docs/gitea-actions-compatibility.md](../gitea-actions-compatibility.md)
- [B-track issues #256–#283](https://github.com/HerbHall/samverk/milestone/5)
