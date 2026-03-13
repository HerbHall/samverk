# Samverk Production Pipeline Design

**Status:** Draft v0.9
**Date:** 2026-03-13 (v0.1: 2026-03-11)
**Origin:** Claude session -- design discussion starting from Anthropic Code Review analysis

## Context

Anthropic launched Claude Code Review (March 9, 2026) -- a multi-agent PR review system for Teams/Enterprise. GitHub-only, $15-25/review, not available for individual users or Gitea. Rather than waiting for platform support, we are designing our own pipeline-native review and quality system that works across both forges and aligns with Samverk's dispatch architecture.

Key insight from the discussion: PR review is not a standalone feature -- it is one quality gate in a larger production pipeline. Every stage of work needs the same work --> verify --> rework loop.

## Pipeline Architecture

### Manufacturing Model

The Samverk pipeline operates like a production line with **divisions**. Each division contains one or more **departments** that may run in parallel. Every division has an embedded **quality gate** that enforces pass/fail criteria before work advances to the next division.

### Universal Pattern (Every Division)

```text
Work --> Verify --> Pass --> Next Division
                --> Fail --> Rework (loop back to departments)
                --> Stuck --> Escalate (needs-human --> learn --> update rules)
```

## Division 1: Intake

- **Purpose:** Ideas, bugs, feature requests enter the system
- **Departments:** Triage, Classification, Deduplication
- **Quality Gate:** Is the issue well-formed and actionable?
  - PASS --> Classified issue with priority
  - FAIL --> Returned for clarification or rejected
  - STUCK --> Escalate for human review

**Cross-reference:** This division maps to [project-lifecycle.md](project-lifecycle.md) Phase 1 (Intake). The lifecycle defines idea capture, ideation agent templates, and the `phase:intake` label. For existing projects, feature requests and bug reports enter here and may skip Divisions 2-3 if the scope is small enough (single-file fix, clear acceptance criteria already present).

**Existing implementation:** The dispatcher's `classify()` function (`internal/dispatcher/router.go`) validates frontmatter and applies heuristic classification. Issues failing validation receive `status:needs-human` escalation. The communication protocol (`docs/communication-protocol.md`) defines the YAML frontmatter schema, label taxonomy, and 12 agent types.

## Division 2: Research

- **Purpose:** Feasibility, context gathering, dependencies, prior art
- **Departments:** Codebase Analysis, External Docs, Dependency Mapping
- **Quality Gate:** Are findings sufficient to inform design?
  - PASS --> Research findings package
  - FAIL --> Gaps identified, more research needed
  - STUCK --> Escalate for human review

**Cross-reference:** This division maps to [project-lifecycle.md](project-lifecycle.md) Phases 2-3 (Research and Go/No-Go Gate). The lifecycle defines three research tiers (Quick/Standard/Deep), feasibility and legal agent templates, and the `phase:research` label.

**Existing implementation:** Research is currently human-initiated via MCP chat or Claude Code sessions. The `agent:research` type is defined in the communication protocol but not yet routed by the dispatcher. For existing projects, research context is embedded in issue bodies rather than produced by a dedicated research phase.

## Division 3: Design

- **Purpose:** Architecture, specs, acceptance criteria, task decomposition
- **Departments:** Spec Writing, Task Decomposition, Acceptance Criteria
- **Quality Gate:** Is the spec production-ready with clear handoff?
  - PASS --> Production-ready issue specs
  - FAIL --> Spec gaps, rework needed
  - STUCK --> Escalate for human review

**Cross-reference:** This division maps to [project-lifecycle.md](project-lifecycle.md) Phases 4-5 (Requirements and Scaffold).

### Planning Agent

The planning agent is the core of Division 3. It transforms research findings and issue context into production-ready specifications.

**Inputs:**

- Issue body with context section (what problem, why it matters)
- Research findings (if Division 2 was executed)
- Codebase context (relevant file paths, existing patterns)
- Project rules (`.samverk/rules.yaml` -- learned constraints)

**Outputs:**

- Decomposed sub-issues with complete frontmatter (schema v1.1.0)
- Each sub-issue includes: acceptance criteria checklist, file context references, constraints list
- Dependency graph between sub-issues (`depends_on` frontmatter field)
- Estimated token budget per sub-issue

