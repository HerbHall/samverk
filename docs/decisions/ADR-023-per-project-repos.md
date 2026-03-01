# ADR-023: Per-Project Repos with Coordination Layer

## Status

Accepted

## Context

Samverk manages multiple unrelated projects simultaneously (Samverk itself, DockPulse, PacketDeck, Subnetree, RunNotes, future ideas). Several architectural questions arise:

1. Do projects share a common issue tree, or does each project have its own?
2. Is Samverk's own development a project under the Samverk framework?
3. How does cross-project coordination work (priorities, resource allocation, check-ins)?
4. How does the Gitea (development) to GitHub (release) migration path work?

Three options were considered:

### Option A: Shared Issue Tree

All projects share one issue tracker (Samverk's Gitea instance). Issues tagged by project label.

Pros:

- Single view of everything
- Simple setup (one repo to watch)
- Easy cross-project queries

Cons:

- Noisy — unrelated projects pollute each other's issue lists
- Issue numbers are meaningless across projects (RunNotes #14 vs DockPulse #14)
- Cannot migrate a project to GitHub without disentangling its issues
- Doesn't scale — 5 active projects with 50 issues each = 250 issues in one tracker
- Violates the forge abstraction (ADR-013) by coupling projects to a single forge instance
- Projects are not portable

### Option B: Fully Isolated Projects

Each project exists independently with no coordination layer. The user provides all cross-project intelligence manually.

Pros:

- Simplest setup
- No framework overhead
- Each project is completely portable

Cons:

- Cross-project coordination lives in the user's head (the exact failure mode Samverk exists to solve)
- No unified check-in digest across projects
- No resource allocation across projects
- "I'll forget" problem is guaranteed at scale

### Option C: Per-Project Repos with Coordination Layer (Selected)

Each project has its own repo and issue tracker on whatever forge it needs. Samverk maintains a coordination layer — a registry of managed projects and cross-project state — in its own infrastructure (database + optionally its own issue tracker).

Pros:

- Projects are fully self-contained and portable
- Each project can live on any forge (Gitea, GitHub, GitLab) independently
- Issue numbers are project-scoped and meaningful
- Migration from Gitea to GitHub is a repo-level operation, not an issue-extraction surgery
- Samverk's own development is just another managed project (dogfooding)
- Cross-project intelligence (priority, phase, resource allocation) lives in the coordination layer
- Scales cleanly — adding a project means registering it, not restructuring

Cons:

- Dispatcher must poll/watch multiple repos across potentially different forges
- Cross-project dependencies need a coordination mechanism outside any single project's issues
- More moving parts than a single-repo approach
- Project registration and template enforcement add setup overhead

## Decision

Each project managed by Samverk is a self-contained repository with its own issue tracker, on whatever forge is appropriate for its lifecycle stage. Samverk maintains a coordination layer for cross-project concerns.

### Project Structure

```text
Samverk Coordination Layer
├── Project Registry (which projects, where, what phase)
├── Cross-Project State (priorities, resource allocation, phase tracking)
├── Check-In Digest (aggregates status from all registered projects)
└── Lifecycle Pipeline (idea intake through delivery, per-project)

Managed Projects (each self-contained)
├── Samverk (Gitea → GitHub at release)
│   ├── repo: own code, own issues, own branches
│   └── .samverk/: project-level config
├── DockPulse (Gitea → GitHub at release)
│   ├── repo: own code, own issues, own branches
│   └── .samverk/: project-level config
├── PacketDeck (Gitea → GitHub at release)
│   └── ...
├── Subnetree (GitHub — already public)
│   └── ...
└── [Future projects]
    └── ...
```

### Samverk Is a Managed Project

Samverk's own development is registered as a project under its own framework. This means:

- Samverk's backlog is managed through its own issue tracker
- Samverk's agents can work on Samverk's own code
- The lifecycle pipeline (intake → research → execution → delivery) applies to Samverk features
- The check-in digest includes Samverk's own status alongside other projects

This is the ultimate dogfooding: the framework manages its own development. Any friction in the process is discovered firsthand.

### Coordination Layer Responsibilities

The coordination layer does NOT duplicate project-level data. It provides cross-cutting intelligence:

| Concern | Where It Lives | Why |
|---------|---------------|-----|
| Task issues | Project's own repo | Self-contained, portable |
| Code and branches | Project's own repo | Standard git workflow |
| Project-level config | Project's `.samverk/` dir | Travels with the repo |
| Labels and schema | Project's own issue tracker | Enforced by Samverk templates |
| Cross-project priority | Coordination layer (SQLite) | No single project owns this |
| Resource allocation | Coordination layer (SQLite) | Token budgets span projects |
| Lifecycle phase | Coordination layer (SQLite) | Idea pipeline is pre-repo |
| Check-in digest | Coordination layer (generated) | Aggregates all projects |
| Project registry | Coordination layer (config) | Maps projects to forges |
| Idea briefs (pre-repo) | Coordination layer or local files | Ideas exist before repos do |

### Forge Migration Path

The development-to-release migration (Gitea → GitHub) is a standard git operation at the repo level:

1. Development happens on Gitea (self-hosted, no rate limits, full control)
2. When a project reaches the delivery phase (Phase 7), the repo is pushed to GitHub
3. The Samverk project registry is updated to point to the new remote
4. Issues can be migrated via Samverk's forge abstraction (read from Gitea, write to GitHub) or started fresh on GitHub if the project is launching publicly

The forge abstraction (ADR-013) already supports this — the `IssueTracker` interface works with both GitHub and Gitea implementations. The dispatcher doesn't care which forge a project uses; it reads and writes through the interface.

Projects that are already public (like Subnetree on GitHub) stay where they are. The forge choice is per-project, not system-wide.

### Project Template

Every managed project gets a standard `.samverk/` directory:

```yaml
# .samverk/project.yaml
name: dockpulse
forge: gitea
forge_url: https://gitea.local/HerbHall/DockPulse
phase: scaffold
registered: 2026-03-01
labels_synced: true
autonomy_overrides: {}
```

This file is the project's registration with Samverk. It travels with the repo, so even if the coordination layer is rebuilt, projects can re-register from their own config.

## Consequences

- The dispatcher must support watching multiple repos across multiple forges simultaneously
- A project registry (config file or database table) must be implemented
- The check-in digest aggregator must query multiple issue trackers and merge results
- Idea briefs (Phase 1) exist before a repo does — they live in the coordination layer until a repo is created at Phase 5 (Scaffold)
- The forge migration path is a standard operation, not a special case
- Samverk developing itself creates a bootstrap dependency — the framework must be functional enough to manage its own backlog before it can fully manage other projects

## References

- [ADR-013: Abstract Git Forge Behind Interface](ADR-013-forge-abstraction.md)
- [Project Lifecycle](../project-lifecycle.md)
- [Multi-Session Safety](../multi-session-safety.md)
- [Communication Protocol](../communication-protocol.md)
- [Architecture](../architecture.md)
