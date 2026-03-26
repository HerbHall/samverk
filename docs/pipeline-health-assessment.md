# Pipeline Health Assessment

**Date:** 2026-03-26
**Period analyzed:** 168h (7 days)
**Total failures:** 489

## Executive Summary

71% of pipeline failures were caused by two infrastructure bugs now fixed
(CLAUDE.md corruption PR #297/#314/#315, stream-json buffering PR #298).
The remaining 29% reveal structural gaps in issue preparation, agent context,
quality evaluation, and feedback loops.

Grades: Issue Preparation (B-), Agent Runtime (B), Quality Gates (D),
Continuous Improvement (F).

## Failure Breakdown

| Class | Count | % | Status |
|-------|-------|---|--------|
| timeout | 181 | 37% | Fixed (PR #298 stream-json) |
| unknown | 167 | 34% | Fixed (PRs #297, #314, #315 CLAUDE.md) |
| post_process | 74 | 15% | Real quality issues -- agent code doesn't compile |
| provider_error | 29 | 6% | Rate limits, model not found |
| permanent | 20 | 4% | Bad frontmatter, budget exceeded |
| classify | 12 | 2% | Agent type validation failures |
| provider_down | 6 | 1% | Connection errors |

## Finding 1: Issue Preparation

**Score: B- (83% well-formed, critical gaps)**

### Strengths

- 100% have acceptance criteria checklists
- 100% have proper frontmatter (type, agent_type, priority)
- 70% have `file_context` pointing agents to relevant files
- Most reference related ADRs and issues

### Gaps

- Schema v1.1.0 is undocumented (3 new fields in use but not in
  `communication-protocol.md`)
- 0% have complexity labels (dispatcher can't route by compute)
- 30% lack `file_context` (agents explore blind, wasting tokens)
- 40% lack `constraints` (agents don't know what NOT to do)
- 40% lack `estimated_tokens` (cost prediction impossible)
- 17% are low-quality (vague descriptions, no technical notes)

## Finding 2: Agent Runtime

**Score: B (solid execution, weak context)**

### Strengths

- Workspace isolation with worktrees
- Explore phase reads CLAUDE.md + referenced files before prompting
- Synapset memory enrichment for relevant patterns
- Build + test validation before committing
- EDIT block format validation catches Ollama prose
- Checkpoint/resume for interrupted sessions

### Gaps

- Agent doesn't know its time budget (timeout not communicated)
- Explorer ignores `file_context` from frontmatter YAML (only reads
  regex matches from issue body text)
- No plan-before-code step for complex issues
- `agent:docs` routes to Ollama triage (100% failure rate)
- Retry gets error messages but no strategy guidance
- Agent doesn't know provider capabilities (CLI vs API flow)

## Finding 3: Quality Gates

**Score: D (instrumented but disconnected)**

### Strengths

- Failure events classified into 13 categories
- Per-issue failure counters with escalation threshold
- Correction engine retries with timeout/provider adjustments
- KPI framework (FTFR, recurrence rate, MTTR)

### Gaps

- Quality gate is **log-only** -- no action on failure
- No quality result visible on the issue
- Gate checks length and code blocks, not acceptance criteria
- No partial success state (80% done = same as 0%)

## Finding 4: Continuous Improvement

**Score: F (data collected, never acted on)**

### The broken loop

```text
Failure -> Classify -> Persist -> Advisor detects pattern -> Recommendation -> /api/v1/recommendations
                                                                                      |
                                                                               (nobody reads this)
```

### Missing feedback loops

| What happens | What should happen |
|---|---|
| Agent fails 3x on same issue | Prompt should include prior failure context |
| Provider has 40% failure rate | Router should deprioritize that provider |
| Same build error across 5 issues | Advisory should auto-file systemic fix |
| Quality gate fails | Issue should get label for human review |
| Docs agent on Ollama always fails | Route docs to Haiku fallback |
| Agent code doesn't compile | Next attempt should include compile errors |

## Implementation Plan

### Wave 0: Pipeline Prerequisites (manual, blocks all others)

| # | Fix | Impact | Effort |
|---|-----|--------|--------|
| 0a | Route docs to default chain | 100% docs failure -> fallback to Haiku | 1 line |
| 0b | Feed file_context from frontmatter into explorer | 30% more context for agents | Small |
| 0c | Tell agent its time budget in prompt | Better prioritization | Small |
| 0d | Make quality gate actionable (label + comment) | Visibility into output quality | Small |
| 0e | Include prior failure context in retries | Smarter retries | Medium |

### Wave 1: Issue Quality (dispatched to agents)

- Update communication-protocol.md to v1.1.0
- Enforce file_context/constraints in dispatcher pre-flight
- Add complexity labels to issue templates
- Document schema completeness requirements

### Wave 2: Feedback Loops (dispatched, depends on Wave 1)

- Auto-populate RCA category from error patterns
- Wire advisor recommendations to automated actions
- Track decomposition outcomes
- Acceptance criteria verification in quality gate
- Cross-issue pattern learning
- Post-failure retrospective automation

## References

- ADR-027: Failure recovery strategy
- ADR-030: Cross-model QA validation
- PRs #297, #298, #313, #314, #315: Infrastructure fixes (this session)
- Issue #492: Multi-machine routing (referenced in quality.go)
