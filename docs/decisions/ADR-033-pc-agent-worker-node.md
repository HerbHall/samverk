# ADR-033: PC Agent Worker Node with Isolated Workspaces

## Status

Accepted

## Context

Samverk's agent architecture (ADR-007, ADR-023) targets a hybrid model: cloud API agents handle
stateless single-turn tasks while a local worker node can run a full Claude Code session with
persistent file state. The PC agent is that local worker -- it runs on the developer's Windows
machine, processing GitHub/Gitea issues autonomously while the developer is away.

### The user-repo invariant

The developer's working copy of the repository is sacred. It holds uncommitted work, experimental
branches, and active features. Any agent that modifies the working copy risks:

- Destroying uncommitted changes
- Leaking agent-generated code into unrelated branches
- Causing merge conflicts that break the developer's current session
- Triggering pre-commit hooks with unexpected side effects

The PC agent must process repository tasks without ever reading from or writing to the
developer's working copy.

### Why local execution matters

Claude Code (`claude --print`) runs as a full agentic loop with tool access: it reads files,
executes shell commands, runs tests, and makes multi-step decisions. This is qualitatively
different from a single-turn cloud API call. Tasks that require iterative debugging, reading build
output, and adapting based on test failures need this capability. The CT 202 agent in the
Proxmox cluster can also run CC sessions, but:

- The developer's machine has the same git credentials, SSH keys, and tool access as their
  normal workflow
- No provisioning delay -- the bare clone is pre-seeded
- Windows-native toolchain (Go, PowerShell, VS Code) matches the project's development environment
- Lower latency for small tasks (no container start time)

### Why PowerShell first

The PC agent is implemented in PowerShell rather than compiled Go for several reasons:

- **No build step**: drop the scripts into `scripts/pc-agent/` and run. No compilation
  dependency on the project being in a buildable state.
- **Rapid iteration**: fix a bug in `forge.psm1`, restart the loop -- no recompile.
- **Native Windows integration**: PowerShell has first-class access to Windows APIs, the
  registry, scheduled tasks, and credential managers.
- **Low barrier**: the scripts are readable and modifiable by anyone familiar with PowerShell,
  without requiring Go knowledge.

