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

## Task Context

---
schema_version: 1.1.0
type: task
agent_type: code-gen
priority: high
---

## Problem

PRs merge via auto-merge without reading Copilot review comments. Valid
feedback is ignored, reducing code quality.

## Fix

1. Add a post-CI step in `internal/prwatcher/` that fetches PR review comments
2. Use `gh api repos/{o}/{r}/pulls/{n}/comments` to get Copilot feedback
3. Filter for actionable comments (not just "looks good")
4. If actionable comments exist, create a follow-up commit addressing them
5. Only merge after comments are addressed or explicitly dismissed

See AP#125 for the Copilot post-merge review followup workflow.

## Acceptance Criteria

- PR watcher reads Copilot comments before merging
- Actionable feedback is addressed in follow-up commits
- Non-actionable comments are logged but don't block merge