**Model selection:** Sonnet for standard decomposition. Opus for architectural decisions or cross-cutting concerns that span multiple packages.

### Task Decomposition Heuristics

The planning agent splits work when any of these signals are present:

- **Token budget:** Estimated tokens > 50,000 (beyond single-session reliability)
- **File spread:** Changes span 3+ packages or both Go and TypeScript
- **Acceptance criteria:** More than 5 acceptance criteria items (scope too broad for focused execution)
- **Dependency risk:** Changes require coordinated modifications to interfaces consumed by multiple callers

Decomposition targets issues that are small enough for a single agent session with a single model. The ideal sub-issue modifies 1-3 files in one package with 2-3 acceptance criteria.

### Handoff Validation

The dispatcher validates issues before routing to Division 4 (Production). An issue entering `phase:execution` must have:

1. `handoff_ready: true` in frontmatter
2. Non-empty acceptance criteria as a markdown checklist
3. `file_context` listing paths the agent should read before starting
4. `constraints` listing what the agent must NOT do (prevents context conflicts)
5. `estimated_tokens` with a reasonable budget

If any field is missing or empty, the dispatcher escalates instead of routing. This is the quality gate between Division 3 and Division 4.

**Frontmatter example (schema v1.1.0):**

```yaml
schema_version: 1.1.0
type: task
agent_type: code-gen
priority: medium
estimated_tokens: 25000
depends_on: [420]
handoff_ready: true
file_context:
  - internal/dispatcher/router.go
  - internal/dispatcher/router_test.go
constraints:
  - do not modify the forge.IssueTracker interface
  - do not add new dependencies to go.mod
```

### Design Quality Gate

Before a spec enters the production queue, a cross-model review agent validates:

- Acceptance criteria are testable (each item can be verified by a QC agent)
- File context paths exist in the repository
- Constraints don't contradict acceptance criteria
- Token estimate is reasonable for the scope (not 5,000 tokens for a 10-file change)

This review uses a different model than the planning agent (cross-model validation per ADR-030).

## Division 4: Production

- **Purpose:** Focused agents execute to spec -- code, config, docs
- **Departments:** Code Agents, Config Agents, Doc Agents, QC Agents
- **Quality Gate:** Does the work match the spec? PR passes review checklist?
  - PASS --> Approved PR ready to merge
  - FAIL --> Fix agent assigned or rework
  - STUCK --> Escalate for human review

### Master Review Checklist

Seven categories derived from the error taxonomy in [agent-quality.md](agent-quality.md). Categories 1-4 are deterministic (zero token cost, high catch rate) and always run first. Categories 5-7 require LLM review agents.

| # | Category | Check Type | Agent Required | Error Category (agent-quality.md) |
|---|----------|-----------|----------------|-----------------------------------|
| 1 | Compilation | `go build ./...`, `tsc --noEmit` | No | Syntax errors (100% catch) |
| 2 | Tests | `go test -race ./...`, `vitest run` | No | Logic errors (via test coverage) |
| 3 | Lint | `golangci-lint run`, `eslint` | No | Dead code, resource issues (80-90%) |
| 4 | Dependencies | `go mod tidy`, lockfile check | No | API hallucination (70-85%) |
| 5 | Spec conformance | Does the PR satisfy acceptance criteria? | Yes (cross-model) | Requirement conflicts (55-70%) |
| 6 | Security scan | `gosec` + CodeQL + LLM review | Yes | Security vulns (65-87%) |
| 7 | Project context | Patterns, naming, architecture alignment | Yes (cross-model) | Context conflicts (30-50%) |

The PR coordinator marks each category **Required** or **N/A** based on PR scope. For example, a docs-only PR skips categories 1-4 and 6.

### PR Coordinator Flow

The existing PR watcher (`internal/prwatcher/watcher.go`) evolves into the PR coordinator. It already handles tier classification, CI status checks, and review comment remediation. The coordinator adds review dispatch capability.

**Step-by-step flow:**

1. Production agent completes work, opens PR with documentation
   - Issue receives `status:pr-open` label
2. PR coordinator detects new PR (polling via `PullRequestManager.ListPullRequests`)
   - Issue receives `status:pr-reviewing` label
