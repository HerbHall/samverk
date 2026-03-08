# Label Taxonomy

Samverk uses a structured labeling system to organize issues by lifecycle state, classification, and assignment. This document defines all labels in the repository and their purposes.

## Two-Tier Label Architecture

Labels are split into two tiers to support the DevKit ↔ Samverk boundary:

- **Base labels** (DevKit `project-templates/github-labels.json`): Applied to all projects. Covers type, priority, milestones, and general workflow labels.
- **Overlay labels** (Samverk `overlay/labels.json`): Applied only to Samverk-managed projects. All labels documented below are overlay labels — they are added when a project opts into Samverk lifecycle management.

The machine-readable source of truth for overlay labels is `overlay/labels.json` in this repository. This document provides the human-readable reference with usage guidelines.

## Status Labels

Lifecycle state labels indicate where an issue sits in the dispatcher workflow.

| Label | Color | Description |
|-------|-------|-------------|
| status:needs-human | #d73a4a | Tier 3 action awaiting user decision |
| status:in-progress | #fbca04 | Actively being worked by an agent |
| status:queued | #0075ca | Ready for an agent to claim |
| status:blocked | #e4e669 | Blocked on dependency (not user) |
| status:done | #0e8a16 | Completed work |
| status:claimed | #6f42c1 | Claimed by an agent (work in progress) |
| status:needs-qc | #fd7e14 | Awaiting quality control review |

## Type Labels

Issue classification labels describe the kind of work being done.

| Label | Color | Description |
|-------|-------|-------------|
| type:task | #FBCA04 | Implementation task |
| type:research | #FBCA04 | Research and evaluation |
| type:design | #FBCA04 | Design/specification work |
| type:spike | #FBCA04 | Timeboxed exploration/prototype |

## Priority Labels

Urgency labels indicate how important or time-sensitive an issue is.

| Label | Color | Description |
|-------|-------|-------------|
| priority:critical | #B60205 | Must be done first |
| priority:high | #D93F0B | Important, do soon |
| priority:normal | #E4E669 | Standard priority |

## Agent Labels

Agent assignment labels identify which agent or person should handle the work.

| Label | Color | Description |
|-------|-------|-------------|
| agent:human | #1D76DB | Needs human input |
| agent:dispatcher | #1D76DB | Dispatcher agent work |
| agent:code-gen | #1D76DB | Code generation agent |
| agent:research | #1D76DB | Research agent |
| agent:qc | #1D76DB | Quality control agent |
| agent:docs | #1D76DB | Documentation agent |
| agent:orchestrator | #1D76DB | Orchestration agent |
| agent:test | #1D76DB | Testing agent |

## Complexity Labels

Implementation complexity labels indicate the technical difficulty and resource requirements.

| Label | Color | Description |
|-------|-------|-------------|
| complexity:local | #C5DEF5 | Safe for local model |
| complexity:cloud | #5319E7 | Requires cloud model |
| complexity:ambiguous | #9e5ba8 | Ambiguous or unclear scope |

## Epic Labels

Feature grouping labels organize work into logical epics.

| Label | Color | Description |
|-------|-------|-------------|
| epic:foundation | #0052CC | Project foundation epic |
| epic:dispatcher | #0052CC | Dispatcher agent epic |
| epic:communication | #0052CC | Communication protocol epic |
| epic:frontend | #0052CC | Front-end agent / MCP epic |
| epic:local-agents | #0052CC | Local agent infrastructure epic |
| epic:autonomy | #0E8A16 | Autonomy and trust tier model |
| epic:user-profile | #1D76DB | User profile and persistent preferences |

## Phase Labels

Release phase labels indicate which delivery phase the work belongs to.

| Label | Color | Description |
|-------|-------|-------------|
| phase-1 | #a2eeef | Phase 1 work |
| phase-2 | #a2eeef | Phase 2 work |
| phase-3 | #a2eeef | Phase 3 work |
| phase-4 | #a2eeef | Phase 4 work |

## Workflow Labels

Action type labels classify the kind of change being made.

| Label | Color | Description |
|-------|-------|-------------|
| feat | #0075ca | New feature or enhancement |
| fix | #d73a4a | Bug fix |
| docs | #0075ca | Documentation |
| test | #bfd4f2 | Tests and test infrastructure |
| chore | #e4e669 | Maintenance, refactor, tooling, dependencies |
| bug | #d73a4a | Something isn't working |
| research | #7d5c2f | Research and investigation |

## Default GitHub Labels

These are default GitHub labels maintained for standard issue tracking.

| Label | Color | Description |
|-------|-------|-------------|
| good first issue | #7057ff | Good for newcomers |
| help wanted | #008672 | Extra attention is needed |
| question | #d876e3 | Further information is requested |
| blocked | #808080 | Issue is blocked |

## Label Usage Guidelines

### Status Workflow

Issues progress through status labels as they move through the dispatcher:

1. **status:queued** — Issue ready for an agent to start work
2. **status:claimed** — Agent has claimed the work
3. **status:in-progress** — Agent is actively working
4. **status:blocked** — Work is blocked on a dependency
5. **status:needs-human** — Awaiting human decision before proceeding
6. **status:needs-qc** — Work complete, needs quality control
7. **status:done** — Work completed and verified

### Type + Agent + Priority

All issues should have:

- **One type label** (task, research, design, or spike)
- **One agent label** if assigned (human, dispatcher, code-gen, research, qc, docs, orchestrator, or test)
- **One priority label** (critical, high, or normal)

### Complexity and Phase

Use as needed:

- **Complexity** when the technical difficulty is important context
- **Phase** to group work into release phases
- **Epics** to connect related work across multiple issues

### Workflow Labels

Use conventional labels (feat, fix, docs, test, chore) to mark the nature of the change for changelog/commit organization.
