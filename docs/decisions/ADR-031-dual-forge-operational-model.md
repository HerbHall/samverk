# ADR-031: Forge Operations Model

**Status:** Accepted (revised 2026-03-17)
**Date:** 2026-03-08 (original), 2026-03-17 (revised)
**Supersedes:** None

## Context

Samverk originally targeted GitHub as the only issue tracker and forge.
Three operational concerns drove the need for a self-hosted forge:

1. **Autonomy risk** -- a dispatcher running autonomously against GitHub has
   write access to a public-facing service. A Gitea instance on the LAN gives
   the same issue-tracking capability with a smaller blast radius for
   misconfigured automation.

2. **Self-hosted alignment** -- Samverk's design philosophy is self-hosted-first
   (ADR-019). Operating against an external SaaS for core dispatcher state is
   inconsistent with that principle.

3. **Privacy** -- development notes, draft frontmatter, and intermediate agent
   comments that live in issues may not be suitable for a public repository
   until they are ready for review.

The original ADR described a "dual-forge model" where projects were
simultaneously registered on both GitHub and Gitea. In practice, this created
divergence (27 stale Gitea issues that never synced back to GitHub) and
confusion about which forge was authoritative. The 2026-03-17 revision
replaces dual-forge operation with a single-forge-per-project model.

## Decision

### Principle 1: One project, one development forge

Every project has exactly one forge for development -- its source of truth
for code, issues, PRs, CI, and agent dispatch. The MCP, dispatcher, and all
tooling resolve the correct forge from project configuration. There is no
"dual-forge operation" in steady state.

### Principle 2: Forge-agnostic tooling

The MCP exposes one set of tools. Callers never specify or care which forge
backs a project. `set_project("samverk")` resolves to the configured forge
transparently. The forge adapter is a deployment detail in `server.yaml`.

### Principle 3: Gitea is the default for private work

Self-hosted, no rate limits, full control, contained blast radius. Aligns
with ADR-019 (self-hosted first). Public forges (GitHub or others) are used
when a project needs public visibility or community collaboration.

### Principle 4: Public presence is not a development forge

A private project on Gitea may have a public GitHub repo, but it is
informational only: README, project description, license, feature request
templates, discussions. No source code, no CI, no PRs. This is a one-way
curated publish from the development forge, not bidirectional sync.

### Principle 5: Dual-forge registration is a migration capability

The ability to register a project on two forges simultaneously exists to
support transitioning a project from one forge to another while preserving
history (issues, comments, labels, cross-references). It is a transient
state, not a target architecture. Once migration completes, the old
registration is removed or converted to a read-only archive.

### Principle 6: Extensible to N forges

The forge abstraction (`IssueTracker`, `RepoReader`, `RepoWriter`,
`PullRequestManager`) supports adding any forge by implementing the
interfaces. The pattern is: implement, register in `server.yaml`, done. No
MCP or dispatcher changes needed.

### Principle 7: Mirroring is forge infrastructure, not Samverk operations

All forms of public visibility (full code mirror, selective mirror,
informational-only presence) are configured at the forge level -- not managed
by Samverk. Whether Gitea pushes a mirror to GitHub, or a script publishes a
README, that is infrastructure outside Samverk's scope. Samverk interacts
with one forge per project and never pushes code, issues, or artifacts to a
second forge.

### Authentication

Gitea uses Bearer token auth. The token is stored in `GITEA_TOKEN` environment
variable or per-project `gitea_token` field in `server.yaml`. The token for
`samverk-admin` on the self-hosted instance is provisioned separately and not
committed to the repository.

## Consequences

**Positive:**

- Each project has a single source of truth -- no sync, no divergence.
- Dispatcher blast radius is contained to the LAN Gitea instance.
- GitHub can serve as a read-only public presence without development noise.
- Infrastructure is portable -- replacing a forge only requires implementing
  a new adapter and updating `server.yaml`.
- `server.yaml` model generalises to N forges without code changes.

**Negative:**

- Projects migrating between forges have a transition period with two entries.
  This must be time-boxed and cleaned up when migration completes.
- The Gitea instance is a single point of failure for autonomous operation
  (mitigated by fallback to GitHub mode via config change).

## Revision History

- **2026-03-08**: Original "dual-forge operational model" accepted. Described
  dual-registration as steady-state with issue sync between forges.
- **2026-03-17**: Revised to single-forge-per-project model. Dual-registration
  reframed as migration-only capability. Added 7 principles. Removed issue
  sync concept (sync never worked and created divergence).

## Related

- [ADR-013: Forge Abstraction](ADR-013-forge-abstraction.md)
- [ADR-019: Self-Hosted First](ADR-019-self-hosted-first.md)
- [docs/gitea-migration-plan.md](../gitea-migration-plan.md)
- [docs/gitea-actions-compatibility.md](../gitea-actions-compatibility.md)