3. Coordinator evaluates master checklist, marks Required/N/A per category based on:
   - Changed file types (`.go` --> compilation/lint/test required; `.md` --> skip)
   - Issue labels (priority:critical --> security scan always required)
   - PR tier classification (Tier 3 --> all categories required)
4. Deterministic checks run first (categories 1-4)
   - These are CI jobs -- the coordinator reads CI status via `forge.CheckRunStatus()`
   - Any failure --> immediate `[FAIL]` for that category, no need for LLM review
5. If deterministic checks pass, LLM review agents are dispatched (categories 5-7)
   - Each scoped review agent runs independently (can be parallel)
   - Each agent receives: the diff, the acceptance criteria, project rules, and its category scope
   - Review agents use a different model than the generator (cross-model per ADR-030)
6. Each review agent posts findings as a PR comment with standardized tags
7. Coordinator reads all results and decides:
   - All `[PASS]` --> merge (respecting tier delay from pr-merge-policy.md)
   - Any `[FAIL]` --> assign fix agent with structured feedback
   - Any `[REVIEW]` --> escalate to human for judgment
8. Fix-review loop repeats (see circuit breaker below)

### Standardized Review Reporting

Review agents post PR comments in machine-parseable format:

```text
REVIEW [category] [verdict]

Details: <human-readable findings>
Fix hint: <suggested direction, not a complete fix>
Agent: <agent-id>
Model: <model-used>
```

Verdicts:

- `[PASS]` -- Check passed, no issues found
- `[FAIL]` -- Actionable issue found, fix required before merge
- `[REVIEW]` -- Ambiguous finding, needs human or senior-model judgment

The coordinator parses these tags to make merge/fix/escalate decisions without reading the full comment body.

### Fix-Review Loop and Circuit Breaker

When a review category returns `[FAIL]`:

1. Coordinator creates structured feedback for the fix agent:
   - Which category failed
   - The specific findings
   - The fix hint from the review agent
   - The acceptance criteria (unchanged -- the spec is the source of truth)
2. Fix agent receives the feedback and pushes a new commit to the PR branch
3. CI re-runs (deterministic checks)
4. If deterministic checks pass, only the previously-failed LLM categories re-run
5. Coordinator re-evaluates

**Circuit breaker:** After 3 fix-review cycles (matching `MaxConsecutiveFailures = 3` in dispatcher config and the 94% cumulative retry success rate from agent-quality.md research), the coordinator escalates:

- Posts all 3 cycles' review findings as a summary comment
- Adds `status:needs-human` label
- Stops the fix-review loop

The human reviews with a fresh session, resolves the issue, and the resolution feeds back into `.samverk/rules.yaml` to prevent the same class of failure in future.

### Integration with Existing PR Watcher

The PR watcher at `internal/prwatcher/watcher.go` already implements:

- PR eligibility checks (not draft, trusted author, no excluded labels, mergeable)
- Tier classification (Tier 1/2/3 from `prwatcher/tier.go`)
- CI pass verification
- Review comment detection with remediation issue creation
- Tier 2 delay enforcement

The production pipeline extends the watcher with:

- Review agent dispatch (new capability)
- Structured review comment parsing (new capability)
- Fix-review loop orchestration (new capability)
- Circuit breaker escalation (new capability)

The existing tier logic is preserved -- the review pipeline runs between "CI passes" and "merge decision."

## Division 5: Delivery

- **Purpose:** Merge, deploy, release notes, integration verification
- **Departments:** Merge, Deploy, Release Notes, Smoke Test
- **Quality Gate:** Is the change live and verified?
  - PASS --> Shipped and confirmed
  - FAIL --> Rollback or hotfix path
  - STUCK --> Escalate for human review

### Deploy Trigger

When a PR merges to main for an issue with `phase:execution`:

1. The issue transitions to `phase:delivery`
2. If the merge is part of a release (release-please creates a release PR), the delivery division waits for the release to be published
3. For non-release merges on the self-hosted instance (CT202), deployment is triggered automatically via `make redeploy`

### Smoke Test Framework

Post-deploy verification runs automatically:

- **Health check:** `GET /healthz` returns 200
- **API smoke:** Critical endpoints respond (e.g., `GET /api/v1/sessions`, `GET /api/v1/metrics`)
- **MCP smoke:** MCP endpoint accepts a valid JSON-RPC request

