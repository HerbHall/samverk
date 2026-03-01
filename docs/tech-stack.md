# Tech Stack

Comprehensive technology choices for Samverk. For architectural decisions, see [Architecture](architecture.md) and the [ADR index](decisions/README.md).

## Core Language: Go

Decided in [ADR-005](decisions/ADR-005-go-language.md). Single-binary deployment, strong concurrency (goroutines for dispatcher and agent watchers), cross-platform compilation, consistent with SubNetree project experience.

## Deployment Model: Single Binary

One binary with subcommands:

```text
samverk serve      # MCP server + REST API + embedded dashboard
samverk dispatch   # Issue watcher, task router, agent orchestrator
samverk config     # Setup, configuration management
samverk version    # Version info
```

Can be split into separate binaries later if deployment topology demands it. Single binary is simpler to build, test, and deploy for v0.0.1.

## Go Libraries

### Core Dependencies

| Need | Library | Why |
|------|---------|-----|
| HTTP server | `net/http` (stdlib) | MCP Streamable HTTP is JSON-RPC over HTTP. No framework needed. Go 1.22+ routing patterns are sufficient. |
| CLI framework | `spf13/cobra` | Standard Go CLI framework, subcommand pattern |
| GitHub API | `google/go-github/v68` | Mature, well-maintained, covers full Issues/PRs API |
| Gitea API | `code.gitea.io/sdk/gitea` | Official SDK, mirrors GitHub API patterns |
| Claude API | `anthropics/anthropic-sdk-go` | Official Go SDK *(Phase 2 -- not yet in go.mod)* |
| OpenAI/GPT-4 | `sashabaranov/go-openai` | Most popular Go client, supports all endpoints *(Phase 2 -- not yet in go.mod)* |
| Ollama (local) | Raw `net/http` | Ollama's REST API is trivial (3 endpoints). No SDK needed. **Implemented.** |
| Docker mgmt | `docker/docker/client` | Manage local agent containers programmatically *(Phase 2 -- not yet in go.mod)* |
| Config (YAML) | `gopkg.in/yaml.v3` | Direct YAML parsing for `.samverk/*.yaml`. Viper is overkill -- no env var layering needed since config is file-based. **Implemented.** |
| Logging | `uber-go/zap` | Structured logging, fast, consistent with SubNetree *(Phase 2 -- not yet in go.mod)* |
| SQLite | `modernc.org/sqlite` | Pure Go (no CGO required), for local state persistence. **Implemented.** |

### What NOT to Use

| Avoided | Why |
|---------|-----|
| LangChain / LangGraph / CrewAI | Custom orchestration per [ADR-004](decisions/ADR-004-custom-orchestration.md) |
| gRPC / protobuf | MCP spec uses JSON-RPC over HTTP, not gRPC |
| PostgreSQL / MySQL | SQLite is sufficient for single-host state. Git issues are the primary "database" for task state. |
| Redis | No caching layer needed. Dispatcher polls or receives webhooks. |
| Gemini SDK | `sashabaranov/go-openai` supports Gemini's OpenAI-compatible endpoint. Separate SDK unnecessary unless using Gemini-specific features. |
| Web framework (gin / echo / fiber) | `net/http` with Go 1.22+ routing patterns handles MCP + REST API endpoints |

## Web Dashboard

The chat (Claude + MCP) is the primary interface for project progress and decisions ([ADR-011](decisions/ADR-011-chat-as-interface.md)). The web dashboard handles operational concerns: configuration, monitoring, logs, troubleshooting ([ADR-020](decisions/ADR-020-web-dashboard.md)).

### Frontend Stack

| Choice | Pick | Why |
|--------|------|-----|
| Framework | React + TypeScript | Consistent with SubNetree experience, reusable patterns |
| Build tool | Vite | Fast builds, proven in SubNetree |
| UI components | shadcn/ui + Tailwind CSS | Composable, accessible, same as SubNetree |
| Server state | TanStack Query | Cache management, background refetching |
| Client state | Zustand | Lightweight, no boilerplate |
| Charts/metrics | Recharts | Known quirks documented, adequate for dashboards |
| Routing | React Router | Standard SPA routing |

### Embedding

The React SPA is embedded in the Go binary via `embed.FS`. Single binary deployment stays intact -- no separate web server process, no CORS, no reverse proxy.

### Dashboard Scope

> **Note:** Dashboard implementation is Phase 2. The sections below describe the target design; implementation starts after dispatcher and MCP server are stable.

