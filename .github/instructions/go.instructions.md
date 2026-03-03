---
applyTo: "**/*.go"
---

# Go Code Instructions

## Error Handling

- Wrap errors with context: `fmt.Errorf("operation: %w", err)`
- Check errors immediately after the call, never defer error checks
- Named return values when returning multiple values
- After adding named returns, use `=` not `:=` for those variables

## Lint Patterns (golangci-lint v2)

These patterns cause CI failures. Avoid them:

- `for _, v := range slice` on structs > 64 bytes: use `for i := range` with `slice[i]`
- `var s []T` in loops: preallocate with `make([]T, 0, len(source))`
- Two consecutive `append()` to same slice: combine into one call
- `db.Exec()` / `db.Query()`: always use `ExecContext` / `QueryContext`
- `len(s) > 0` for strings: use `s != ""`
- `http.NewRequestWithContext(ctx, method, url, nil)`: use `http.NoBody` not `nil`
- `resp.Body.Close()`: use `defer func() { _ = resp.Body.Close() }()`
- Switch on enum types: list ALL cases explicitly, even with default

## Testing

- Table-driven tests with `t.Run(tc.name, func(t *testing.T) {...})`
- Use `testutil.NewStore(t)` for in-memory SQLite test databases
- Test names: `TestFunctionName_Scenario`
- Use `t.Helper()` in test helper functions
- Parallel tests where safe: `t.Parallel()`

## HTTP Handlers

- Use Go 1.22+ patterns: `mux.HandleFunc("GET /api/v1/path", handler)`
- Accept `context.Context` or extract from `r.Context()`
- Write JSON responses with a helper, set Content-Type header
- Return appropriate HTTP status codes

## Imports

- Group: stdlib, blank line, external deps, blank line, internal packages
- No dot imports
- No unused imports

## Project-Specific

- IssueTracker interface in `internal/forge/` abstracts GitHub and Gitea
- Provider interface in `internal/provider/` abstracts AI model clients
- Store interface in `internal/store/` wraps SQLite operations
- Shared types live in `pkg/models/`, not in internal packages