Smoke tests run as a shell script (`scripts/smoke-test.sh`) invoked after `make redeploy`. The script exits non-zero on any failure.

### Post-Merge Feedback Loop

- **Smoke test passes:** Delivery issue is closed. The original issue receives `status:done`.
- **Smoke test fails:** A remediation issue is created with the failure details. The delivery issue receives `status:needs-human`. If the failure is clearly a regression (new endpoint returns 500), a revert PR is created automatically.

### Release Automation

Release-please handles version bumping and changelog generation (`.github/workflows/release-please.yml`). The pipeline adds a quality gate:

- Release PRs (created by release-please) enter Division 4 like any other PR
- The coordinator skips spec conformance (category 5) for release PRs -- there's no feature spec
- Compilation, tests, lint, and security scan still run
- Tier classification: release PRs are always Tier 2 (delayed merge)

### Rollback Mechanics

If a deployed change causes issues detected by smoke tests or reported by users:

1. Create a revert PR: `git revert <merge-commit> && gh pr create`
2. Revert PR enters Division 4 as Tier 1 (fast-track -- deterministic checks only)
3. On merge, redeploy triggers automatically
4. A post-mortem issue is created linking to the original PR and the revert PR

## Dispatcher Intelligence Upgrade

The dispatcher (`internal/dispatcher/dispatcher.go`) gains two enhancements:

### Handoff Validation

Before routing an issue to Division 4, the dispatcher's `classify()` function validates handoff readiness. Issues with `phase:execution` label but missing `handoff_ready: true` or empty acceptance criteria are escalated instead of routed. This prevents incomplete specs from reaching production agents.

### Model Selection

The dispatcher owns model selection via `selectProviderKey()` in `internal/dispatcher/router.go`. The existing routing chains map to the model tiers recommended in [agent-quality.md](agent-quality.md):

| Routing Key | Model Tier | Use Case |
|-------------|-----------|----------|
| `local` | Ollama/Qwen 32B | Boilerplate, config, docs (estimated_tokens < 10,000) |
| `default` | Claude Sonnet | Standard code generation, tests |
| `complex` | Claude Opus | Architecture, cross-cutting changes, multi-package work |
| `qc` | Cross-model chain | QC agents always use a different model than the generator (ADR-030) |
| `triage` | Haiku | Issue classification and frontmatter validation |

No planning agent is needed for model selection -- the dispatcher's signal-based routing (priority labels, complexity labels, agent type, estimated tokens) is sufficient.

## Agent Workflow Lifecycle

1. Dispatcher validates issue handoff readiness (Division 3 gate)
2. Dispatcher assigns issue to agent via the appropriate provider chain
3. Agent reads file context and constraints from frontmatter
4. Agent attempts to complete work to spec (acceptance criteria)
5. On failure: agent documents the problem, issue returns to dispatcher for retry or escalation
6. On success: agent opens PR with documentation referencing the acceptance criteria
7. PR enters review pipeline (Division 4 quality gate)
8. On merge: issue enters Division 5 (Delivery)

## Human-in-the-Loop: Supervisor Model

### Herb's Role

Herb is **not** an approval gate in the process. He is the **agents' supervisor**. When agents are stuck on a problem, they ask for help. Herb helps them figure it out and the resolution becomes part of the rules for future work.

### Escalation Mechanics

Escalation uses the same pattern across all divisions -- label transition plus structured comment:

1. Agent gets stuck --> documents problem --> adds `status:needs-human` label
2. Posts structured comment: `ESCALATE [component] trigger: <reason> details: <context> action_needed: <what help is needed>`
3. Herb reviews flagged item with a **fresh agent session** (clean context, fresh perspective)
4. Together they perform root cause analysis
5. Resolution is posted as a structured comment: `RESOLVE [human] [timestamp] rule: <new-rule> action: <what was done>`
6. The escalation handler removes `status:needs-human`, applies the appropriate status, and appends the rule to `.samverk/rules.yaml`

This pattern is already implemented in `internal/dispatcher/dispatcher.go` for issue escalations. The PR coordinator follows the same pattern with `ESCALATE [pr-coordinator]` prefix.

### Learned Rules Storage

Rules accumulate from human escalation resolutions and are stored in `.samverk/rules.yaml` per project:

