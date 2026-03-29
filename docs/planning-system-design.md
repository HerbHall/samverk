# Planning System Design

**Status:** Approved v1.0
**Date:** 2026-03-28
**Origin:** Design session resolving architectural decisions for issues #214, #215, #239, #242, #245
**Parent:** [production-pipeline-design.md](production-pipeline-design.md) (Divisions 2-3)

## Problem

Five planning system issues are blocked on architectural decisions. The production pipeline design doc (Draft v0.9) defines Divisions 2-3 at a high level but leaves implementation sequencing and scope boundaries unresolved. This doc captures the decisions that unblock pipeline-ready work.

## Dependency Graph

```text
Wave 1 (parallel, no inter-dependencies):
  #239 -- Capability profiles schema + Go types
  #242 -- Readiness review (dispatcher pre-filter)
  #215 -- Research routing in dispatcher

Wave 2 (after Wave 1 merges):
  #214 -- Planning agent (needs #239 profiles, #215 research handoff)

Wave 3 (after operational data accumulates):
  #245 -- Continuous improvement engine
```

## Decision 1: Readiness Review Is a Dispatcher Pre-Filter, Not an Agent

**Issue:** #242
**Context:** The production pipeline design describes Stage 1 as a "readiness review agent." The six checks it performs are mostly deterministic.

**Decision:** Implement readiness review as a `readinessCheck()` function in the dispatcher, not as a separate agent type. No LLM call for the fast path.

**Rationale:** The six checks map to existing capabilities:

| Check | Implementation | LLM needed? |
|-------|---------------|-------------|
| Already implemented? | Title similarity against recently closed issues | No -- fuzzy string match |
| Issue body still accurate? | `os.Stat()` on `file_context` paths | No |
| Dependencies satisfied? | Check `depends_on` issues are closed via forge API | No |
| Agent type label correct? | Existing `classifyByHeuristic()` in router.go | No |
| Duplicate? | Title/body similarity against open issues | No -- fuzzy match |
| Scope appropriate? | Existing `classifyComplexity()` in complexity.go | No |

**Ambiguous cases only** (e.g., duplicate confidence 0.4-0.8) escalate to an LLM triage call using the existing `triage` provider chain. This saves tokens on the 80%+ of issues that are clearly ready or clearly not.

**Output actions** (unchanged from #242 spec):

- `ready` -- advance to planning/dispatch
- `close` -- already done (evidence comment posted)
- `needs-human:decision` -- ambiguous (specific question attached)
- `needs-revision` -- body outdated (comment with what's wrong)

**Metrics:** Log all readiness results to `pipeline_events` with stage `readiness_review` (schema from #240).

## Decision 2: Research Routing Uses Per-Issue Phase Gating

**Issue:** #215
**Context:** The original spec says "Research agent blocked by dispatcher when project phase is `development` or later." But `.samverk/project.yaml` has `phase: execution`, which would block research for samverk itself.

**Decision:** Phase gating for research is per-issue, not per-project. An issue with `agent:research` label routes to the research chain regardless of project phase. The project phase gate only blocks the automatic *creation* of research issues by the planning agent (Division 3) -- it does not block human-created or planner-created research issues from executing.

**Rationale:** Research is needed at any project phase. A production-phase project still needs research for new features. The phase gate exists to prevent the planning agent from spinning up expensive deep-research for issues during a code freeze, not to prevent all research.

**Scope reduction for #215:** The issue currently specifies both routing (dispatcher change) AND research agent behavior (producing findings, 3 tiers). These are separable:

- **#215 scope (Wave 1):** Dispatcher routing only. Add `agent:research` to `selectProviderKey()` mapping. Map research tiers to provider chains: `research:quick` -> `triage`, `research:standard` -> `default`, `research:deep` -> `complex`. Remove per-project phase blocking.
- **Research agent prompts and behavior:** Already partially implemented in `agent/prompts.go` (`buildResearchPrompt`). The structured output format (`## Research Findings` section) and QC handoff to `agent:planner` are part of #214's scope, since the planner defines what research output it consumes.

## Decision 3: Capability Profiles Are Config-Only, Consumed by Planner

**Issue:** #239
**Context:** The planning agent needs to know provider capabilities for cost-optimized decomposition.

**Decision:** No changes needed to #239's spec -- it's already well-scoped. The issue is pipeline-ready as-is with `handoff_ready: true`.

**Key points:**

- `.samverk/agent-profiles.yaml` is a static config file, not a runtime database
- `MatchCapability()` is called by the planning agent (#214), not by the dispatcher
- The dispatcher continues to use `selectProviderKey()` for routing -- profiles don't change that
- `reliability` field starts with estimated values; #245 (continuous improvement) updates them from execution data later

## Decision 4: Planning Agent Consumes Both Profiles and Research

**Issue:** #214
**Context:** The planning agent is the core of Division 3. It needs both capability profiles (#239) and research findings (#215) as inputs.

**Decision:** #214 is blocked until #239 merges. It is NOT blocked on #215 -- research findings are optional input ("if Division 2 was executed -- may be absent for simple issues" per the issue spec).

**Minimum viable planner:**

1. Read parent issue body + acceptance criteria
2. Query Synapset for prior decomposition patterns
3. Load capability profiles from `.samverk/agent-profiles.yaml`
4. Decompose into child issues with complete v1.1.0 frontmatter
5. Match each child to cheapest capable provider via `MatchCapability()`
6. If confidence < threshold, escalate with typed reason

The planner does NOT need to be perfect on day one. The escalation-and-learn loop (Synapset pattern storage) means it improves with each human resolution.

## Decision 5: Continuous Improvement Is Deferred

**Issue:** #245
**Context:** Pattern detection, profile updates, rules promotion, and feedback loops.

**Decision:** Defer until Waves 1-2 are operational and producing data. #245 needs:

- Readiness gap data (from #242 running in production)
- Planning escalation data (from #214 running in production)
- Provider reliability data (from execution sessions using #239 profiles)

Without real data, the continuous improvement engine would have nothing to improve. Target: 2-4 weeks after Wave 2 ships, once enough pipeline events accumulate.

## Implementation Sequencing

### Wave 1 (parallel, unblock now)

| Issue | Scope | Pipeline-ready? |
|-------|-------|----------------|
| #239 | Capability profiles YAML + Go types + `MatchCapability()` | Yes -- relabel to `status:queued` |
| #242 | `readinessCheck()` function in dispatcher, 4 output actions, metrics | Yes after rewrite |
| #215 | Dispatcher routing for `agent:research`, tier-to-chain mapping | Yes after rewrite |

### Wave 2 (after Wave 1 merges)

| Issue | Scope | Blocked on |
|-------|-------|-----------|
| #214 | Planning agent: decomposition, Synapset patterns, profile matching | #239 (profiles) |

### Wave 3 (after operational data)

| Issue | Scope | Blocked on |
|-------|-------|-----------|
| #245 | Pattern detection, profile updates, rules promotion | All of Wave 1 + Wave 2 |

## References

- [production-pipeline-design.md](production-pipeline-design.md) -- Parent design (Divisions 1-5)
- [communication-protocol.md](communication-protocol.md) -- Schema v1.1.0, label taxonomy
- [project-lifecycle.md](project-lifecycle.md) -- Phase definitions
- `internal/dispatcher/router.go` -- Existing routing logic
- `internal/dispatcher/complexity.go` -- Existing complexity estimation
- `internal/store/pipeline.go` -- Pipeline event stages (#240)
