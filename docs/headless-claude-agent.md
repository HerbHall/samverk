# Headless Claude Agent via Max Plan

## Problem

Samverk's `claudecli` provider shells out to `claude --print --dangerously-skip-permissions`
to run tasks using the user's Max plan subscription. This works but operates in **text-only
mode** -- the CLI receives a prompt and returns text. It cannot:

- Read files from the repository
- Edit or create files
- Run shell commands (build, test, lint)
- Search the codebase
- Verify its own work

This means Claude agent sessions produce text responses posted as issue comments. For
code-gen and test tasks, the runner parses EDIT blocks from the text and creates PRs via
the forge API. The agent has no ability to understand the actual codebase, run tests, or
iterate on failures.

Meanwhile, the Max plan subscription is paid for and underutilized. The Agent SDK requires
separate API billing, which is not an option.

## Current Architecture

```text
Dispatcher -> Runner -> claudecli.Client.Chat()
                            |
                            v
                    exec: claude --print --dangerously-skip-permissions
                            |
                            stdin: system prompt + user prompt (issue body)
                            stdout: text response (parsed for EDIT blocks)
```

### Current CLI invocation (after PR #298)

```go
// internal/provider/claudecli/claudecli.go
args := []string{
    "--print",
    "--dangerously-skip-permissions",
    "--no-session-persistence",
    "--output-format", "stream-json",
    "--verbose",
}
```

### Resolved Limitations (as of 2026-03-25)

1. **Tool use**: `--allowedTools "Bash,Read,Edit,Write,Glob,Grep"` enables full agentic mode
2. **Codebase awareness**: Agent reads files, searches code via Glob/Grep
3. **Self-verification**: Agent runs `go build`, `go test`, lint via Bash tool
4. **Direct file edits**: Agent uses Edit/Write tools in isolated worktrees
5. **Multi-turn**: `--max-turns 25` allows iterative problem-solving

## Proposed Solution

Claude Code's `-p` flag (same as `--print`) supports full agentic tool calling when
combined with `--allowedTools`. This unlocks Read, Edit, Write, Bash, Glob, and Grep
tools -- all using the existing Max plan subscription.

### Target CLI invocation

```bash
claude -p "task prompt" \
  --dangerously-skip-permissions \
  --allowedTools "Bash,Read,Edit,Write,Glob,Grep" \
  --output-format stream-json \
  --max-turns 25
```

### Key flags

| Flag | Purpose |
|------|---------|
| `--allowedTools "Bash,Read,Edit,Write,Glob,Grep"` | Pre-approve tools so the agent can act without interactive prompts |
| `--output-format stream-json` | Structured output with tool call details, streaming for activity detection |
| `--max-turns 25` | Safety limit to prevent runaway sessions |
| `--max-budget-usd 5` | Cost guard (optional, may not apply to Max plan) |
| `--fallback-model` | Auto-fallback if primary model is overloaded |

### Current Architecture (implemented)

```text
Dispatcher -> Runner -> claudecli.Client.Chat()
                            |
                            v
                    exec: claude --print --dangerously-skip-permissions
                         --no-session-persistence
                         --output-format stream-json --verbose
                         --allowedTools "Bash,Read,Edit,Write,Glob,Grep"
                         --max-turns 25 --model <model>
                            |
                            stdin: system prompt + user prompt (issue body)
                            stdout: stream-json events (one JSON line per event)
                            |
                            v
                    streamOutput() parses JSON lines via bufio.Scanner
                    Each line resets activity timer (proof-of-life)
                    Final result extracted from {"type":"result"} event
```

### stream-json Event Flow

The CLI emits one JSON object per line as events occur:

| Event | When | Key Fields |
|-------|------|-----------|
| `{"type":"system","subtype":"init"}` | Startup (~2s) | session_id, tools, mcp_servers |
| `{"type":"assistant"}` | Each LLM turn | message content, tool_use |
| `{"type":"user"}` | Tool results | tool_use_id, content |
| `{"type":"rate_limit_event"}` | After API call | rate_limit_info |
| `{"type":"result"}` | Session end | result (final text), is_error, duration_ms |

### Why stream-json (not plain --print)

Plain `--print` mode buffers ALL stdout until the session ends. A multi-turn
agentic session produces exactly 0 bytes on stdout for minutes, then one final
`write(1,...)` syscall. This caused a 56% "startup timeout" failure rate because
our activity timer saw no output and killed active sessions.