```yaml
rules:
  - id: rule-001
    source_issue: 42
    category: security
    pattern: "never use raw SQL in HTTP handlers"
    added: "2026-03-11"
  - id: rule-002
    source_issue: 55
    category: project-context
    pattern: "all forge operations must use forge.IssueTracker interface"
    added: "2026-03-12"
```

Rules are version-controlled (git), project-scoped (each project has different conventions), and loaded by agents at review time. The `.samverk/` directory convention (`status.md`, `project.yaml`, `autonomy.yaml`) establishes the pattern.

### Fresh Session Pattern

Flagged items get reviewed in a new Claude session, not the stuck agent's context. This provides:

- Clean context without inherited bias
- Fresh perspective on the problem
- Ability to see the problem from outside the agent's framing

### Auto-Learn Philosophy

Early on there will be many needs-human escalations. Each one results in fixes to **both** the project and the process. The goal is to eliminate entire classes of problems, not just individual instances. This is analogous to the existing auto-learn process -- when something gets tagged for human review, the decision we make creates the rules for making that decision automatically in the future.

Escalation frequency decreases over time as the system learns. This is a measurable metric -- the dashboard should track escalations per week as a system health indicator.

## Production Philosophy

### Two-Sided System

- **Research/Design side:** Human + agent collaboration. Produces detailed specs, splits work, defines acceptance criteria. This is where learning happens and where most human interaction occurs.
- **Production side:** Tightly focused agents (potentially different models) executing to spec. Review agents verify against the same spec. **Problems in production mean the design side needs fixing, not the agents.**

### Task Scoping Principle

Work should be divided into small enough parts that it's hard for a single issue to be a problem. The research and design phases should produce detailed handoffs that give agents very clear instructions. Fresh agents are armed with only the tools and skills necessary for a highly focused scope of work.

### Review Agent Principle

Review agents use the **same specs** that production agents used to build. They verify results against the design, not against subjective judgment. This closes the loop -- if the spec is sufficient for an agent to build correctly, it's sufficient for an agent to verify correctly.

## New Types and Interfaces

### Review Pipeline Types

```go
// ReviewResult captures one category's evaluation of a PR.
type ReviewResult struct {
    Category string // "compilation", "tests", "lint", "deps", "spec", "security", "context"
    Verdict  string // "pass", "fail", "review"
    Details  string // human-readable findings
    FixHint  string // suggested direction (not a complete fix)
    Agent    string // which agent performed the review
    Model    string // which model was used
}

// ReviewReport aggregates all review results for one fix-review cycle.
type ReviewReport struct {
    PRNumber    int
    IssueNumber int
    Results     []ReviewResult
    Cycle       int    // fix-review cycle number (1-3)
    Overall     string // "pass", "fail", "escalate"
}
```

### New Labels

| Label | Purpose |
|-------|---------|
| `status:pr-open` | Issue has an open PR |
| `status:pr-reviewing` | PR is in the review pipeline |
| `review:pass` | All review categories passed |
| `review:fail` | One or more review categories failed |

### Extended Frontmatter (Schema v1.1.0)

New fields added to the issue frontmatter schema:

| Field | Type | Purpose |
|-------|------|---------|
| `handoff_ready` | boolean | Division 3 gate -- must be `true` for production routing |
| `file_context` | string list | Paths the agent should read before starting |
| `constraints` | string list | What the agent must NOT do |
| `review_tier` | integer | Override default checklist scope (1-3) |

## Design Decisions

Resolved design questions from v0.1 with rationale.

### D1: Dispatcher Reuse vs Dedicated PR Dispatcher

**Decision:** Extend existing architecture. The dispatcher handles issue lifecycle (unchanged). The PR watcher (`internal/prwatcher/watcher.go`) evolves into the PR coordinator for PR lifecycle.

**Rationale:** The dispatcher already handles 6 event types and explicitly skips PRs. The PR watcher already runs alongside via errgroup, handles tier classification, CI checks, and review comment remediation. These are adjacent but distinct responsibilities -- issue routing vs. PR quality verification. They share forge interfaces (`IssueTracker`, `PullRequestManager`) and run concurrently.

### D2: Master Review Checklist Categories

**Decision:** Seven categories derived from the 8 error categories in [agent-quality.md](agent-quality.md). Categories 1-4 are deterministic (zero token cost). Categories 5-7 require LLM review agents. See the Master Review Checklist table in Division 4 above.