| Section | Purpose |
|---------|---------|
| System Health | Dispatcher status, agent containers, forge connectivity, model availability |
| Agent Monitor | Running agents, queue depth, current tasks, container resource usage |
| Cost Dashboard | Burn rate by provider, per-project spend, budget alerts, historical trends |
| Logs | Structured log viewer with filtering (agent, severity, task), real-time tail |
| Autonomy Config | Edit trust tiers, view pending Tier 3 actions, override history |
| Project Config | Forge connections, API keys (masked), model priority, polling intervals |
| Task Timeline | Task execution timeline, dependency graph visualization |
| User Profile | View/edit persistent preferences, conventions, standing decisions |

### Chat vs Dashboard Boundary

```text
CHAT (Claude + MCP)              DASHBOARD (web UI)
---------------------            ---------------------
"How's my project?"              System health metrics
"Approve this merge"             Agent container status
"Change priority on #42"         Cost graphs and trends
"What got done today?"           Log viewer and search
Give direction, make decisions   Edit autonomy config visually
                                 Troubleshoot failed agents
                                 API key management
```

## Agent Containers

Local agents run in Docker via Ollama. Each agent type gets its own container profile:

```text
samverk-agent-codegen   -> ollama + code generation model (e.g., deepseek-coder)
samverk-agent-test      -> ollama + reasoning model
samverk-agent-docs      -> ollama + general model
samverk-agent-qc        -> ollama + reasoning model (or cloud fallback)
```

Managed via `docker/docker/client` SDK. Container definitions stored as Go structs, not docker-compose -- more control over lifecycle, health checks, and timeout handling.

## State Persistence

| State | Where | Why |
|-------|-------|-----|
| Task state | Git issues | Human-readable, multi-device, core design principle |
| Agent session state | SQLite | Survives restarts, queryable, single-file backup |
| Cost tracking | SQLite | Per-task attribution, historical trends, budget queries |
| Audit log | Git issue comments + SQLite | Issues for visibility, SQLite for querying |
| User profile | YAML (`.samverk/profile.yaml`) | Version-controlled, portable |
| Autonomy config | YAML (`.samverk/autonomy.yaml`) | Already specified in [Autonomy Model](autonomy-model.md) |
| Server config | YAML (`.samverk/server.yaml`) | Already specified in [MCP Server](mcp-server.md) |
| Auth keys | YAML (`.samverk/auth.yaml`) | Never committed to git |

## CI/CD

- **GitHub Actions** for CI (lint, test, build, cross-compile verification)
- **GoReleaser** for cross-platform binary releases (Linux/Windows/Darwin, amd64/arm64)
- **Docker images** via GoReleaser for the server/dispatcher (deploy to Proxmox)
- **golangci-lint** for Go linting
- **markdownlint** for documentation quality
- **ESLint + TypeScript** for frontend linting

## HTTP Surfaces

The Go server serves three concerns on the same port:

| Path | Purpose | Consumer |
|------|---------|----------|
| `/mcp` | MCP Streamable HTTP (JSON-RPC) | Claude (any device) |
| `/api/v1/` | REST API | Dashboard SPA |
| `/` | Embedded SPA static files | Browser |

## Project Structure

```text
samverk/
├── cmd/samverk/               # Single binary entrypoint (cobra)
├── internal/
│   ├── server/                # HTTP server (MCP + API + embedded SPA)
│   ├── api/                   # REST API handlers for dashboard
│   ├── mcp/                   # MCP protocol handler (Streamable HTTP)
│   ├── dispatcher/            # Issue watcher, task router, dependency DAG
│   ├── forge/                 # IssueTracker interface + GitHub/Gitea impls
│   ├── agent/                 # Agent runtime, container management
│   ├── provider/              # AI provider clients (Claude, OpenAI, Ollama)
│   ├── autonomy/              # Trust tier evaluation engine
│   ├── profile/               # User profile management
│   ├── cost/                  # Token tracking, budget, attribution
│   └── store/                 # SQLite persistence layer
├── pkg/models/                # Shared types (Issue, Agent, Action, etc.)
├── web/                       # React SPA (Vite + TypeScript)
│   ├── src/
│   │   ├── components/        # Reusable UI components
│   │   ├── pages/             # Route pages (dashboard, logs, config, etc.)
│   │   ├── api/               # API client (TanStack Query hooks)
│   │   └── App.tsx
│   ├── package.json
│   └── vite.config.ts
├── docs/                      # Design docs and ADRs
├── .samverk/                  # Runtime config (gitignored)
├── .github/workflows/         # CI/CD pipelines
├── Makefile                   # Build and development tasks
└── .goreleaser.yaml           # Release configuration
```
