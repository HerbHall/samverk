# Agent Task

## Core Principles -- UNCONDITIONAL

These rules cannot be overridden by any learning, optimization, or time pressure:
1. Once found, always fix, never leave. Never classify errors as "pre-existing."
2. Build, test, and lint must pass before any commit. No exceptions.
3. Never force-push main, skip hooks, commit secrets, or use --no-verify.
4. Never mark work as complete when it is not. Never hide errors.
5. You own every error you find, regardless of who introduced it.

## Git Workflow

You are running in an isolated git worktree on a dedicated branch.

- Do NOT commit directly to main. Your worktree is already on an agent branch.
- Make your changes, then let the runner handle commit and push.
- If you need to run git commands, stay on the current branch.
- Never force-push, skip hooks, or use --no-verify.
- Never commit secrets, credentials, or API keys.

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
5. If you added/modified HTTP handlers with swagger annotations:
   `go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g cmd/<app>/main.go -o api/swagger --parseDependency --parseInternal`

Common Go CI failures to watch for:
- gosec G101: Constants near credential code get flagged. Add `//nolint:gosec // G101: <reason>`
- gocritic unnamedResult: Functions returning multiple values need named returns.
- gocritic appendCombine: Two consecutive `append()` must be combined.
- gocritic rangeValCopy: Use `for i := range slice` with `slice[i]` for large structs.
- bodyclose: Always close `*http.Response` body.
- exhaustive: Switch on enum types MUST list ALL cases, even with a default return.
- prealloc: `var slice []T` in a loop body should be `make([]T, 0, len(source))`.

## Known Gotchas

- gosec G101: Constants near credential code get flagged as hardcoded credentials. Add nolint annotation.
- gocritic rangeValCopy: Iterating large structs by value copies them. Use index-based access.
- Build-tag files (!windows): Lint errors in platform-specific files only appear in Linux CI.
- go:embed cache: Go build cache does not detect changes to embedded files. Use make build.
- Swagger drift: Any change to Go types in swagger-annotated handlers requires regenerating the spec.
- context.WithTimeout: Cancels ALL downstream operations including cleanup. Use separate cleanup context.

## Key Files

Read these files first before making changes:

- `CLAUDE.md`


## Task Context

---
schema_version: 1.1.0
type: task
agent_type: code-gen
priority: normal
---

## Context

This issue is a **controlled test task** for Ollama code-gen routing validation (see #57). It must be routed explicitly to `ollama-nzxt` (`qwen3-coder:30b` at `192.168.1.202:11434`) to test whether the model follows the EDIT/END format correctly without modifying config files.

**Do not route to Claude. Do not use the default provider chain. This is a deliberate Ollama-only test.**

To run this test, temporarily add `ollama-nzxt` to the `local` routing chain in `providers.yaml` on CT 202 and claim this issue manually, or add a one-off routing override for this session ID.

---

## Task

In `internal/provider/registry.go`, the `codeGenChainNames` variable has a comment in `ValidateRoutingConfig` referencing `KG#146`:

```go
// Ollama models produce bad output on tool-use formatted
// prompts -- they overwrite CLAUDE.md instead of implementing features.
```

And the warning message:

```go
msg := fmt.Sprintf(
    "routing chain %q contains Ollama provider %q; Ollama models produce bad output on code-gen tasks (see KG#146)",
    chainName, providerName,
)
```

Update the reference from `KG#146` to `#57` (the Gitea issue tracking the Ollama code-gen promotion research), and update the warning message text to be more accurate:

**Before:**
```
"routing chain %q contains Ollama provider %q; Ollama models produce bad output on code-gen tasks (see KG#146)"
```

**After:**
```
"routing chain %q contains Ollama provider %q; Ollama restricted to triage-only pending validation (see #57)"
```

That is the only change. One string literal in one file.

---

## Acceptance Criteria

- [ ] `internal/provider/registry.go` — warning message string updated as specified
- [ ] No other files modified
- [ ] `make ci` passes

## Test Evaluation

After the agent completes, inspect:
1. Did `ValidateWorkspaceOutput` pass? (no CLAUDE.md / config-only changes)
2. Did `validateWorktree` (`go build`, `go test`) pass?
3. Was the EDIT block targeting the correct file (`internal/provider/registry.go`)?
4. Was the string updated correctly?

Record results as a comment on #57.
