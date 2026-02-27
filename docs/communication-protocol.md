# Communication Protocol

## The Core Idea

Inter-agent communication is built on git issues rather than a custom message queue. This gives the system:

- Human-readable audit trail by default
- Universal accessibility (any device with a browser)
- The user check-in interface for free (the issue list IS the digest)
- Webhook-based event system built in
- No vendor lock-in -- abstract behind an interface layer for GitHub, Gitea, GitLab

## Issue as Communication Unit

Every task, question, result, and handoff is an issue:

```markdown
---
type: task | question | result | block
agent_type: orchestrator | dispatcher | code-gen | test | docs | qc | human
priority: critical | high | normal | low
parent_issue: #123
depends_on: #121, #122
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

## Label Taxonomy

### Agent type (who should pick this up)

- `agent:orchestrator`
- `agent:dispatcher`
- `agent:code-gen`
- `agent:test`
- `agent:docs`
- `agent:research`
- `agent:qc`
- `agent:human` -- needs user input

### Status

- `status:queued` -- ready to be picked up
- `status:claimed` -- agent is working on it
- `status:in-progress`
- `status:blocked`
- `status:needs-qc`
- `status:needs-human` -- waiting for user
- `status:done`

### Priority

- `priority:critical`
- `priority:high`
- `priority:normal`
- `priority:low`

### Complexity / routing

- `complexity:local` -- safe to run on local model
- `complexity:cloud` -- requires cloud model
- `complexity:ambiguous` -- dispatcher needs to evaluate

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
