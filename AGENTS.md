<!--
  Scope: AGENTS.md guides the Copilot coding agent and Copilot Chat.
  For code completion and code review patterns, see .github/copilot-instructions.md
  and .github/instructions/*.instructions.md
  For Claude Code, see CLAUDE.md
-->

# Samverk

Async background development engine -- keeps your project building while you live your life. Multi-forge development workflow manager that manages Git repositories across GitHub, Gitea, and other forges.

## Tech Stack

- **Language:** Go 1.25 -- single binary with Cobra subcommands (`samverk serve`, `samverk dispatch`, `samverk config`)
- **AI Providers:** Anthropic Claude API (primary), OpenAI/GPT-4, Gemini, Ollama (local containers)
- **Web Dashboard:** React 19 + TypeScript + Vite, embedded in Go binary via `embed.FS`
- **Frontend:** shadcn/ui, Tailwind CSS, TanStack Query, Zustand, Recharts
- **State:** Git issues (tasks) + SQLite (sessions, cost, audit) + YAML (config)
- **Git Forge:** GitHub (primary), Gitea (self-hosted), abstracted behind `IssueTracker` interface
- **SDKs:** Gitea SDK, GitHub Go SDK, MCP Go SDK
- **Testing:** Go stdlib + table-driven tests, Vitest + Testing Library (frontend)
- **Build:** Make + GoReleaser
- **CI/CD:** GitHub Actions
- **Linting:** golangci-lint v2 (Go), ESLint + TypeScript (frontend), markdownlint (docs)

## Build and Test Commands

```bash
# Build (compiles frontend first, then Go binary)
make build          # or: go build ./...

# Test
make test           # or: go test ./...

# Lint
make lint           # or: golangci-lint run ./...

# Markdown lint
make lint-md

# Full verification (run before any PR)
make ci             # or: go build ./... && go test ./... && golangci-lint run ./...

# Frontend only
make web            # Build frontend to internal/server/static/
make dev-web        # Run Vite dev server
```

## Project Structure

```text
samverk/
├── cmd/samverk/              # Single binary entrypoint (Cobra subcommands)
├── internal/
│   ├── server/               # HTTP server (MCP + API + embedded SPA)
│   ├── api/                  # REST API handlers for dashboard
│   ├── mcp/                  # MCP protocol handler (Streamable HTTP)
│   ├── dispatcher/           # Issue watcher, task router, dependency DAG
│   ├── digest/               # Check-in digest builder and formatting
│   ├── forge/                # IssueTracker interface + GitHub/Gitea implementations
│   ├── agent/                # Agent runtime, container management
│   ├── provider/             # AI provider clients (Claude, OpenAI, Ollama)
│   ├── autonomy/             # Trust tier evaluation engine
│   ├── profile/              # User profile management
│   ├── cost/                 # Token tracking, budget, attribution
│   ├── store/                # SQLite persistence layer
│   └── version/              # Build version injection (ldflags)
├── pkg/models/               # Shared types (Issue, Agent, Action, etc.)
├── web/                      # React SPA dashboard (Vite + TypeScript + pnpm)
├── docs/                     # Design docs and ADRs
├── deploy/                   # Deployment configurations
├── scripts/                  # Build and utility scripts
├── bin/                      # Compiled binaries (gitignored)
├── .github/workflows/        # CI/CD pipelines
└── Makefile                  # Build and development tasks
```

## Workflow Rules

### Always Do

- Create a feature branch for every change (`feature/issue-NNN-description`)
- Use conventional commits: `feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`
- Run build, test, and lint before opening a PR
- Write table-driven tests with descriptive names
- Wrap errors with context: `fmt.Errorf("operation: %w", err)`
- Fix every error you find, regardless of who introduced it

### Ask First

- Adding new dependencies (check if stdlib covers the need)
- Architectural changes (new packages, major interface changes)
- Database schema migrations
- Changes to CI/CD workflows
- Removing or renaming public APIs
- Changes to the forge abstraction layer (`IssueTracker` interface)

### Never Do

- Commit directly to `main` -- always use feature branches
- Skip tests or lint checks -- even for "small changes"
- Use `--no-verify` or `--force` flags
- Commit secrets, credentials, or API keys
- Add TODO comments without a linked issue number
- Mark work as complete when build, test, or lint failures remain

## Core Principles

These are unconditional -- no optimization or time pressure overrides them:

1. **Quality**: Once found, always fix, never leave. There is no "pre-existing" error.
2. **Verification**: Build, test, and lint must pass before any commit.
3. **Safety**: Never force-push `main`. Never skip hooks. Never commit secrets.
4. **Honesty**: Never mark work as complete when it is not.

## Error Handling

```go
// Wrap errors with context -- every return site should add meaning
if err != nil {
    return fmt.Errorf("load config: %w", err)
}

// Use sentinel errors for caller-distinguishable conditions
var ErrNotFound = errors.New("not found")
if errors.Is(err, ErrNotFound) { ... }
```

## Testing Conventions

```go
// Table-driven tests with descriptive names
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {
            name:  "valid input returns expected output",
            input: "example",
            want:  "result",
        },
        {
            name:    "empty input returns error",
            input:   "",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := FunctionName(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("FunctionName() error = %v, wantErr %v", err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("FunctionName() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Commit Format

```text
feat: add user authentication endpoint

Implements JWT-based login and token refresh. Tokens expire after 1h.

Closes #42
Co-Authored-By: GitHub Copilot <copilot@github.com>
```

Types: `feat` (new feature), `fix` (bug fix), `refactor` (no behavior change),
`docs` (documentation only), `test` (tests only), `chore` (build/tooling).
