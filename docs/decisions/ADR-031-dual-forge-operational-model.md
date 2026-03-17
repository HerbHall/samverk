# ADR-031: Single-Forge-Per-Project Model

**Status:** Revised (2026-03-17, supersedes dual-forge model)
**Date:** 2026-03-08
**Revised:** 2026-03-17

## Context

The original ADR-031 (March 2026) described a dual-forge model where a single project
could be registered against both GitHub and Gitea simultaneously, with issues synced
between them. In practice this model created more problems than it solved:

- Issue sync was never reliably implemented — state diverged silently.
- Dual-registration in `server.yaml` doubled webhook, label, and milestone maintenance.
- Agents occasionally created issues on the wrong forge, requiring manual cleanup.
- The "GitHub as public source of truth" rationale weakened as the project matured on Gitea.

The migration of the Samverk project to Gitea (March 2026) made the dual-forge model
irrelevant for the primary project. This ADR replaces it with a clearer policy.

## Decision

Each project has exactly one active forge at any given time. Dual-registration is
only permitted during a time-bounded migration window.

### Seven Forge Principles

1. **One forge per project** — `server.yaml` has a single entry per logical repository.
   The `name` field is unique and maps to exactly one forge + owner + repo combination.

2. **Gitea by default for new projects** — Self-hosted-first (ADR-019). New Samverk
   projects are registered on `gitea.herbhall.net` unless there is a specific reason
   to use a public forge.

3. **GitHub for public visibility only** — Projects on GitHub are either actively
   developed there (external contributor projects) or are read-only public archives.
   The dispatcher does not write to GitHub unless the project's forge is explicitly
   set to `github`.

4. **No cross-forge issue sync** — Issues live on one forge. Migration is a one-time
   operation (create on destination, close on source with a redirect comment). There
   is no ongoing sync mechanism and none will be built.

5. **Dual-registration is migration-only** — The `server.yaml` multi-entry capability
   exists solely to support a clean cutover. Both entries must have distinct `name`
   values. The old entry is removed within one sprint of the migration completing.

6. **Forge abstraction is the interface** — `forge.IssueTracker` and related interfaces
   (ADR-013) remain the single interface layer. Forge choice is a configuration concern,
   not a code concern. Switching a project between forges requires only a `server.yaml`
   change and a token rotation.

7. **Releases follow the code** — Release tooling (release-please, semantic-release)
   runs on whichever forge hosts the primary development branch. For Gitea projects,
   use `@saithodev/semantic-release-gitea`. There is no requirement to mirror releases
   to a secondary forge.

## Current Forge Allocation

| Project | Forge | Status |
|---------|-------|--------|
| samverk/samverk | Gitea (`gitea.herbhall.net`) | Active development |
| samverk/synapset | Gitea (`gitea.herbhall.net`) | Active development |
| samverk/devkit | Gitea (`gitea.herbhall.net`) | Active development |
| HerbHall/samverk | GitHub | Read-only archive (issues disabled) |
| HerbHall/* | GitHub | Active (external-facing projects) |

## Consequences

**Positive:**

- Dispatcher configuration is unambiguous — one forge per project, no sync state.
- Label and milestone maintenance is halved.
- Agent issue creation has a single target — no wrong-forge mistakes.
- Migration path is clear: create, close with redirect, remove old entry.

**Negative:**

- Projects migrating from GitHub to Gitea require a migration sprint.
- GitHub public history for Samverk is frozen — external contributors must be
  directed to Gitea.

## Revision History

| Date | Change |
|------|--------|
| 2026-03-08 | Original: dual-forge operational model accepted |
| 2026-03-17 | Revised: replaced with single-forge-per-project model after Samverk migration completed |

## Related

- [ADR-013: Forge Abstraction](ADR-013-forge-abstraction.md)
- [ADR-019: Self-Hosted First](ADR-019-self-hosted-first.md)
- [docs/gitea-migration-plan.md](../gitea-migration-plan.md)
- [docs/gitea-actions-compatibility.md](../gitea-actions-compatibility.md)
