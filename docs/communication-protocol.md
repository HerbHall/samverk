# Communication Protocol

**Schema Version: 1.1.0**

## The Core Idea

Inter-agent communication is built on git issues rather than a custom message queue. This gives the system:

- Human-readable audit trail by default
- Universal accessibility (any device with a browser)
- The user check-in interface for free (the issue list IS the digest)
- Webhook-based event system built in
- No vendor lock-in -- abstract behind an interface layer for GitHub, Gitea, GitLab

## Issue Schema

Every task, question, result, and handoff is an issue. The issue body uses YAML frontmatter followed by markdown sections.

### Frontmatter Fields

| Field | Type | Required | Valid Values | Description |
| ----- | ---- | -------- | ------------ | ----------- |
| `schema_version` | string | Yes | `1.1.0` | Schema version for forward compatibility |
| `type` | enum | Yes | `task`, `question`, `result`, `block`, `idea`, `research`, `feasibility`, `gate`, `requirement`, `architecture`, `scaffold` | Communication type |
| `agent_type` | enum | Yes | See [Agent Type Labels](#agent-type-who-should-pick-this-up). Includes `ideation`, `feasibility`, `legal` for pre-project phases. | Who should work on this |
| `priority` | enum | Yes | `critical`, `high`, `normal`, `low` | Scheduling priority |
| `parent_issue` | int | No | Issue number | Parent issue in decomposition tree |
| `depends_on` | int[] | No | Issue numbers | Issues that must close before this starts |
| `estimated_tokens` | int | No | Positive integer | Token budget estimate |
| `actual_tokens` | int | No | Positive integer | Actual tokens consumed (set on completion) |
| `model_used` | string | No | Model identifier | Which model completed the work |
| `file_context` | string[] | No | File paths | Explicit file paths the agent should read before starting. Highest priority context source. |
| `constraints` | string[] | No | Free-form strings | Operational constraints the agent must respect (e.g., "do not modify X", "run make ci before finishing"). |
| `handoff_ready` | boolean | No | `true`, `false` | Flag indicating the issue has been reviewed and is ready for agent work. Defaults to `false` when omitted. |

### Body Sections

| Section | Required | Purpose |
| ------- | -------- | ------- |
| Summary | Yes | One sentence human-readable description |
| Context | Yes | What the agent needs to know -- parent issue, files, decisions |
| Acceptance Criteria | Yes (for `task`) | Specific, testable conditions as a checklist |
| Result | No (populated on completion) | Agent's output, findings, or deliverables |
| Notes | No | Decisions made, alternatives considered, problems hit |

### Schema Template

```markdown
---
schema_version: "1.1.0"
type: task
agent_type: code-gen
priority: normal
parent_issue: 123
depends_on: [121, 122]
estimated_tokens: 2000
actual_tokens:
model_used:
file_context:
  - "internal/forge/tracker.go"
  - "pkg/models/issue.go"
constraints:
  - "run make ci before finishing"
handoff_ready: true
---

## Summary

One sentence human-readable description.

## Context

What the agent needs to know to do this work.
Reference to parent issue, relevant files, prior decisions.

## Acceptance Criteria

- [ ] Specific, testable condition 1
- [ ] Specific, testable condition 2

## Result

(Populated by agent when complete)

## Notes

Any decisions made, alternatives considered, problems hit.
```

### Schema Versioning

The schema uses semantic versioning (`MAJOR.MINOR.PATCH`):

- **PATCH**: New optional fields, documentation clarifications
- **MINOR**: New required fields (with migration path), new enum values
- **MAJOR**: Breaking structural changes, removed fields

Agents must check `schema_version` and handle unknown fields gracefully. An agent receiving a schema version higher than it supports should log a warning and process the fields it recognizes.

### Context Discovery and `file_context`

Agents receive file context from three sources, applied in priority order:

1. **Foundational files (automatic):** The explorer auto-includes files that prevent common failures. These are included when they exist in the repo root, regardless of whether the issue author lists them:
   - `CLAUDE.md` -- project conventions and build commands
   - `go.mod` -- correct module path and import paths (Go projects)
   - `Makefile` -- build targets and commands
   - `web/package.json` -- dependencies and scripts (frontend projects)
   - `web/tsconfig.json` -- TypeScript configuration (frontend projects)

2. **`file_context` (explicit):** File paths listed in the frontmatter. The agent reads these first after foundational files. This is the highest priority author-specified context because the issue author knows which files are relevant.

3. **Explorer regex-based discovery (implicit):** The agent's explorer scans the issue body (Summary, Context, Acceptance Criteria) for file path patterns (e.g., `internal/forge/tracker.go`, `pkg/models/issue.go`). Discovered paths supplement `file_context` but do not override it.

File **contents** (not just paths) are injected into the agent's prompt. When a file path is discovered but the content cannot be read, the file is listed in a warning section so the agent knows context is incomplete.

When both explicit and discovered sources are present, `file_context` paths are read first. Explorer-discovered paths are read after, skipping any already covered by `file_context` (unless the existing entry has empty content, in which case the discovered content takes precedence).

When `file_context` is omitted, the agent relies on foundational files and explorer discovery.

### File Context Budgets

- **Explore phase:** 64KB total, 8KB per individual file
- **Prompt injection:** 32KB total for the `## Relevant Files` section

When the budget is exceeded, files are truncated or omitted. A warning is appended to the prompt listing which files were affected.

### Provider-Specific File Access

- **CLI providers** (e.g., claude-cli): Run in an isolated git worktree with full filesystem access via tools (Read, Glob, Grep). Prompt-injected files serve as a starting point; the agent can discover additional files at runtime.
- **API providers** (e.g., Ollama): Have no filesystem access. Prompt-injected files are the agent's only source context. The prompt instructs the agent to base all implementations strictly on the provided files.

The `constraints` field provides operational guardrails that the agent must respect throughout execution. Unlike acceptance criteria (which define what success looks like), constraints define boundaries on how the agent works (e.g., "do not modify the public API", "skip integration tests on Windows").

The `handoff_ready` flag signals that a human or orchestrator has reviewed the issue and confirmed it is ready for agent pickup. The dispatcher skips issues where `handoff_ready` is `false` or omitted, preventing agents from starting work on draft or incomplete issues.

## Example Issues

### Task Issue (v1.1.0)

```markdown
---
schema_version: "1.1.0"
type: task
agent_type: code-gen
priority: high
parent_issue: 45
depends_on: [42, 43]
estimated_tokens: 3000
file_context:
  - "internal/forge/tracker.go"
  - "internal/forge/github.go"
constraints:
  - "do not modify the IssueTracker interface"
  - "run make ci before finishing"
handoff_ready: true
---

## Summary

Implement the IssueTracker interface for Gitea.

## Context

The IssueTracker interface is defined in internal/forge/tracker.go. The GitHub
implementation (internal/forge/github.go) is complete and can serve as a reference.
Gitea SDK: code.gitea.io/sdk/gitea.

## Acceptance Criteria

- [ ] GiteaTracker struct implements all IssueTracker methods
- [ ] Unit tests with mock Gitea server pass
- [ ] Optimistic locking pattern implemented for task claiming
- [ ] Error handling follows project conventions (wrapped errors with context)

## Result

## Notes
```

### Question Issue

```markdown
---
schema_version: "1.1.0"
type: question
agent_type: human
priority: normal
parent_issue: 45
---

## Summary

Should Gitea webhook validation use HMAC-SHA256 or HMAC-SHA1?

## Context

Gitea supports both HMAC-SHA256 and HMAC-SHA1 for webhook signature validation.
SHA256 is more secure but SHA1 is the default. The GitHub implementation uses SHA256.

## Acceptance Criteria

- [ ] User provides a decision on which algorithm to use

## Result

## Notes
```

### Result Issue

```markdown
---
schema_version: "1.1.0"
type: result
agent_type: qc
priority: normal
parent_issue: 50
actual_tokens: 1200
model_used: claude-sonnet-4-20250514
---

## Summary

QC validation passed for Gitea IssueTracker implementation.

## Context

Reviewed PR #52 implementing GiteaTracker against acceptance criteria from issue #50.

## Result

All acceptance criteria met:
- GiteaTracker implements all 7 IssueTracker methods
- 14 unit tests pass with mock server
- Optimistic locking uses comment-based claiming with 10s window
- All errors wrapped with fmt.Errorf and %w verb

## Notes

Minor style suggestion: consider extracting label conversion to a helper function.
Not blocking -- filed as issue #55.
```

### Block Issue

```markdown
---
schema_version: "1.1.0"
type: block
agent_type: dispatcher
priority: high
parent_issue: 50
depends_on: [48]
---

## Summary

Blocked: Gitea webhook endpoint requires TLS certificate configuration.

## Context

Issue #50 (Gitea integration) requires a webhook endpoint. The self-hosted Gitea
instance requires TLS for webhook delivery. No TLS certificate is configured for
the Samverk server yet (issue #48).

## Acceptance Criteria

- [ ] Issue #48 (TLS configuration) is resolved
- [ ] Webhook endpoint is accessible from Gitea instance

## Result

## Notes

Independent work on Gitea API client (non-webhook methods) can continue.
Only the Watch() method is blocked.
```

## Label Taxonomy

### Two-Tier Label Architecture

Labels are split into two tiers to support both Samverk-managed and plain DevKit projects:

1. **Base labels** — Defined in DevKit's `project-templates/github-labels.json`. Applied to all projects regardless of Samverk status. Covers type (`feat`, `fix`, `chore`, etc.), priority (`priority:critical` through `priority:low`), milestones, and general workflow.

2. **Overlay labels** — Defined in Samverk's `overlay/labels.json`. Applied only to Samverk-managed projects (projects with a `.samverk/` directory). Covers all labels listed below: agent types, status workflow, priority (shared naming with base), complexity routing, and lifecycle phases.

The base and overlay sets are disjoint — no label appears in both files. A project scaffolded without the Samverk overlay uses only base labels. Applying the Samverk overlay adds the labels below additively.

### Agent Type (who should pick this up)

| Label | Color | Description |
| ----- | ----- | ----------- |
| `agent:orchestrator` | `#1d76db` | High-level task decomposition and planning |
| `agent:dispatcher` | `#0e8a16` | Issue routing and dependency management |
| `agent:code-gen` | `#5319e7` | Code generation and implementation |
| `agent:test` | `#fbca04` | Test writing and execution |
| `agent:docs` | `#c5def5` | Documentation generation |
| `agent:research` | `#d4c5f9` | Research and analysis tasks |
| `agent:qc` | `#e99695` | Static data validation — no live infrastructure |
| `agent:infra` | `#006b75` | Live infrastructure: SSH to servers, Gitea/GitHub API, deployed services |
| `agent:pc` | `#e36209` | Runs on dev PC via PowerShell CC agent scripts |
| `agent:ideation` | `#ff9f1c` | Idea intake, synthesis, intent alignment |
| `agent:feasibility` | `#2ec4b6` | Technical assessment, effort estimation, risk analysis |
| `agent:legal` | `#e71d36` | Trademark, licensing, regulatory concerns (external contractor) |
| `agent:human` | `#b60205` | Requires user judgment, GUI interaction, or time-based observation |

### Status (lifecycle state)

| Label | Color | Description |
| ----- | ----- | ----------- |
| `status:queued` | `#c2e0c6` | Ready to be picked up by an agent |
| `status:claimed` | `#bfd4f2` | Agent has claimed, work starting |
| `status:in-progress` | `#0075ca` | Active work underway |
| `status:blocked` | `#d93f0b` | Waiting on dependency (see `depends_on`) |
| `status:needs-qc` | `#fbca04` | Work complete, awaiting QC validation |
| `status:needs-human` | `#d73a4a` | Escalated from automation — requires user decision |
| `status:human-pending` | `#8b5cf6` | Awaiting human execution — not dispatcher-eligible |
| `status:done` | `#0e8a16` | Complete and validated |

### Priority

| Label | Color | Description |
| ----- | ----- | ----------- |
| `priority:critical` | `#b60205` | Blocks multiple work streams, address immediately |
| `priority:high` | `#d93f0b` | Important, schedule next |
| `priority:normal` | `#0075ca` | Standard priority |
| `priority:low` | `#c2e0c6` | Nice to have, schedule when convenient |

### Complexity (routing hint)

| Label | Color | Description |
| ----- | ----- | ----------- |
| `complexity:local` | `#c2e0c6` | Safe to run on local model (narrow, well-defined) |
| `complexity:cloud` | `#d4c5f9` | Requires cloud model (complex reasoning, ambiguity) |
| `complexity:ambiguous` | `#fbca04` | Dispatcher needs to evaluate before routing |

#### Auto-Classification

When an issue does not have a complexity label, the dispatcher auto-classifies based on frontmatter signals:

| Classification | Condition | Routing chain |
| -------------- | --------- | ------------- |
| `complexity:local` | estimated_tokens < 10k AND file_context <= 3 files | local (Ollama fleet -> Sonnet) |
| `complexity:cloud` | estimated_tokens > 30k OR file_context >= 4 files | complex (Opus -> Sonnet) |
| `complexity:ambiguous` | Everything else (default) | Determined by agent type |

Thresholds are defined as package-level variables in `router.go` for tuning without recompile.

#### Routing Chain Priority

The dispatcher selects a routing chain using these rules in priority order:

1. **Agent-type overrides** -- docs agents use default chain (Ollama has 100% failure rate for prose); research agents use tier-based routing (quick/standard/deep)
2. **Complex signals** -- critical priority, complexity:cloud, or architectural title keywords route to `complex` chain
3. **Local signals** -- boilerplate/scaffold labels, complexity:local, or chore: prefix route to `local` chain
4. **QC agent** -- dedicated `qc` chain for cross-model validation
5. **Code-gen/test/infra agents** -- `code-gen` chain (CLI-only providers, no Ollama)
6. **Triage signals** -- low priority or short prose body route to `triage` chain
7. **Default** -- everything else

Routing decisions are logged with the chain key, reason, and complexity label for observability.

### Phase (lifecycle position)

| Label | Color | Description |
| ----- | ----- | ----------- |
| `phase:intake` | `#D4C5F9` | Idea capture and initial structuring |
| `phase:research` | `#C5DEF5` | Research and feasibility investigation |
| `phase:gate` | `#B60205` | Awaiting go/no-go or approval decision |
| `phase:requirements` | `#0075CA` | Requirements and architecture definition |
| `phase:scaffold` | `#C2E0C6` | Project setup and infrastructure |
| `phase:execution` | `#0E8A16` | Active development |
| `phase:delivery` | `#FBCA04` | Publishing and deployment |
| `phase:parked` | `#EDEDED` | Deferred for later |
| `phase:killed` | `#000000` | Abandoned with documented rationale |

### Routing Rules

The dispatcher uses labels to make assignment decisions:

1. **Filter by `status:queued`** -- only pick up unassigned work
2. **Check `depends_on`** -- if any dependency is not `status:done`, set `status:blocked`
3. **Read `agent_type`** -- route to the matching agent pool
4. **Read `complexity`** -- determine local vs. cloud execution
5. **Sort by `priority`** -- critical before high before normal before low
6. **Apply optimistic locking** -- claim via comment before starting work

## Dispatcher Agent Behavior

The dispatcher is the always-running agent that watches the issue tracker:

1. New issue created with `status:queued` -- dispatcher wakes (webhook or poll)
2. Dispatcher evaluates: complexity, agent type needed, dependencies met?
3. If dependencies not met -- add `status:blocked`, comment with blocking issue numbers
4. If ready -- assign to appropriate agent pool, change label to `status:claimed`
5. Monitor for completion or timeout
6. On timeout -- reassign or escalate

## Optimistic Locking for Task Claiming

GitHub/Gitea don't have atomic claim operations. Pattern to prevent two agents grabbing the same issue:

1. Agent comments "Claiming this issue -- [agent-id] [timestamp]"
2. Agent waits 10 seconds
3. Agent checks: is my claim the first comment in that window?
4. If yes -- proceed, add `status:claimed` label
5. If no -- back off, find next available issue

## QC Flow

```text
Agent completes work
    --> Comments result on issue
    --> Removes status:in-progress
    --> Adds status:needs-qc

QC Agent picks up
    --> Validates against acceptance criteria
    --> Pass: closes issue, parent issue notified
    --> Fail: reopens, comments specific failures, adds status:queued for retry
    --> Repeated failure (3x): adds status:needs-human, notifies user
```

### Retry Context Requirements

When an issue is re-queued after failure, the retry prompt must include additional context beyond the original issue body:

1. **Prior failure messages (automatic):** The 3 most recent failure messages from the store, untruncated. Full error messages are diagnostic data needed for self-correction -- never truncate them.

2. **Human comments (automatic):** All comments added by humans after issue creation. These contain clarifications, design decisions, and scope corrections that the agent must follow. Agent-generated comments (dispatcher, QC, correction engine) are filtered out.

3. **All validation errors (on in-session retry):** When the runner retries after a validation failure within the same session, all validation errors are included -- not just the first.

The correction engine decides whether to retry, escalate, switch providers, or pause. The runner's job is to ensure the retry prompt has maximum context so the agent can self-correct.

## User Check-in Flow

When user opens chat and asks "how's my project doing?":

Front-end agent queries issue tracker and surfaces:

1. `status:needs-human` issues (prioritized -- these are blocking work)
2. Recently closed issues (progress since last check-in)
3. Currently `status:in-progress` issues (what's happening now)
4. `status:queued` count (backlog depth)
5. Cost summary (tokens used since last check-in)

User addresses blocked issues in conversation. Front-end agent translates responses into issue comments/updates. Work resumes.

## Platform Abstraction

Build a thin interface layer so the same codebase supports multiple git forges:

```go
type IssueTracker interface {
    CreateIssue(issue Issue) (int, error)
    UpdateIssue(id int, update IssueUpdate) error
    AddComment(id int, comment string) error
    ListIssues(filter IssueFilter) ([]Issue, error)
    SetLabels(id int, labels []string) error
    Assign(id int, agent string) error
    Watch(handler func(Event)) error
}
```

Implementations: GitHubTracker, GiteaTracker, GitLabTracker.

Gitea on self-hosted server = no API rate limits, full control. GitHub = easiest to start with, most familiar.

## Related Documents

- [Intent Verification Protocol](intent-verification.md) — pre-execution understanding verification (uses issue comments for tier exchanges and concern flags)
- [Autonomy Model](autonomy-model.md) — permission tiers governing what actions agents may take
- [ADR-012: Git Issues as Communication Protocol](decisions/ADR-012-git-issues-protocol.md)
- [ADR-021: Intent Verification Protocol](decisions/ADR-021-intent-verification.md)
