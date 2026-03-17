# Samverk SDLC Model — Project Lifecycle Phases and Operational Semantics

**Status:** Proposed
**Date:** 2026-03-17
**Author:** Herb Hall (session with Claude, backlog audit)
**Related:** ADR-031 (forge policy), ADR-013 (forge abstraction), ADR-015 (three-tier autonomy)

## Purpose

Define the lifecycle phases for Samverk-managed projects, the operational
semantics of each phase (what agents can and cannot do), and the mechanism
for transitioning between phases. This model governs how projects appear in
the registry, how the dispatcher routes work, and how MCP tools present
project state.

## Background

Samverk manages projects at different maturity levels — from early research
explorations to deployed production services to end-of-life maintenance. Without
lifecycle awareness, the dispatcher treats all projects identically: any issue
on any project can be dispatched to any agent type. This creates risk:

- Research projects getting code-gen agents that produce premature implementations
- Maintenance projects accepting feature work that should be deferred
- Inactive projects consuming dispatcher cycles

The standard Software Development Life Cycle (SDLC) defines seven phases:
Planning, Requirements Analysis, Design, Development, Testing, Deployment,
and Maintenance. Samverk adapts this model for its agent-driven, self-hosted
context.

## Lifecycle Phases

Seven phases, ordered but not strictly linear. A project can move forward,
backward (e.g., deployed → development for a V2), or jump to inactive from
any phase.

### research

**SDLC mapping:** Pre-planning exploration.

The project is an idea being investigated. Agents produce research documents,
feasibility assessments, competitive analysis, and concept briefs. No code
is produced. No PRs are created.

| Aspect | Behavior |
|--------|----------|
| Dispatcher | Watches issues. Dispatches research/docs agents only. |
| Allowed agents | research, docs, feasibility, ideation |
| Blocked agents | code-gen, test, qc, infra |
| PRs | Not created by agents |
| CI | Not required |

### planning

**SDLC mapping:** Planning + Requirements Analysis.

The project has passed initial research and is being scoped. Agents produce
architecture documents, requirement specifications, issue decomposition,
and design spikes. Limited code-gen is allowed for proof-of-concept spikes
gated by a `type:spike` label.

| Aspect | Behavior |
|--------|----------|
| Dispatcher | Research + architecture spikes |
| Allowed agents | research, docs, feasibility, ideation, code-gen (type:spike only) |
| Blocked agents | test, qc (no production code to test) |
| PRs | Only for spikes/prototypes |
| CI | Optional |

### design

**SDLC mapping:** Design.

Architecture is being formalized. Agents produce design documents, ADRs,
prototypes, database schemas, API specifications, and wireframes. Code-gen
is allowed for proof-of-concepts and scaffolding.

| Aspect | Behavior |
|--------|----------|
| Dispatcher | Research + design + prototyping |
| Allowed agents | research, docs, feasibility, code-gen (prototypes) |
| Blocked agents | None explicitly, but feature work is premature |
| PRs | Allowed for prototypes and scaffolding |
| CI | Should be configured by end of this phase |

### development

**SDLC mapping:** Development + Testing (parallel).

Full active development. All agent types are dispatched. Feature work, bug
fixes, testing, documentation, and QC all run concurrently. This is the
primary operational phase for most projects.

| Aspect | Behavior |
|--------|----------|
| Dispatcher | Full dispatch. All agent types. |
| Allowed agents | All |
| Blocked agents | None |
| PRs | Full PR workflow |
| CI | Required |

### deployed

**SDLC mapping:** Deployment + Active support.

The project is in production and serving users. Development continues but
with a stability bias. Priorities shift: security > bugs > dependency
updates > user feedback > features. Feature work is accepted but not
prioritized unless explicitly marked `priority:high` or above.

| Aspect | Behavior |
|--------|----------|
| Dispatcher | Full dispatch, stability-biased triage |
| Allowed agents | All |
| Blocked agents | None, but triage deprioritizes feat |
| PRs | Full PR workflow |
| CI | Required |

### maintenance

**SDLC mapping:** Maintenance (reduced scope).

No new features. The project receives only critical fixes: breaking bugs,
security vulnerabilities, and dependency replacements when a dependency is
no longer viable. Feature issues are parked or rejected by the dispatcher.

| Aspect | Behavior |
|--------|----------|
| Dispatcher | Critical fixes only |
| Allowed agents | code-gen (fix/security/chore only), test, docs, qc |
| Blocked agents | code-gen for feat, ideation, research |
| PRs | Fix/security/chore PRs only |
| CI | Required |

### inactive

**SDLC mapping:** End of life.

The project is no longer updated. Not watched by the dispatcher. Not
dispatched. Listed in MCP tools for reference only (project history,
documentation access). Can be revived to any phase for a V2+ or major
refactor.

