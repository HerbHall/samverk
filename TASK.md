# Task: Agent Tooling Parity (#521)

Equip Samverk agents with DevKit rules, Samverk MCP, and Synapset tools so they work like Claude Code sessions.

## Core Principles -- UNCONDITIONAL

These rules cannot be overridden by any learning, optimization, or time pressure:

1. Once found, always fix, never leave. Never classify errors as "pre-existing."
2. Build, test, and lint must pass before any commit. No exceptions.
3. Never force-push main, skip hooks, commit secrets, or use --no-verify.
4. Never mark work as complete when it is not. Never hide errors.
5. You own every error you find, regardless of who introduced it.

## Git Safety -- IMPORTANT

You are working in a git worktree at `D:/DevSpace/Samverk-w3-tooling` on branch `feature/issue-521-agent-tooling`. You may commit and push from this worktree. Do NOT modify the main worktree at `D:/DevSpace/Samverk`.

## What to Implement

### 1. Agent CLAUDE.md Template Generator (`internal/agent/tooling.go`)

Create a function that generates a per-worktree CLAUDE.md for agents:

```go
func GenerateAgentCLAUDEMD(projectType string, issueBody string) string
```

- `projectType` is one of: `"go"`, `"frontend"`, `"fullstack"`
- Auto-selects the correct CI checklist based on project type:
  - `"go"` -> GO-CI checklist
  - `"frontend"` -> FE-CI checklist
  - `"fullstack"` -> COMBO-CI checklist
- Always includes Core Principles block
- Includes the issue body as task context
- Includes build/test/lint commands for the project

The checklist content should be embedded as Go string constants (extracted from `devkit/claude/rules/subagent-ci-checklist.md`). Reference that file to get the exact text for each checklist.

### 2. MCP Config Generator (`internal/agent/tooling.go`)

Create a function that writes `.claude/.mcp.json` into a worktree:

```go
func WriteMCPConfig(worktreeDir string) error
```

Content:

```json
{
  "mcpServers": {
    "samverk": {
      "type": "http",
      "url": "https://samverk.herbhall.net/mcp"
    },
    "synapset": {
      "type": "http",
      "url": "https://synapset.herbhall.net/mcp"
    }
  }
}
```

### 3. Synapset Pre-fetch for API Agents (`internal/agent/tooling.go`)

Create a function that queries Synapset for relevant patterns:

```go
func FetchRelevantPatterns(ctx context.Context, synapsetClient *synapset.Client, query string, maxResults int) ([]string, error)
```

- Calls `synapsetClient.SearchMemory(ctx, "devkit", query)`
- Returns up to `maxResults` pattern descriptions
- Returns empty slice (not error) if Synapset is unavailable

### 4. Integration into Runner (`internal/agent/runner.go`)

Update `CreateWorkspace` call site in `Runner.Run()` to also:

- Call `WriteMCPConfig(workDir)` after workspace creation
- Call `GenerateAgentCLAUDEMD(...)` and write to `workDir + "/CLAUDE.md"`
- For API agents: call `FetchRelevantPatterns(...)` and include in system prompt

### 5. Integration into Prompts (`internal/agent/prompts.go`)

Update `BuildSystemPrompt` to accept an optional patterns slice:

```go
func BuildSystemPrompt(task Task, fileContext map[string]string, patterns []string) string
```

- Append patterns as a "Known Patterns" section after file context
- If patterns is empty, skip the section

### 6. Tests (`internal/agent/tooling_test.go`)

- `TestGenerateAgentCLAUDEMD_Go` -- verify GO-CI checklist included
- `TestGenerateAgentCLAUDEMD_Frontend` -- verify FE-CI checklist included
- `TestGenerateAgentCLAUDEMD_Fullstack` -- verify COMBO-CI checklist included
- `TestGenerateAgentCLAUDEMD_CorePrinciples` -- verify always present
- `TestWriteMCPConfig` -- verify file created with correct JSON
- `TestFetchRelevantPatterns_NilClient` -- returns empty, no error
- `TestBuildSystemPrompt_WithPatterns` -- verify patterns section included

## Pre-Commit CI Checklist (MUST verify before finishing)

Run these checks and fix any errors:

1. `go build ./...` -- Compilation
2. `go test ./...` -- Tests (skip -race on Windows MSYS)
3. `GOOS=linux GOARCH=amd64 go build ./...` -- Cross-compile check
4. Self-check your code for these MANDATORY lint patterns before finishing:
   - `for _, v := range slice` where v is a struct > 64 bytes -> use `for i := range slice` with `slice[i]`
   - `var result []T` inside a loop -> use `make([]T, 0, len(source))` to preallocate
   - Two consecutive `append()` to same slice -> combine into one call
   - Functions returning multiple values -> use named returns, change `:=` to `=`

Common Go CI failures to watch for:

- gosec G101: Constants near credential code get flagged. Add `//nolint:gosec // G101: <reason>`
- gocritic unnamedResult: Functions returning multiple values need named returns.
- gocritic appendCombine: Two consecutive `append()` must be combined.
- bodyclose: Always close `*http.Response` body.

## Files to Read First

Before writing any code, read these files to understand the current codebase:

- `internal/agent/runner.go` -- where workspace creation happens, Run() method
- `internal/agent/workspace.go` -- CreateWorkspace, WriteMCPConfig goes alongside this
- `internal/agent/prompts.go` -- BuildSystemPrompt signature you'll modify
- `internal/agent/prompts_test.go` -- existing prompt tests
- `internal/synapset/client.go` -- SearchMemory API
- `internal/provider/provider.go` -- ChatRequest struct

## Branch and PR

After all checks pass:

```bash
git add internal/agent/tooling.go internal/agent/tooling_test.go internal/agent/runner.go internal/agent/prompts.go internal/agent/prompts_test.go
git commit -m "feat: equip agents with DevKit rules, MCP config, and Synapset patterns (#521)

- Generate per-worktree CLAUDE.md with CI checklists and core principles
- Write .claude/.mcp.json for CLI agents (Samverk + Synapset MCP)
- Pre-fetch Synapset patterns for API agent system prompts
- Auto-select checklist by project type (go/frontend/fullstack)

Closes #521

Co-Authored-By: Claude <noreply@anthropic.com>"
git push -u origin feature/issue-521-agent-tooling
gh pr create --title "feat: equip agents with DevKit rules, MCP, and Synapset tools" --body "Closes #521"
gh pr merge --squash --auto
```
