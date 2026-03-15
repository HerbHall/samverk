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

### Current CLI invocation

```go
// internal/provider/claudecli/claudecli.go:91
args := []string{"--print", "--dangerously-skip-permissions"}
```

### Limitations

1. **No tool use**: `--print` without `--allowedTools` runs text-only
2. **No codebase awareness**: Agent cannot read files to understand context
3. **No self-verification**: Agent cannot run `go build`, `go test`, or lint
4. **Brittle EDIT blocks**: Text-based file edits parsed via regex are error-prone
5. **Single turn**: No iterative problem-solving or error recovery

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

### Architecture after change

```text
Dispatcher -> Runner -> claudecli.Client.Chat()
                            |
                            v
                    exec: claude -p --dangerously-skip-permissions
                         --allowedTools "Bash,Read,Edit,Write,Glob,Grep"
                         --output-format stream-json
                         --max-turns 25
                            |
                            stdin: system prompt + user prompt
                            stdout: stream-json (tool calls, results, final answer)
                            |
                            v
                    Parse stream-json for final assistant message
```

### What changes in the codebase

1. **`internal/provider/claudecli/claudecli.go`**: Add `--allowedTools`, `--output-format`,
   and `--max-turns` to the CLI args. Parse `stream-json` output to extract the final
   assistant message (or fall back to raw text if format is `text`).

2. **`internal/provider/provider.go`**: Consider extending `ProviderConfig` with
   `allowed_tools` and `max_turns` fields so these are configurable per provider entry
   in the YAML config.

3. **Provider config YAML**: Add configuration for the new flags:

   ```yaml
   providers:
     claude-max:
       type: claude-cli
       default_model: "claude-sonnet-4-20250514"
       timeout_seconds: 600
       allowed_tools: "Bash,Read,Edit,Write,Glob,Grep"
       max_turns: 25
   ```

### What does NOT change

- The `Provider` interface stays the same (Chat, Healthy, Name)
- The `Runner` still calls `provider.Chat()` and gets a `ChatResponse`
- The routing, registry, and dispatcher are unaffected
- Ollama and Claude API providers are unaffected

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

### Remaining Tests

- [ ] `--output-format json` response structure
- [ ] `--output-format stream-json` streaming behavior
- [ ] `--max-turns` enforcement
- [ ] `go test` execution via Bash tool
- [ ] Full end-to-end: dispatcher -> runner -> claude-cli with tools -> PR

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

### Why the CLI hangs

The systemd service runs `claude --print --dangerously-skip-permissions` but the
CLI produces nothing. Manual testing as the `samverk` user (same user as the
systemd service) confirms the flag works interactively. The hang in systemd
context is likely caused by:

1. Missing TTY or terminal allocation in the systemd unit
2. The CLI attempting an interactive prompt that silently blocks without a TTY
3. Possible `--dangerously-skip-permissions` behavior difference in non-interactive contexts

The fix (`--allowedTools` with explicit tool list) pre-approves tools so the CLI
never prompts, eliminating the hang regardless of TTY availability.

## Open Questions

1. Does `--max-budget-usd` work with Max plan, or only API billing?
2. Can `--allowedTools` accept MCP tool names for Samverk's own tools?
3. Should we set `--working-directory` to the repo checkout path?
4. Do we need `--no-session-persistence` to avoid filling disk with session files?
5. Should `--fallback-model` be configured to fall back to a cheaper model?
6. Should the repo be cloned to `/var/lib/samverk/repos/samverk` for full codebase access?
7. Should we also try `--permission-mode bypassPermissions` as an alternative?

## References

- Current provider: [`internal/provider/claudecli/claudecli.go`](../internal/provider/claudecli/claudecli.go)
- Provider interface: [`internal/provider/provider.go`](../internal/provider/provider.go)
- Provider registry: [`internal/provider/registry.go`](../internal/provider/registry.go)
- ADR-033: Multi-machine free agent runtime ([`docs/decisions/ADR-033-multi-machine-free-agent-runtime.md`](decisions/ADR-033-multi-machine-free-agent-runtime.md))
- Claude Code headless docs: `claude -p --help`