With `--output-format stream-json --verbose`, events stream in real-time. The
`init` event arrives in ~2 seconds, and each subsequent turn/tool-call produces
events. The activity timer resets on every JSON line.

**Requires `--verbose`** -- stream-json without it produces an error.

### Timeout Configuration

| Timeout | Value | Rationale |
|---------|-------|-----------|
| startupTimeout | 30s | Init event arrives in ~2s; 30s catches real crashes |
| staleOutputTimeout | 120s | API round-trips between events can take 30-60s |
| Provider timeout | 600s (configurable) | Overall session limit from providers.yaml |

Previously startupTimeout was 300s (to accommodate plain --print buffering).

### Provider config YAML

```yaml
providers:
  claude-sonnet:
    type: claude-cli
    default_model: claude-sonnet-4-6
    timeout_seconds: 600
    allowed_tools: "Bash,Read,Edit,Write,Glob,Grep"
    max_turns: 25
```

## Prerequisites

### Authentication on CT 202

The Max plan OAuth credentials must exist on the server:

```bash
# Option A: Copy from desktop
rsync -avz ~/.claude/ root@192.168.1.162:~/.claude/

# Option B: Login on server (needs browser redirect)
ssh root@192.168.1.162
claude auth login

# Verify:
claude auth status
```

### Claude Code installed on CT 202

```bash
ssh root@192.168.1.162
which claude  # should resolve
claude --version
```

## Testing Plan

### Phase 1: Manual CLI test on CT 202

SSH to CT 202 and run a simple agentic task to confirm tools work:

```bash
ssh root@192.168.1.162
cd /opt/samverk  # or wherever the repo checkout lives

echo "List the Go files in internal/agent/ and summarize what each does" | \
  claude -p --dangerously-skip-permissions \
  --allowedTools "Read,Glob,Grep" \
  --output-format json
```

**Success criteria**: Response includes actual file contents, not generic guesses.

### Phase 2: Test with Bash tool

```bash
echo "Run 'go test ./internal/agent/...' and report the results" | \
  claude -p --dangerously-skip-permissions \
  --allowedTools "Bash,Read" \
  --output-format json
```

**Success criteria**: Response includes actual test output.

### Phase 3: Test with Edit tool

Create a throwaway branch and test code modification:

```bash
echo "Create a file /tmp/test-edit.go with a Hello World program" | \
  claude -p --dangerously-skip-permissions \
  --allowedTools "Bash,Write,Read" \
  --output-format json
```

**Success criteria**: File is created on disk.

### Phase 4: Update claudecli provider

If manual tests pass, update the Go code to use the new flags and parse
structured output.

### Phase 5: End-to-end test

Create a test issue on Gitea, let the dispatcher pick it up, and verify
the agent produces a working PR (not just text).

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Max plan rate limits under heavy use | `--max-turns 25` caps each session; monitor for 429s |
| OAuth token expiry on server | Check `claude auth status` in health probe; alert on failure |
| Agent modifies wrong files | `--allowedTools` scoping; work in branch; dispatcher PR review |
| `stream-json` format changes between CLI versions | Pin CLI version; parse defensively |
| Agent hangs in tool loop | Existing stale-output timeout (3 min) + `--max-turns` |
| Credential theft from CT 202 | Container is on private LAN; SSH key-only access |

## Test Results (2026-03-15)

All manual tests passed on CT 202 (LXC container, Debian, `samverk` user).

### Environment

- Claude Code v2.1.71 installed at `/usr/bin/claude`
- Auth: Max plan credentials at `/var/lib/samverk/.claude/.credentials.json`
- Service user: `samverk` (uid 999), home `/var/lib/samverk`
- `--dangerously-skip-permissions` works as non-root `samverk` user
- **Does NOT work as root** -- Claude Code blocks it for security

### Phase 1: Read/Glob/Grep tools -- PASS

```bash
echo "Read internal/agent/pool.go and describe the Pool struct fields" | \
  claude -p --dangerously-skip-permissions --allowedTools "Read,Glob,Grep"
```

Result: Agent read the actual file and returned accurate struct field descriptions
with types and purposes. Not hallucinated -- matched real code.

### Phase 2: Bash tool -- PASS

```bash
echo "Run: ls -la internal/agent/ and report file sizes" | \
  claude -p --dangerously-skip-permissions --allowedTools "Bash,Read"
```

Result: Agent ran the command and reported correct file names and sizes.

### Phase 3: Write tool -- PASS

```bash
echo "Create /tmp/samverk-test.txt with content: Headless agent test successful" | \
  claude -p --dangerously-skip-permissions --allowedTools "Bash,Write,Read"
```

