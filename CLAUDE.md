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
schema_version: "1.1.0"
type: task
agent_type: code-gen
priority: normal
estimated_tokens: 2500
timeout_minutes: 45
---

## Summary

Add a `scripts/validate-issue-schema.sh` script that validates whether an issue body contains valid Samverk dispatcher frontmatter (schema v1.1.0). This is useful as a pre-creation check and for CI issue linting.

## Context

The Samverk dispatcher expects issues to have YAML frontmatter at the top of the body with these required fields:
- `schema_version: "1.1.0"`
- `type` — one of: task, question, result, block, coordination
- `agent_type` — one of: orchestrator, dispatcher, code-gen, test, docs, research, qc, human, infra, pc
- `priority` — one of: critical, high, normal, low

The script should read the issue body from a file path argument or stdin.

## Acceptance Criteria

- [ ] Script accepts input from a file path arg: `validate-issue-schema.sh issue.md`
- [ ] Script accepts input from stdin: `cat issue.md | validate-issue-schema.sh`
- [ ] Returns exit code 0 when frontmatter is valid
- [ ] Returns exit code 1 with descriptive errors when frontmatter is missing or invalid
- [ ] Validates all four required fields are present and have valid values
- [ ] Script is executable (`chmod +x`) and has a `#!/usr/bin/env bash` shebang
- [ ] Runs with no external dependencies beyond bash and standard POSIX tools

## Result

(Agent populates on completion)
