# ADR-039: Two-Location Centralization Rule

**Status:** Accepted
**Date:** 2026-03-26
**Supersedes:** None
**Related:** ADR-035 (solo developer agent model), docs/agent-output-format.md

## Context

During a pipeline health audit (2026-03-26), we discovered that agent output
format expectations were defined independently in 6 different files. The edit
block parser accepted `EDIT <file>`, the quality gate checked `EDIT:`, and
each agent prompt had its own hand-written format instructions. This caused
real production failures:

- Issue #286: Agent produced valid `EDIT renovate.json` output that the
  quality gate flagged as "no code blocks" (score 0.2) because it checked
  for `EDIT:` with a colon
- Multiple issues received false positive truncation alerts because the
  quality gate and the stream parser had different truncation markers

The root cause in every case was the same: a value, format, or behavioral
contract was defined in 2+ locations that drifted apart over time.

This problem is amplified in samverk's autonomous agent pipeline because:

1. Agents read the prompt but cannot verify against the validator. Any
   mismatch between instruction and validation is a guaranteed failure.
2. The pipeline has 6 stages (issue creation, dispatch, prompting, execution,
   validation, quality gate). A value in 2 stages today spreads to 4 as
   features are added.
3. Autonomous agents writing code will copy patterns they see. Duplicated
   constants in the codebase teach agents to duplicate further.

## Decision

**If a value, format, or behavioral contract appears in 2 or more locations,
it must be extracted to a single authoritative source.**

This applies to:

| Pattern | Example | Centralization Target |
|---------|---------|----------------------|
| String constants | Label names, status values, error messages | `pkg/models/` constants or generated code |
| Behavioral contracts | "output must contain EDIT blocks" | Shared function in the relevant package |
| Format specifications | EDIT block format, PR title format | Single function referenced by prompt + validator |
| Configuration values | Timeouts, thresholds, limits | Config file read at startup, never hardcoded |
| Regex patterns | File path matching, frontmatter parsing | Compiled regex in shared package |
| API contracts | Request/response shapes | Shared types in `pkg/models/` |

### Enforcement

- **Code review gate**: Any PR that introduces a second copy of a value
  already defined elsewhere must extract it to a shared location. The
  reviewer (human or agent) must check for this.
- **Agent prompt instruction**: The CLAUDE.md and system prompts must
  include: "Before defining a constant, format string, or behavioral
  check, search for an existing definition. If one exists, import it."
- **Lint opportunity**: A future static analysis check could detect
  duplicate string literals across packages.

### Exceptions

- **Test fixtures**: Test files may duplicate values for test isolation.
  A test that imports production constants is fine; a test that hardcodes
  a value for readability is also fine.
- **Documentation**: Docs may repeat values for clarity. The doc is not
  a source of truth -- the code is.
- **Comments**: Explaining a value in a comment near its usage is not
  duplication.

## Alternatives Considered

### Three-location threshold

Allow 2 copies, centralize at 3+. Rejected because:

- Every format inconsistency found in the pipeline audit was a 2-location
  problem (parser vs quality gate, prompt vs validator)
- By the time a third copy appears, the first two have already diverged
- The cost of centralizing early is low (a constants file + shared function)

### No formal threshold (developer judgment)

Let developers decide case-by-case. Rejected because:

- Autonomous agents cannot exercise judgment about centralization
- The pipeline audit proved that drift happens silently and causes
  production failures that are hard to diagnose
- A clear rule is enforceable; "use good judgment" is not

## Consequences

### Positive

- Format inconsistencies like #286 become impossible (single source of truth)
- Changing a value requires updating one location, not hunting through files
- Agents receive instructions that match what validators check
- Codebase teaches good patterns to code-gen agents

### Negative

- Slightly more upfront work per feature (find existing definition or create shared one)
- More cross-package imports (acceptable trade-off for consistency)
- Shared packages become larger over time (mitigated by keeping them focused)

### Neutral

- Existing violations need a one-time audit and fix (tracked in issue)
- New contributors need to learn the rule (documented in CLAUDE.md)

## Compliance

This is the reference list of known centralized sources:

| Domain | Source of truth | Consumers |
|--------|----------------|-----------|
| Labels | `overlay/labels.json` + `pkg/models/labels_gen.go` (#247) | Dispatcher, digest, API, dashboard, PC agent |
| Output format | `internal/agent/format.go` | Prompts, parser, validator, quality gate |
| Issue schema | `pkg/models/issue.go` (IssueFrontmatter) | Dispatcher, runner, MCP tools |
| Failure classes | `pkg/models/failure.go` | Classifier, store, API, advisor |
| Provider config | `.samverk/providers.yaml` | Registry, runner, health monitor |
| Pipeline stages | `internal/store/pipeline.go` | Store, API, dashboard |

New domains should be added to this table when centralized.