Result: File created on disk with correct content. Verified with `cat`.

### Key Findings

1. **Root restriction**: `--dangerously-skip-permissions` is blocked when running as
   root. The `samverk` systemd user works fine.
2. **Auth already in place**: Credentials were already at `/var/lib/samverk/.claude/`
   and the systemd service already has `ReadWritePaths=/var/lib/samverk/.claude`.
3. **Working directory matters**: The repo checkout at `/var/lib/samverk` is partial
   (only some Go files). For full agentic work, the agent needs access to a complete
   repo checkout. Consider cloning to `/var/lib/samverk/repos/<project>`.
4. **No JSON output tested yet**: `--output-format json` and `stream-json` need testing
   for structured response parsing in the Go provider.

### Remaining Tests (all resolved 2026-03-25)

- [x] `--output-format stream-json` streaming behavior -- implemented in PR #298
- [x] `--max-turns` enforcement -- configured per-provider in providers.yaml
- [x] `go test` execution via Bash tool -- agents run `make ci` in worktrees
- [x] Full end-to-end: dispatcher -> runner -> claude-cli with tools -> PR -- verified with issue #299

## Root Cause Confirmed (2026-03-15)

Dispatcher logs from CT 202 confirm the exact failure pattern. Every code-gen
task fails identically:

```text
{"level":"error","msg":"task failed",
 "error":"provider chat: claude-cli: hung: no output for 3m0s: output: ",
 "duration":180.7}
```

The CLI starts, produces **zero bytes** of output, and the 3-minute stale-output
timer kills it. This happens on every issue (#473, #475, #479, #485, #494, #500).

The circuit breaker correctly opens after 3 consecutive provider failures:

```text
{"level":"warn","msg":"provider circuit OPEN","provider":"default","failures":3,"cooldown":900}
```

Issues are classified as `provider_down` and not counted toward per-issue
escalation (fix from #482), so they stay `status:queued` and retry after the
15-minute cooldown -- but they fail again in the same way.

### Why the CLI appeared to hang (resolved 2026-03-25)

The systemd service ran `claude --print` which buffers ALL stdout until the
entire agentic session ends. A session running 25 tool-call turns for 5+ minutes
produces exactly 0 bytes on stdout during execution -- one final `write(1,...)`
syscall at the very end. Confirmed via strace.

The 300-second startup timeout killed these active-but-buffering sessions,
classifying them as "hung". This was a 56% failure rate (26 hangs in 48h).

**Root cause was NOT**: TTY, interactive prompts, OAuth contention, or concurrency.
Controlled tests showed hangs occurred even with zero other active CLI processes.

**Fix**: `--output-format stream-json --verbose` (PR #298). Events stream in
real-time during execution. The init event arrives in ~2s, immediately satisfying
the startup timeout. Each subsequent turn produces events that reset the stale
timer. Startup timeout reduced from 300s to 30s.

## Open Questions (resolved)

1. ~~Does `--max-budget-usd` work with Max plan?~~ -- Not used; Max plan has no per-session billing
2. ~~Can `--allowedTools` accept MCP tool names?~~ -- Yes, but MCP servers configured in .claude.json load automatically
3. ~~Should we set `--working-directory`?~~ -- Yes, `cmd.Dir` is set to the worktree path by the runner
4. ~~Do we need `--no-session-persistence`?~~ -- Yes, prevents session file accumulation (456 files observed)
5. ~~Should `--fallback-model` be configured?~~ -- No, failover is handled by the routing chain in the dispatcher
6. ~~Should the repo be cloned for full codebase access?~~ -- Yes, per-project repo dirs at `/var/lib/samverk/repo`, `/var/lib/samverk/devkit-repo`, `/var/lib/samverk/synapset-repo` (PR #268)
7. ~~Should we use `--permission-mode bypassPermissions`?~~ -- No, `--dangerously-skip-permissions` is equivalent and simpler

## References

- Current provider: [`internal/provider/claudecli/claudecli.go`](../internal/provider/claudecli/claudecli.go)
- Provider interface: [`internal/provider/provider.go`](../internal/provider/provider.go)
- Provider registry: [`internal/provider/registry.go`](../internal/provider/registry.go)
- ADR-033: Multi-machine free agent runtime ([`docs/decisions/ADR-033-multi-machine-free-agent-runtime.md`](decisions/ADR-033-multi-machine-free-agent-runtime.md))
- Claude Code headless docs: `claude -p --help`