The long-term plan (issue #323, tracked in this ADR's "Future" section) is a compiled
`samverk worker` Go binary that replaces the PowerShell scripts with a proper service.

### PC agent vs CT 202 agent

| Capability | PC Agent | CT 202 Agent |
|-----------|---------|-------------|
| Runtime | PowerShell loop + `claude --print` | Containerised Go runner |
| Working directory | Isolated git worktree | Ephemeral container FS |
| Concurrency | Up to 3 worker slots (serial by default) | Pool-managed goroutines |
| Toolchain | Developer's full toolchain (Go, pnpm, etc.) | What's in the container image |
| Forge access | gh CLI or Gitea REST API | Samverk store + forge.go |
| Autonomy gate | `autonomy.psm1` (ADR-015 tiers) | Dispatcher (ADR-014) |
| Persistence | Labels on issues, git branches | SQLite sessions + labels |

The two agents are complementary. Samverk routes Tier 1-2 issues to whichever resource is
available. The PC agent handles the subset that benefits from the developer's full local
toolchain.

### Options considered for workspace isolation

**Option A: Fresh clone per task**

Clone the repo into a temporary directory for each task. Simple and completely isolated.

Pros: No shared state between tasks. Easy to reason about.
Cons: Full clone (5-30 minutes for large repos) per task. Network bandwidth cost. Not viable.

**Option B: Shallow clone per task**

`git clone --depth 1` for each task.

Pros: Faster than full clone. Still isolated.
Cons: Still minutes per task for non-trivial repos. Shallow clone limits git history access
(CC may need `git log` for context). Partial solution to the time problem.

**Option C: Bare clone + worktrees (chosen)**

A single bare clone acts as the shared object store. Each task gets its own worktree
(`git worktree add`) pointing at a new branch. The worktree is a full working directory
with a separate HEAD, index, and working tree -- but shares the object store with the
bare clone.

Pros:

- Worktree creation is near-instant (no object transfer).
- Each task has its own isolated branch and working tree.
- The bare clone is never a working copy -- no risk of tool operations touching it.
- Git operations inside the worktree (commits, branches, resets) are fully isolated.
- Multiple worktrees can exist simultaneously for future concurrency.
- Proven git feature, production-stable since git 2.5 (2015).

Cons:

- Requires periodic `git fetch` to keep the bare clone current.
- Two levels of indirection (bare clone + worktree) to explain.

Option C was chosen. The worktree model is the standard approach for CI systems that need
parallel isolated builds from a shared cache.

## Decision

The PC agent uses a **bare clone + worktree** isolation model:

1. **Bare clone** at a configurable root (default `D:\bots\samverk.git`). Created once via
   `Initialize-AgentWorkspace`. Never contains a working tree. Updated before each task
   via `git fetch --all`.

2. **Worker slots** are numbered directories (`D:\bots\worker-1` through `D:\bots\worker-N`)
   corresponding to git worktrees. The default maximum is 3 slots.

3. **Per-task worktree** created by `New-AgentWorktree`:
   - Branch name: `fix/<issueNumber>-<slug>` (slug is sanitised from the issue title)
   - Branch is created from `origin/main` at task start
   - Worktree path: `D:\bots\worker-<slot>\`

4. **Claude Code invocation** runs inside the worktree with `--print
   --dangerously-skip-permissions --output-format json --max-turns 50`. CC is told to:
   - Read `CLAUDE.md` before starting
   - Run `make ci` before committing
   - Push its branch and exit (never create the PR itself)

5. **Post-task cleanup** by `Invoke-PostTask`:
   - Detects git state (pushed, unpushed commits, staged changes)
   - Auto-commits staged changes if CC left them uncommitted
   - Pushes to remote (with single retry)
   - Opens a PR via forge API
   - Transitions issue labels: `status:active` → `status:needs-qc`
   - Removes the worktree: `git worktree remove --force`
   - Deletes the local branch from the bare repo

6. **Autonomy gate** (`autonomy.psm1`) evaluates each issue before claiming:
   - Tier 1 (local complexity): auto-execute
   - Tier 2 (cloud complexity, normal/high priority): auto-execute, log prominently
   - Tier 3 (ambiguous, critical): skip and post explanatory comment
   - Hard-skip labels (`agent:human`, `status:needs-human`): always skip

### Critical invariant

No function in the PC agent stack reads from or writes to the developer's working copy.
All file operations occur inside worktree directories under the workspace root.
This invariant is enforced by architecture: the bare repo has no working tree, and worktrees
are created at a separate path configured by `WorkspaceRoot` (not inside the developer's repo).

### Module structure

```text
scripts/pc-agent/
    agent-loop.ps1      Main orchestration loop (poll -> gate -> claim -> execute -> cleanup)
    workspace.psm1      Bare clone and worktree lifecycle management
    forge.psm1          GitHub/Gitea polling, claiming, label transitions, PR creation
    formatter.psm1      Issue prompt formatting for CC
    launcher.psm1       CC headless invocation with timeout enforcement
    post-task.psm1      Push, PR, label update, failure tracking, worktree cleanup
    autonomy.psm1       Tier gate: classifies issues and enforces execution policy
```

### Workspace lifecycle

```text
Initialize-AgentWorkspace  (one-time setup)
    └── git clone --bare <remote> <root>/samverk.git

Per-task lifecycle:
    Update-BareRepo          git fetch --all --prune
    Get-AvailableWorkerSlot  scan worker-1..N for free slots
    New-AgentWorktree        git worktree add -b fix/<N>-<slug> <slot>/ origin/main
    [CC executes in worktree]
    Invoke-PostTask
        Push (if needed)     git push origin <branch>
        Open-PullRequest     gh pr create / Gitea REST
        Update-IssueStatus   add status:needs-qc, remove status:active
        Remove-AgentWorktree git worktree remove --force <slot>/
        Remove-AgentBranch   git branch -D <branch> (in bare repo)
```

## Consequences

### Positive

- The user's working copy is never touched. The invariant is structural, not procedural.
- Worktree creation is instantaneous -- no clone time per task.
- Multiple worktrees can run concurrently (supported by git, enforced by MaxWorkers config).
- The PowerShell module boundary (`Export-ModuleMember`) provides a testable interface.
- Pester unit tests verify tier classification, frontmatter parsing, and gate logic without
  requiring a live forge or git repo.
- The autonomy gate gives the user fine-grained control over what the PC agent will touch.

### Negative

- The bare clone must be kept current via periodic `git fetch`. Stale object stores cause
  agents to work from outdated code.
- Worktree removal can fail if CC left a process holding file handles (Windows file locking).
  `--force` handles most cases but may leave orphaned directories.
- PSScriptAnalyzer linting requires the `PSScriptAnalyzer` module installed locally; CI
  uses the Go/markdown lint pipeline and does not lint PowerShell scripts.

### Neutral

- PowerShell is Windows-only. On macOS/Linux, a Bash equivalent or the future Go binary
  would be needed (not a current requirement).
- The `--dangerously-skip-permissions` flag in CC invocation grants the agent full tool
  access without per-tool confirmation. This is necessary for headless operation but means
  the autonomy gate is the only guardrail before execution begins.

## Future directions

### Go binary: `samverk worker`

Issue #323 tracks replacing the PowerShell scripts with a compiled `samverk worker` subcommand
that integrates with the existing Samverk server and store. The binary would:

- Register as a worker node (ADR-013 forge abstraction applies to the agent side too)
- Report metrics to the Samverk API (visible in the dashboard)
- Use the same SQLite store for session recording, cost tracking, and task profiles
- Support Linux/macOS in addition to Windows
- Be distributed as part of the GoReleaser build pipeline

The worktree isolation model transfers directly: `os/exec`-based `git worktree` calls in Go
are equivalent to the PowerShell `git` invocations. The business logic (autonomy gate, post-task
handler, failure tracking) maps 1:1 to Go packages.

### Multi-slot concurrency (issue #315)

The current default is 1 active slot (single-issue serial execution). The `MaxWorkers`
config and `Get-AvailableWorkerSlot` already support up to 3 concurrent slots. Issue #315
adds resource monitoring (CPU/memory) and concurrent `Invoke-CCTask` calls with per-slot
tracking.

### Multiple worker machines

The bare clone + worktree model extends naturally to multiple machines. Each machine maintains
its own bare clone. The forge is the coordination layer (issue labels prevent double-claiming).
No central coordinator is needed between worker machines.

## References

- [ADR-007: Hybrid Local/Cloud Agents](ADR-007-hybrid-local-cloud.md)
- [ADR-013: Forge Abstraction](ADR-013-forge-abstraction.md)
- [ADR-014: Dedicated Dispatcher](ADR-014-dispatcher-agent.md)
- [ADR-015: Three-Tier Autonomy Model](ADR-015-three-tier-autonomy.md)
- [ADR-021: Intent Verification Protocol](ADR-021-intent-verification.md)
- [ADR-023: Per-Project Repos with Coordination Layer](ADR-023-per-project-repos.md)
- [ADR-031: Dual-Forge Architecture](ADR-031-dual-forge.md)
- [PC Agent Design Doc](../pc-agent-design.md)
- [Issue #304: CC headless invocation research](https://github.com/HerbHall/samverk/issues/304)
