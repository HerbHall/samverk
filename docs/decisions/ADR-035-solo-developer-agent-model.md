# ADR-035: Solo Developer Agent Model

**Status:** Proposed
**Date:** 2026-03-15
**Supersedes:** None
**Related:** ADR-015 (three-tier autonomy), ADR-033 (multi-machine runtime)

## Context

Samverk agents currently operate in a prompt-response model: the runner sends
an issue body plus a system prompt to a provider, receives text back, and posts
it as a GitHub comment or attempts PR creation via EDIT block parsing. This
model has three structural problems:

1. **Blind coding** -- API-based providers (Claude API, OpenAI, Ollama) receive
   zero codebase context. Agents write code from memory of their training data,
   not from reading the actual source tree. The file context injection point
   exists (runner.go:394 TODO) but was never implemented.

2. **No workspace isolation** -- All agent work happens in the runner process
   context. There is no concept of "agent checks out a branch, makes changes,
   submits a PR." Code changes are extracted from text output via regex
   parsing of EDIT blocks.

3. **Blind retries** -- When agents fail, the system re-queues with identical
   configuration. No root cause analysis, no provider switching, no learning.
   The failure classification system (13 classes) captures *what* failed but
   never informs *what to do differently*.

Meanwhile, Claude Code in headless mode (`claude -p --allowedTools`) operates
exactly like a solo developer: it reads files, makes changes, runs tests, and
can create branches and PRs. This capability exists on CT 202 today (pending
issue #501 for the allowedTools fix).

## Decision

Adopt the **solo developer agent model**: every agent operates as an
independent developer assigned to an issue. The agent receives a workspace
(git worktree or checkout), reads the necessary files, makes changes, and
submits results through the standard git workflow (branch, commit, PR).

### Single Unified Workflow, Tiered Permissions

All 10 agent types use the same execution pipeline with permissions tiered
by the work they perform:

| Tier | Agent Types | Permissions | Output |
|------|-------------|-------------|--------|
| **Read-only** | orchestrator, dispatcher, research, qc | Read issues, read source, post comments, add labels | GitHub comments |
| **Code-write** | code-gen, test | Full source access, create branches, create PRs | Pull requests |
| **File-write** | docs, infra, pc | Selective file access, post comments with file changes | Comments or PRs |
| **Manual** | human | None (human decision required) | Manual intervention |

### Execution Model

For **Claude CLI agents** (CT 202, PC workers):

```text
1. Dispatcher assigns issue to agent
2. Runner creates isolated git worktree: git worktree add /tmp/agent-{id} -b agent/{issue}
3. Runner invokes: claude -p --allowedTools "Bash,Read,Edit,Write,Glob,Grep" \
     --max-turns 50 --no-session-persistence \
     -p "You are assigned issue #{N}. Read the issue, understand the codebase, implement the fix, run tests."
4. Claude Code operates as solo developer in the worktree
5. On completion: runner validates output (build, test, lint)
6. If validation passes: push branch, create PR
7. If validation fails: record failure with context, route to correction engine
8. Cleanup worktree
```

For **API-based agents** (Claude API, OpenAI, Ollama):

```text
1. Dispatcher assigns issue to agent
2. Runner gathers context: relevant source files (up to 32KB), issue body, related issues
3. Runner invokes provider Chat() with enriched prompt including file contents
4. Provider returns response with EDIT blocks (code changes) or analysis text
5. Runner applies EDIT blocks to a temporary worktree
6. Same validation, PR creation, and cleanup as CLI path
```

### Failure Response: Analyze-Correct-Retest

Replace blind retries with intelligent failure response:

```text
Task fails
  ├─ Classify failure (existing 13-class system)
  ├─ Analyze: Is this recoverable? What should change?
  │   ├─ Provider failure → switch provider, retry
  │   ├─ Auth failure → escalate immediately (don't retry)
  │   ├─ Timeout → increase timeout 1.5x, retry with same or simpler provider
  │   ├─ Code failure → capture error context, retry with error in prompt
  │   └─ Permanent → escalate to needs-human with full context
  ├─ Apply correction (temporary or permanent)
  ├─ Retry with correction applied
  └─ Track: did the correction work? Feed back to routing.
```

Corrections are typed:

- **Temporary**: Provider switch, timeout increase. Auto-revert after success
  or TTL expiry.
- **Permanent**: Classification fix, prompt improvement, routing rule update.
  Persist and apply to future matching tasks.

### Pre-Posting Validation Gate

Before any agent output reaches a GitHub issue or PR:

1. **Syntax check**: Do EDIT blocks parse correctly? Do file paths exist?
2. **Build check**: Does `go build ./...` pass after applying changes?
3. **Test check**: Does `go test ./...` pass?
4. **Lint check**: Does `golangci-lint run` pass on changed files?

Failed validation routes back to the agent with the error output, not to
the issue as a comment. This prevents token waste on QC cycles and reduces
noise on issues.

## Consequences

### Positive

- Agents produce higher quality output because they read actual source code
- Isolated worktrees prevent agents from interfering with each other
- Git-native workflow produces auditable history (branches, commits, PRs)
- Intelligent failure response reduces token burn on blind retries
- Pre-posting validation catches obvious errors before they waste QC cycles
- Single workflow simplifies the codebase (no per-type execution paths)
- Aligns with how Claude Code already works in developer workflows

### Negative

- Worktree creation adds latency (~2-5 seconds per task)
- Disk usage increases (each worktree is a shallow copy)
- Claude CLI agents require more tokens (file reading) than prompt-response
- API providers still need prompt-injected context (can't use tools natively)
- Validation gate adds time before output posts (~30-60 seconds for build/test)

### Neutral

- Provider interface (`Chat()`) does not change -- enriched prompts are
  compatible with existing API
- Existing EDIT block parsing continues to work for API providers
- Dashboard and MCP tools are unaffected
- Tier enforcement (ADR-015) slots directly into the permission model

## Implementation

### Phase 1: Foundation (Issues #501, #503-506, NEW)

- Fix allowedTools for Claude CLI (#501)
- Add provider logging (#503-506)
- Implement tier enforcement in runner
- Add intelligent failure response engine

### Phase 2: Solo Developer Pipeline (NEW)

- Implement worktree creation/cleanup in runner
- Implement file context gathering for API providers
- Implement pre-posting validation gate
- Wire Claude CLI invocation with allowedTools in worktree

### Phase 3: Validation (P07, P08)

- First autonomous issue completion (P07)
- Agent loop processes multiple issues (P08)
- Measure: success rate, token usage, time-to-completion

### Phase 4: Scale (Phase 6)

- Multi-machine agent distribution (ADR-033)
- Gitea migration (B-stream)
- Adaptive scaling (W-stream)
