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
- `internal/api/pipeline.go`
- `internal/store/pipeline.go`
- `internal/store/audit.go`
- `internal/store/audit_test.go`
- `internal/store/check_in_test.go`
- `internal/store/corrections.go`
- `internal/store/cost.go`
- `internal/store/cost_test.go`
- `internal/store/failure.go`
- `internal/store/failure_test.go`
- `internal/store/kpis.go`
- `internal/store/metric_snapshot.go`
- `internal/store/pipeline_test.go`
- `internal/store/provider_health_snapshot.go`
- `internal/store/provider_success_test.go`
- `internal/store/scaling.go`
- `internal/store/scaling_control.go`
- `internal/store/scaling_test.go`
- `internal/store/session.go`
- `internal/store/session_test.go`
- `internal/store/store.go`
- `internal/store/store_test.go`
- `internal/store/task_profiles.go`
- `internal/store/task_profiles_test.go`
- `internal/api/agents.go`
- `internal/api/api.go`
- `internal/api/api_test.go`
- `internal/api/capacity.go`
- `internal/api/chat.go`
- `internal/api/chat_test.go`
- `internal/api/data_sources.go`
- `internal/api/devkit.go`
- `internal/api/doc.go`
- `internal/api/failures.go`
- `internal/api/forges.go`
- `internal/api/forges_test.go`
- `internal/api/host_metrics.go`
- `internal/api/issue_cost.go`
- `internal/api/issues.go`
- `internal/api/kpis.go`
- `internal/api/log_summary.go`
- `internal/api/logs.go`
- `internal/api/mcp_health.go`
- `internal/api/metrics.go`
- `internal/api/metrics_test.go`
- `internal/api/pipeline_events.go`
- `internal/api/pipeline_test.go`
- `internal/api/pressure.go`
- `internal/api/pressure_test.go`
- `internal/api/projects.go`
- `internal/api/projects_test.go`
- `internal/api/provider_health.go`
- `internal/api/provider_health_test.go`
- `internal/api/recommendations.go`
- `internal/api/scaling.go`
- `internal/api/sessions.go`
- `internal/api/sessions_test.go`
- `internal/api/status.go`
- `internal/api/store_metrics.go`
- `internal/api/synapset_proxy.go`
- `internal/api/workers.go`
- `internal/api/workers_test.go`


## Task Context

---
schema_version: "1.1.0"
type: task
agent_type: code-gen
priority: normal
estimated_tokens: 10000
handoff_ready: true
file_context:
  - internal/api/pipeline.go
  - internal/store/pipeline.go
constraints:
  - define constants in internal/store/pipeline.go (near existing pipeline code)
  - replace all hardcoded stage strings in internal/api/pipeline.go with constants
  - do not change any behavior or API responses
  - run make ci before finishing
---

## Summary

Replace hardcoded pipeline stage name strings with typed constants.

## Context

internal/api/pipeline.go classifyStage() uses hardcoded strings like "queued", "claimed", "in-progress", etc. These should be constants for type safety and to prepare for the planning system adding new stages.

## Required Changes

1. Add stage name constants to internal/store/pipeline.go
2. Replace all hardcoded strings in internal/api/pipeline.go with the new constants
3. Ensure existing tests still pass (no behavior change)

## Acceptance Criteria

- [ ] All pipeline stage strings are constants
- [ ] No hardcoded stage strings remain in pipeline.go
- [ ] All existing tests pass unchanged
- [ ] make ci passes