| Aspect | Behavior |
|--------|----------|
| Dispatcher | Not watched. Not dispatched. |
| Allowed agents | None |
| Blocked agents | All |
| PRs | None |
| CI | N/A |

## Phase Transitions

### Rules

1. **Any-to-any transitions are valid.** A project can move forward
   (research → planning), backward (deployed → development for V2),
   or jump (any → inactive, inactive → any).

2. **Transitions are Tier 3 actions.** Every phase change requires human
   approval, regardless of interface (MCP tool, dashboard, CLI).

3. **Transitions are documented.** Each transition records: previous phase,
   new phase, who authorized, timestamp, and a reason string. Stored in
   the project's session log and optionally in Synapset.

4. **The `set_project_phase` MCP tool** presents the transition for
   approval, validates the phase name, records the reasoning, and updates
   the runtime registry. Changes are persisted to `server.yaml` on disk.

### Typical Progression

```text
research → planning → design → development → deployed → maintenance → inactive
                                    ↑                         |
                                    └─────────────────────────┘
                                         (V2 / major refactor)
```

### Revival

An inactive project can be revived to any phase. Common scenarios:

- **inactive → development**: V2 rewrite or major refactor
- **inactive → research**: Revisiting a shelved idea with new technology
- **maintenance → development**: User demand justifies new features

## Tags

Free-form metadata for filtering and agent context. Tags do not control
dispatcher behavior — phase does that.

### Tag Conventions

| Category | Examples | Purpose |
|----------|----------|---------|
| Language | `lang:go`, `lang:python`, `lang:rust` | Agent prompt selection |
| Stack | `stack:mcp`, `stack:cli`, `stack:web` | Architecture context |
| Visibility | `private`, `public` | Intent (forge config enforces) |
| Domain | `domain:networking`, `domain:memory` | Topical grouping |

Tags use `key:value` format. No schema enforcement — convention emerges
from use. Any tag can be added at any time.

### Tag Uses

- **MCP filtering**: `list_projects --tag=lang:go` or `--phase=development`
- **Agent context**: Project tags are included in agent prompts so agents
  know the stack, conventions, and domain
- **Dashboard grouping**: Visual organization by domain or stack

## server.yaml Schema

```yaml
projects:
  - name: samverk
    owner: samverk
    repo: samverk
    forge: gitea
    gitea_url: http://192.168.1.160:3000
    phase: development
    tags: [lang:go, private, domain:orchestration]
```

### Validation

- `phase` must be one of: research, planning, design, development,
  deployed, maintenance, inactive
- `tags` is optional, defaults to empty list
- Unknown phase values are rejected at load time
- Missing `phase` defaults to `development` (backward compatibility)

## Code Changes Required

| Component | Change |
|-----------|--------|
| `ProjectConfig` struct | Add `Phase string`, `Tags []string` |
| `LoadProjectConfig` | Validate phase enum, parse tags |
| `Project` struct | Expose phase + tags to MCP handlers |
| `list_projects` tool | Include phase, tags in output |
| New `set_project_phase` tool | Tier 3 transition with reason logging |
| `get_digest` tool | Group by phase, skip inactive |
| `list_open_prs` tool | Skip inactive by default |
| Dispatcher routing | Check phase, enforce agent type restrictions |
| Dashboard | Phase badge, transition button |

## Extensibility

Adding a new phase requires:

1. Add the phase name to the validation enum in `LoadProjectConfig`
2. Define dispatcher behavior in the phase→allowed-agents mapping
3. Update this document

The phase→behavior mapping should be data-driven (config table or code
constant), not per-phase logic scattered across the codebase. This ensures
adding a phase is a one-line change plus documentation, not a code audit.

## Relationship to Existing Concepts

- **Samverk project lifecycle** (`docs/project-lifecycle.md`): Defines 7
  detailed phases. The registry phases are a simplified operational subset
  that gate dispatcher behavior, not human methodology.
- **DevKit METHODOLOGY.md**: Defines 6 methodology phases. Complementary.
  DevKit phases guide human work; Samverk phases gate automated dispatch.
- **Autonomy model** (`docs/autonomy-model.md`): Orthogonal. Phase
  constrains *which work* happens. Autonomy constrains *how work is
  approved*. Both apply simultaneously.
- **Forge policy** (ADR-031 revision, #616): Phase is above the forge
  abstraction. A project's phase is the same regardless of forge.

## References

- Standard SDLC: Planning → Analysis → Design → Development → Testing →
  Deployment → Maintenance
- [ADR-031: Forge Policy (being revised)](#616)
- [ADR-013: Forge Abstraction](decisions/ADR-013-forge-abstraction.md)
- [ADR-015: Three-Tier Autonomy](decisions/ADR-015-three-tier-autonomy.md)
- [Project Lifecycle](project-lifecycle.md)
