# Samverk -- Copilot Instructions

Samverk is an async background development engine. Go backend with React
TypeScript dashboard embedded via `go:embed`.

## Architecture

- Single binary: `cmd/samverk/main.go` with cobra subcommands
- Internal packages under `internal/` (server, api, mcp, dispatcher, forge, agent, provider, autonomy, profile, cost, store, digest, version)
- Shared types in `pkg/models/`
- React SPA in `web/` embedded at build time
- SQLite for persistence (`modernc.org/sqlite`, pure Go, no CGO)
- Config via YAML files in `.samverk/`

## Code Style

- Conventional commits: `feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`
- No over-engineering -- implement only what's needed now
- Comments only where logic isn't self-evident
- Validate at system boundaries, not internal calls
- Remove unused code completely -- no backwards-compatibility hacks

## Go Conventions

- Use `net/http` with Go 1.22+ routing patterns, no web frameworks
- Named returns on functions returning multiple values
- Table-driven tests with `t.Run` subtests
- Error wrapping with `fmt.Errorf("context: %w", err)`
- `context.Context` as first parameter
- Preallocate slices: `make([]T, 0, len(source))` not `var s []T`
- Use `for i := range slice` with `slice[i]` for structs > 64 bytes
- Combine consecutive `append()` calls to the same slice
- Always use `ExecContext`/`QueryContext` (never `Exec`/`Query`)
- Close `*http.Response` bodies explicitly: `defer func() { _ = resp.Body.Close() }()`

## React/TypeScript Conventions

- React 19 + Vite 6 + TypeScript strict mode
- shadcn/ui components with Tailwind CSS
- TanStack Query for server state, Zustand for client state
- All API calls through typed fetch wrappers
- No `any` type -- use `unknown` with type guards
- Verify every import is used (ESLint catches unused imports)

## Testing

- Go: table-driven tests, `testutil.NewStore(t)` for SQLite fixtures
- Frontend: Vitest + Testing Library
- Run `go build ./...`, `go test ./...`, golangci-lint before committing

## Do NOT

- Use LangChain, LangGraph, CrewAI, or agent frameworks
- Use gRPC, PostgreSQL, Redis, or web frameworks (gin/echo/fiber)
- Add dependencies without checking if stdlib covers the need
- Commit directly to `main` -- always use feature branches
