# PC Agent Runner Design

## Overview

The PC agent runner turns a Windows workstation into a Samverk worker node.
It continuously polls for queued issues, provisions isolated workspaces, launches
Claude Code (CC) as an autonomous agent, and handles the results — all without
touching the developer's working copy.

**Critical invariant:** No component reads from or writes to the user's working
copy (`D:\DevSpace\Samverk`). All agent file operations occur inside worktree
directories under the workspace root (`D:\bots\`).

## Component Overview

```text
┌──────────────────────────────────────────────────────────────────────────┐
│                          PC Agent Runner Loop                            │
│                                                                          │
│  ┌─────────────┐   ┌───────────────┐   ┌──────────────┐                 │
│  │Forge Poller │──▶│Autonomy Gate  │──▶│Workspace Mgr │                 │
│  │(PowerShell) │   │(tier check)   │   │(worktree)    │                 │
│  └─────────────┘   └───────────────┘   └──────┬───────┘                 │
│                                               │                         │
│                                               ▼                         │
│                                    ┌──────────────────┐                 │
│                                    │ Prompt Formatter  │                 │
│                                    │ (issue → CC task) │                 │
│                                    └────────┬─────────┘                 │
│                                             │                           │
│                                             ▼                           │
│                                    ┌──────────────────┐                 │
│                                    │   CC Launcher    │                 │
│                                    │ (claude --print) │                 │
│                                    └────────┬─────────┘                 │
│                                             │                           │
│                                             ▼                           │
│                                    ┌──────────────────┐                 │
│                                    │ Post-Task Handler│                 │
│                                    │ (PR, labels,     │                 │
│                                    │  cleanup)        │                 │
│                                    └──────────────────┘                 │
└──────────────────────────────────────────────────────────────────────────┘
```

## Full Issue Lifecycle (Sequence Diagram)

```text
ForgePoller       AutonomyGate  WorkspaceMgr  PromptFormatter  CCLauncher  PostTaskHandler
    │                 │              │                │              │             │
    │──poll issues──▶ │              │                │              │             │
    │◀─issue list──── │              │                │              │             │
    │──check tier────▶│              │                │              │             │
    │◀─proceed────────│              │                │              │             │
    │──claim labels──▶(GitHub API)   │                │              │             │
    │──update bare────────────────▶  │                │              │             │
    │──new worktree───────────────▶  │                │              │             │
    │◀─worktree info──────────────── │                │              │             │
    │──format issue───────────────────────────────▶  │              │             │
    │◀─prompt─────────────────────────────────────── │              │             │
    │──launch CC──────────────────────────────────────────────────▶ │             │
    │                                                               │(CC runs:    │
    │                                                               │ read files  │
    │                                                               │ write code  │
    │                                                               │ run tests   │
    │                                                               │ git commit) │
    │◀─exit code + output─────────────────────────────────────────── │             │
    │──handle result──────────────────────────────────────────────────────────▶   │
    │                                                                             │──push branch
    │                                                                             │──open PR
    │                                                                             │──update labels
    │                                                                             │──add comment
    │──remove worktree────────────────────────────────────────────────────────── │
    │                                                                             │──cleanup
    │──next poll──────(loop)
```

## Component Details

### 1. Forge Poller (`scripts/pc-agent/poller.ps1`)

Queries GitHub/Gitea for issues ready to be claimed.

**Query criteria:**

- `status:queued` label — dispatcher has routed it
- Agent label matches: `agent:code-gen`, `agent:triage`, `agent:research`, `agent:docs`
- Not already assigned or in-progress
- Complexity label is within the configured autonomy tier

**Claim protocol:**

1. Remove `status:queued` label
2. Add `status:active` label
3. Add self as assignee (via forge API)
4. Begin processing — if the runner crashes, a watchdog re-queues after a timeout

**Output:** `PSCustomObject` with `Number`, `Title`, `Body`, `Labels`, `AgentType`, `ComplexityTier`

### 2. Autonomy Gate (`scripts/pc-agent/poller.ps1`)

Filters issues against the configured autonomy tier before claiming.

| Complexity Label | Tier | PC Agent Action |
|-----------------|------|----------------|
| `complexity:local` | 1 | Auto-proceed |
| `complexity:cloud` | 2 | Auto-proceed (log prominently) |
| `complexity:ambiguous` | 3 | Skip — leave in queue |
| `priority:critical` + any | — | Auto-proceed regardless of tier |

Configuration in `.samverk/pc-agent.yaml`:

```yaml
autonomy:
  max_tier: 2
  always_proceed_on_critical: true
```

### 3. Workspace Manager (`scripts/pc-agent/workspace.psm1`)

Manages the bare clone and per-session worktrees.

**Directory layout:**

```text
D:\bots\
├── samverk.git\          ← Bare clone (shared object store)
├── worker-1\             ← Worktree for session 1 (branch: fix/42-add-feature)
├── worker-2\             ← Worktree for session 2
└── worker-3\             ← Worktree for session 3 (max 3 simultaneous)
```

**Key functions:**

| Function | Purpose |
|----------|---------|
| `Initialize-AgentWorkspace` | One-time bare clone setup |
| `Update-BareRepo` | `git fetch --all` before each task |
| `Get-AvailableWorkerSlot` | Find next free `worker-N` slot |
| `New-AgentWorktree` | Create worktree on `fix/<N>-<slug>` branch |
| `Remove-AgentWorktree` | Delete worktree and prune refs |
| `Test-WorktreeHealth` | Verify clean git status + project structure |

**Branch naming:** `fix/<issueNumber>-<slug>` where slug is the issue title lowercased,
non-alphanumeric stripped, spaces → hyphens, max 40 chars.

### 4. Prompt Formatter (`scripts/pc-agent/formatter.ps1`)

Converts a Samverk issue into a CC-consumable task prompt.

**Template:**

```text
You are a Samverk agent implementing GitHub issue #<N>.

Title: <issue title>
Agent type: <agent_type>

Issue body:
<full issue body including frontmatter>

Instructions:
- Work in the current directory (a git worktree on branch fix/<N>-<slug>)
- Read CLAUDE.md for project conventions and build commands
- Run `make ci` (build + test + lint) before committing
- Commit your changes with: git commit -m "feat(#<N>): <summary>"
- Do NOT push — the post-task handler pushes and opens the PR
- If you cannot complete the task, write a comment to AGENT_BLOCKED.md
  explaining what is blocked and why

The project uses Go (backend) and React+TypeScript (frontend).
Run `go build ./...` and `go test ./...` to verify backend changes.
Run `cd web && npx tsc --noEmit` to verify frontend changes.
```

**Why CLAUDE.md is not injected:** CC reads `CLAUDE.md` automatically from
the current working directory. The worktree inherits it from the repo. No prompt
injection needed — project context is available natively.

### 5. CC Launcher (`scripts/pc-agent/launcher.ps1`)

Starts CC in the worktree directory with the formatted prompt.

**Invocation:**

```powershell
$result = & claude `
    --print `
    --dangerously-skip-permissions `
    --output-format json `
    --max-turns 50 `
    $prompt 2>&1 | Out-String

$exitCode = $LASTEXITCODE
$parsed   = $result | ConvertFrom-Json -ErrorAction SilentlyContinue
```

**Parameters:**

| Flag | Rationale |
|------|-----------|
| `--print` | Non-interactive headless mode; CC processes task and exits |
| `--dangerously-skip-permissions` | No permission prompts during unattended execution |
| `--output-format json` | Structured output parseable by PowerShell |
| `--max-turns 50` | Safety cap; prevents infinite loops on buggy tasks |

**Timeout:** Set via `Start-Job` + `Wait-Job -Timeout <seconds>`. Default timeout
by complexity label:

| Complexity | Timeout |
|-----------|---------|
| `complexity:local` | 600 s (10 min) |
| `complexity:cloud` | 1800 s (30 min) |
| Default | 900 s (15 min) |

**Success detection:**

- Exit code 0 AND `$parsed.is_error -eq $false`
- If `$parsed.is_error -eq $true`, treat as failure even with exit 0

### 6. Post-Task Handler (`scripts/pc-agent/post-task.ps1`)

Handles the task result: push code, open PR, update issue.

**Success path:**

1. Check `git status` in worktree — are there committed changes on the branch?
2. `git push origin fix/<N>-<slug>` via the bare repo
3. Open PR via forge API (`gh pr create` or GitHub REST API)
4. Post comment on issue: `"PR opened: #<PR number>"`, link to PR
5. Remove `status:active` label, add `status:done` label
6. Remove worktree (`Remove-AgentWorktree`)

**Failure path:**

1. Post comment on issue with error details and CC output excerpt
2. Remove `status:active` label, add `status:needs-human` label
3. Remove worktree (always clean up)

**Blocked path** (CC wrote `AGENT_BLOCKED.md`):

1. Read `AGENT_BLOCKED.md` content
2. Post as issue comment: `"[BLOCKED] <content>"`
3. Remove `status:active` label, add `status:needs-human` label
4. Remove worktree

**Retry policy:** No automatic retries at the runner level. If the issue label
is reset to `status:queued` by a human, it becomes eligible for the next poll cycle.

## Error Handling

| Error | Recovery |
|-------|---------|
| Bare repo unreachable | Log + wait; retry next poll cycle |
| All worker slots occupied | Skip poll; retry in `poll_interval` seconds |
| Worktree creation fails (branch exists) | Delete branch, retry once |
| CC timeout | Kill process, remove worktree, fail the task |
| CC exit code non-zero | Read output, post error comment |
| Push fails (network) | Retry 3 times with 10 s backoff |
| PR create fails | Post branch name in comment for manual PR creation |

## Windows-Specific Considerations

1. **Paths:** Use `Join-Path` throughout. Never hardcode `\` separators. All paths
   are under `D:\bots\` which avoids MSYS path translation issues.

2. **Process management:** Use `Start-Job` / `Wait-Job -Timeout` for CC invocation.
   Direct `&` blocks the runner; background job with timeout enables safe interruption.

3. **Git credential:** The bare repo uses HTTPS. Git credential manager (GCM) must
   have a stored credential for `github.com` and `gitea.herbhall.net`. Test with
   `git -C D:\bots\samverk.git fetch` before first agent run.

4. **CLAUDE.md loading:** CC loads `CLAUDE.md` from the worktree directory and
   all ancestor directories. Since worktrees share the object store but have their
   own working tree, `CLAUDE.md` at the repo root is present and loaded automatically.

5. **PowerShell execution policy:** The runner scripts require
   `Set-ExecutionPolicy RemoteSigned -Scope CurrentUser` (one-time setup).

6. **Concurrent instances:** Each worker slot is a separate process. PowerShell
   does not provide built-in mutex primitives — use file-based locking if running
   multiple runner instances simultaneously.

## Configuration Reference (`.samverk/pc-agent.yaml`)

```yaml
workspace:
  root: D:\bots
  bare_repo: samverk.git
  max_worktrees: 3
  remotes:
    github: https://github.com/HerbHall/samverk.git
    gitea: http://gitea.herbhall.net/samverk/samverk.git

autonomy:
  max_tier: 2
  always_proceed_on_critical: true

timeouts:
  local_seconds: 600
  cloud_seconds: 1800
  default_seconds: 900

poll_interval_seconds: 60

cc:
  max_turns: 50
  output_format: json
```

## Related Documents

- [PC Agent Research Findings](https://github.com/HerbHall/samverk/issues/304#issuecomment-4020751460) — CC headless invocation modes
- [docs/autonomy-model.md](autonomy-model.md) — Three-tier trust model
- [docs/communication-protocol.md](communication-protocol.md) — Label taxonomy
- [scripts/pc-agent/workspace.psm1](../scripts/pc-agent/workspace.psm1) — Workspace module