**Rationale:** The agent-quality.md research provides catch rates per error category. Deterministic checks (compile, test, lint, deps) catch 70-100% of categories 1-4 at zero token cost. LLM review adds value for spec conformance, security, and context conflicts where deterministic tools have lower coverage.

### D3: Learned Rules Storage

**Decision:** Per-project `.samverk/rules.yaml`, version-controlled and loaded at review time.

**Rationale:** Rules must be version-controlled (git), project-scoped (each project has different conventions), and machine-readable (YAML). The `.samverk/` directory convention is already established. Resolution comments include `RULE:` blocks parsed by the escalation handler.

### D4: Escalation Mechanics

**Decision:** Label transition (`status:needs-human`) plus structured `ESCALATE [component]` comment, consistent with existing dispatcher pattern. Resolution via `RESOLVE [human]` comment.

**Rationale:** This pattern is already implemented in `internal/dispatcher/dispatcher.go` at lines 269-281. Reusing it for the PR coordinator ensures consistency and avoids a second escalation mechanism.

### D5: Circuit Breaker Threshold

**Decision:** 3 fix-review cycles, matching `MaxConsecutiveFailures = 3` in dispatcher config.

**Rationale:** Agent-quality.md research shows 94% cumulative fix rate after 3 retries with diminishing returns beyond that. The same threshold is already used for issue execution failures. Configurable via `server.yaml` under `max_consecutive_failures`.

### D6: Event-Driven vs Polling

**Decision:** Polling first, webhook-ready architecture.

**Rationale:** Both the dispatcher and PR watcher already poll via the `Watch()` abstraction in `internal/forge/forge.go`. The interface hides the implementation -- a webhook source can be swapped in per-forge later without changing dispatcher or coordinator logic. Gitea webhooks require TLS configuration on the self-hosted instance, which is a separate infrastructure task.

### D7: Multi-Forge Sequencing

**Decision:** GitHub first, Gitea via forge abstraction. Not parity from day one.

**Rationale:** Both forge implementations exist behind `IssueTracker` and `PullRequestManager` interfaces. New capabilities (review dispatch, checklist evaluation) build against the interface. If a forge lacks a capability, `ErrNotSupported` provides graceful degradation. The runtime dispatcher already works on Gitea (CT202). PR review features can roll out on GitHub first and extend to Gitea as the `PullRequestManager` implementation matures.

### D8: Model Selection Strategy

**Decision:** Dispatcher owns model selection via `selectProviderKey()` in `router.go`.

**Rationale:** The dispatcher has all relevant signals (priority labels, complexity labels, agent type, estimated tokens, issue body length). The existing routing chains (`local`, `default`, `complex`, `qc`, `triage`) map directly to the model tiers recommended in agent-quality.md. No planning agent overhead is needed for this decision.

### D9: Production-Ready Handoff Format

**Decision:** Extended frontmatter (schema v1.1.0) with `handoff_ready`, `file_context`, `constraints`, and acceptance criteria. Dispatcher validates before routing.

**Rationale:** Incomplete specs reaching production agents waste tokens and produce low-quality output. A validation step in the dispatcher's `classify()` function catches missing fields before routing. The planning agent (Division 3) is responsible for producing complete handoffs; the dispatcher is responsible for enforcing completeness.

## Related Documents

- [Dispatcher Design](dispatcher-design.md) -- Dispatcher architecture, routing logic, escalation policy
- [PR Merge Policy](pr-merge-policy.md) -- Tier-based merge rules, delay configuration
- [Agent Quality](agent-quality.md) -- Error taxonomy, QC effectiveness research, model routing
- [Project Lifecycle](project-lifecycle.md) -- Seven-phase lifecycle that Divisions 1-5 map to
- [Communication Protocol](communication-protocol.md) -- Issue schema, label taxonomy, agent types
- [Autonomy Model](autonomy-model.md) -- Three-tier trust model (orthogonal to quality gates)
- [Unified Execution Plan](unified-execution-plan.md) -- Current execution tracks (B/W/P streams)
- [Gitea AI Review](gitea-ai-review.md) -- Earlier Gitea AI review research
- [ADR-030](decisions/ADR-030-cross-model-qa.md) -- Cross-model QA validation decision
