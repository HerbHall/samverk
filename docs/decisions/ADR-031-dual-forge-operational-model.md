# ADR-031: Single-Forge-Per-Project Model

**Status:** Revised (2026-03-17)
**Date:** 2026-03-08
**Revised:** 2026-03-17
**Supersedes:** None

## Context

Samverk originally targeted GitHub as the only issue tracker and forge.
Three operational concerns drove support for a second forge:

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

The initial design described a "dual-forge" model where a project could be
simultaneously registered on both GitHub and Gitea, with issue state synced
between them. In practice, issue sync created divergence and was never reliably
automated. The dual-registration capability was used exclusively during
migration, not during steady-state operation.

This revision replaces the dual-forge framing with a cleaner
single-forge-per-project model that reflects how the system actually works.

## Decision

Each project has exactly one active forge at any given time. The forge
abstraction layer (ADR-013) makes the choice transparent to the dispatcher and
all agents.

### Seven Principles

1. **One forge per project.** A project entry in `server.yaml` specifies a
   single forge (`github` or `gitea`, defaulting to `github`). The dispatcher
   watches and writes to that forge only.

2. **Forge is chosen at registration time.** The `forge` field in `server.yaml`
   is the authoritative source. Changing a project's forge requires updating
   `server.yaml` and restarting the server — there is no runtime switching.

3. **Gitea is the preferred forge for autonomous work.** New projects under
   active autonomous development are registered on the self-hosted Gitea
   instance. This keeps autonomous write activity on the LAN, reduces blast
   radius, and avoids polluting public GitHub issue trackers with intermediate
   agent state.

4. **GitHub is the release forge.** `release-please` and the public changelog
   live on GitHub. Gitea receives mirrored tags after releases are cut.
   Human-facing artefacts (releases, binaries, public issues) stay on GitHub.

5. **Dual-registration is migration scaffolding, not operational state.**
   The multi-project capability in `server.yaml` exists to support migration
   windows where a project is moving between forges. It is not intended as a
   steady-state dual-tracker configuration.

6. **No issue sync.** There is no synchronisation mechanism between GitHub and
   Gitea issue trackers. Migration is a one-time, one-direction operation.
   Attempting to keep both in sync creates divergence and maintenance burden.

7. **The forge abstraction is the stability guarantee.** Switching a project
   from GitHub to Gitea (or any future forge) requires only a `server.yaml`
   change and a new forge adapter if needed. No dispatcher, agent, or MCP tool
   logic changes.

### Authentication

Gitea uses Bearer token auth. The token is sourced from the `GITEA_TOKEN`
environment variable or the per-project `gitea_token` field in `server.yaml`.
GitHub projects use `GITHUB_TOKEN`. Neither token is committed to the
repository.

The MCP handler no longer requires `SAMVERK_GITHUB_OWNER` or
`SAMVERK_GITHUB_REPO` environment variables to initialise. These were a
bootstrap workaround before the handler was decoupled from the GitHub default
project (PR #644). They can be removed from all deployments.

## Consequences

**Positive:**

- Operational model matches actual usage — no confusion about which forge is
  the source of truth for a given project.
- No issue sync maintenance burden.
- MCP handler boots cleanly with Gitea-only configuration.
- Infrastructure is portable — switching forges requires only a config change.

**Negative:**

- Projects on GitHub that want autonomous dispatcher operation must be migrated
  to Gitea or accept the higher blast radius.
- No automatic mirroring of GitHub issues to Gitea; migration is manual.

## Revision History

| Date | Change |
|------|--------|
| 2026-03-08 | Initial: dual-forge operational model, dual-registration, issue sync concept |
| 2026-03-17 | Revised to single-forge-per-project; removed issue sync; added 7 principles; removed bootstrap env var requirement (#644) |

## Related

- [ADR-013: Forge Abstraction](ADR-013-forge-abstraction.md)
- [ADR-019: Self-Hosted First](ADR-019-self-hosted-first.md)
- [docs/gitea-migration-plan.md](../gitea-migration-plan.md)
