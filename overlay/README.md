# Samverk Overlay for DevKit

## What This Is

The Samverk overlay is an optional addon that transforms a plain DevKit project
into a Samverk-managed project with lifecycle tracking, agent routing, and
coordination layer integration.

**Without the overlay:** A project uses DevKit's base labels, methodology, and
tooling. It works standalone.

**With the overlay:** A project gains lifecycle phase tracking, agent assignment
labels, status workflow labels, dispatcher routing, and registration with
Samverk's coordination layer.

## How to Apply

### During scaffolding (new project)

Run DevKit's Kit 3 with the `--samverk` flag:

```powershell
pwsh -File setup/new-project.ps1 --samverk
```

Or select "Register with Samverk? [y/N]" when prompted during the
`devkit-sync` new-project workflow.

### To an existing project

Run the `devkit-sync` apply-samverk workflow:

```text
/devkit-sync → 13 (Apply Samverk overlay)
```

This adds overlay labels, creates `.samverk/project.yaml` and
`.samverk/status.md`, and optionally registers with the coordination layer.

## What the Overlay Contains

| File | Purpose |
|------|---------|
| `labels.json` | Samverk-specific labels added on top of DevKit's base set |
| `templates/project.yaml.template` | Template for `.samverk/project.yaml` |
| `templates/status.md.template` | Template for `.samverk/status.md` |
| `agents/ideation.md` | Agent template for idea intake and synthesis |
| `agents/feasibility.md` | Agent template for technical assessment |
| `agents/legal.md` | Agent template for trademark/licensing research |

## The Contract

- **Samverk defines** what the overlay contains (this directory).
- **DevKit applies** the overlay mechanically (Kit 3, devkit-sync workflows).
- **Dependency direction:** Samverk → DevKit (one-way). DevKit never depends
  on Samverk. The overlay is optional — DevKit works without it.

## Label Architecture

Labels are split into two tiers:

1. **Base labels** — Defined in DevKit's `project-templates/github-labels.json`.
   Applied to all projects. Covers type (`feat`, `fix`, `chore`), priority
   (`priority:critical` through `priority:low`), milestones, and general workflow.

2. **Overlay labels** — Defined in `overlay/labels.json` (this directory).
   Applied only to Samverk-managed projects. Covers agent types, status workflow,
   complexity routing, and lifecycle phases.

The base + overlay sets are disjoint — no label appears in both files.

## Related Documents

- [DevKit boundary contract](https://github.com/HerbHall/devkit/blob/main/docs/samverk-boundary.md)
- [Communication Protocol](docs/communication-protocol.md) — issue schema and label taxonomy
- [Label Taxonomy](docs/label-taxonomy.md) — canonical label definitions
- [Project Lifecycle](docs/project-lifecycle.md) — the 7-phase lifecycle
- [ADR-017: DevKit as Reference Implementation](docs/decisions/ADR-017-devkit-reference.md)
