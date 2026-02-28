# Communication Protocol

**Schema Version: 1.0.0**

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
| `schema_version` | string | Yes | `1.0.0` | Schema version for forward compatibility |
| `type` | enum | Yes | `task`, `question`, `result`, `block` | Communication type |
| `agent_type` | enum | Yes | See [Agent Type Labels](#agent-type-who-should-pick-this-up) | Who should work on this |
| `priority` | enum | Yes | `critical`, `high`, `normal`, `low` | Scheduling priority |
| `parent_issue` | int | No | Issue number | Parent issue in decomposition tree |
| `depends_on` | int[] | No | Issue numbers | Issues that must close before this starts |
| `estimated_tokens` | int | No | Positive integer | Token budget estimate |
| `actual_tokens` | int | No | Positive integer | Actual tokens consumed (set on completion) |
| `model_used` | string | No | Model identifier | Which model completed the work |

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
schema_version: "1.0.0"
type: task
agent_type: code-gen
priority: normal
parent_issue: 123
depends_on: [121, 122]
estimated_tokens: 2000
actual_tokens:
model_used:
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

## Example Issues

### Task Issue

```markdown
---
schema_version: "1.0.0"
type: task
agent_type: code-gen
priority: high
parent_issue: 45
depends_on: [42, 43]
estimated_tokens: 3000
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
schema_version: "1.0.0"
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
schema_version: "1.0.0"
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
schema_version: "1.0.0"
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

### Agent Type (who should pick this up)

| Label | Color | Description |
| ----- | ----- | ----------- |
| `agent:orchestrator` | `#1d76db` | High-level task decomposition and planning |
| `agent:dispatcher` | `#0e8a16` | Issue routing and dependency management |
| `agent:code-gen` | `#5319e7` | Code generation and implementation |
| `agent:test` | `#fbca04` | Test writing and execution |
| `agent:docs` | `#c5def5` | Documentation generation |
| `agent:research` | `#d4c5f9` | Research and analysis tasks |
| `agent:qc` | `#e99695` | Quality control validation |
| `agent:human` | `#b60205` | Requires user input or decision |

### Status (lifecycle state)

| Label | Color | Description |
| ----- | ----- | ----------- |
| `status:queued` | `#c2e0c6` | Ready to be picked up by an agent |
| `status:claimed` | `#bfd4f2` | Agent has claimed, work starting |
| `status:in-progress` | `#0075ca` | Active work underway |
| `status:blocked` | `#d93f0b` | Waiting on dependency (see `depends_on`) |
| `status:needs-qc` | `#fbca04` | Work complete, awaiting QC validation |
| `status:needs-human` | `#b60205` | Escalated to user for decision or approval |
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
