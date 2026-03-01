# Samverk

Async background development engine -- keeps your project building while you live your life.

## Quick Start

```bash
# No build steps yet -- concept phase
# Future:
make install   # Install dependencies
make build     # Build the project
make test      # Run tests
make lint      # Run linters
make run       # Start the system
```

## Project Structure

```text
samverk/
├── cmd/samverk/               # Single binary entrypoint (cobra subcommands)
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
├── web/                       # React SPA dashboard (Vite + TypeScript)
├── docs/                      # Design docs and ADRs
│   ├── architecture.md        # System design and components
│   ├── tech-stack.md          # Full technology choices and libraries
│   ├── communication-protocol.md  # Issue schema, labels, dispatcher, QC flow
│   ├── concept.md             # Problem space, target user, value proposition
│   ├── cost-model.md          # Tiered cost model and comparisons
│   ├── mcp-server.md          # MCP server requirements and tool definitions
│   ├── user-interface.md      # Check-in model and device flexibility spec
│   ├── autonomy-model.md      # Three-tier trust model for agent actions
│   ├── user-profile.md        # Persistent user preferences across projects
│   ├── open-questions.md      # Unresolved design and business questions
│   └── decisions/             # Architecture Decision Records (20 so far)
├── .samverk/                  # Runtime config (gitignored)
├── .github/workflows/         # CI/CD pipelines
└── Makefile                   # Build and development tasks
```

## Tech Stack

- **Language:** Go -- single binary with subcommands (`samverk serve`, `samverk dispatch`, `samverk config`)
- **AI Providers:** Anthropic Claude API (primary), OpenAI/GPT-4, Gemini, Ollama (local containers)
- **Web Dashboard:** React + TypeScript + Vite, embedded in Go binary via `embed.FS`
- **Frontend:** shadcn/ui, Tailwind CSS, TanStack Query, Zustand, Recharts
- **State:** Git issues (tasks) + SQLite (sessions, cost, audit) + YAML (config)
- **Git Forge:** GitHub (primary), Gitea (self-hosted), abstracted behind `IssueTracker` interface
- **Testing:** Go stdlib + table-driven tests, Vitest + Testing Library (frontend)
- **Build:** Make + GoReleaser
- **CI/CD:** GitHub Actions
- **Linting:** golangci-lint (Go), ESLint + TypeScript (frontend), markdownlint (docs)

For full details including specific libraries, what NOT to use, and project structure rationale, see [docs/tech-stack.md](docs/tech-stack.md).

## Development Workflow

- Branch-per-issue: `feature/issue-NNN-desc`, `fix/issue-NNN-desc`
- Conventional commits: `feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`
- Co-author tag: `Co-Authored-By: Claude <noreply@anthropic.com>`
- Never commit directly to `main` -- always PR
- Explore -> Plan -> Code -> Commit

## Code Style

- Follow language-specific idioms (Go: gofmt, Python: ruff)
- No over-engineering -- implement only what's needed now
- Comments only where logic isn't self-evident
- Validate at system boundaries, not internal calls

## Known Constraints

- Phase 1 implementation complete -- dispatcher, forge, store, autonomy, profile, provider all functional
- Async-first architecture (not synchronous tooling)
- Hybrid local/cloud agent model
- Target audience: hobbyist devs with limited time, not enterprise teams
- Success metric: project completion rate, not response speed

## Key Decisions

- Async-first check-in model ([ADR-006](docs/decisions/ADR-006-async-first.md))
- Hybrid local/cloud agents ([ADR-007](docs/decisions/ADR-007-hybrid-local-cloud.md))
- Multi-model from day one ([ADR-008](docs/decisions/ADR-008-multi-model-default.md))
- Device flexibility non-negotiable ([ADR-009](docs/decisions/ADR-009-device-flexibility.md))
- Ship rate is the success metric ([ADR-010](docs/decisions/ADR-010-success-metric.md))
- Chat (Claude + MCP) is the interface ([ADR-011](docs/decisions/ADR-011-chat-as-interface.md))
- Git issues as agent communication ([ADR-012](docs/decisions/ADR-012-git-issues-protocol.md))
- Git forge abstracted behind interface ([ADR-013](docs/decisions/ADR-013-forge-abstraction.md))
- Dedicated dispatcher agent for routing ([ADR-014](docs/decisions/ADR-014-dispatcher-agent.md))
- Three-tier autonomy model ([ADR-015](docs/decisions/ADR-015-three-tier-autonomy.md))
- User profile as first-class concept ([ADR-016](docs/decisions/ADR-016-user-profile.md))
- Devkit as reference implementation ([ADR-017](docs/decisions/ADR-017-devkit-reference.md))
- Three-phase release: alpha, beta, v1.0 ([ADR-018](docs/decisions/ADR-018-release-versioning.md))
- Self-hosted-first development ([ADR-019](docs/decisions/ADR-019-self-hosted-first.md))
- Web dashboard for operations ([ADR-020](docs/decisions/ADR-020-web-dashboard.md))
- Intent verification protocol ([ADR-021](docs/decisions/ADR-021-intent-verification.md))
- Full project lifecycle -- idea to delivery ([ADR-022](docs/decisions/ADR-022-full-project-lifecycle.md))
- Per-project repos with coordination layer ([ADR-023](docs/decisions/ADR-023-per-project-repos.md))

## References

- [Architecture](docs/architecture.md)
- [Tech Stack](docs/tech-stack.md)
- [Communication Protocol](docs/communication-protocol.md)
- [Concept](docs/concept.md)
- [Competitive Landscape](docs/competitive.md)
- [Cost Model](docs/cost-model.md)
- [MCP Server](docs/mcp-server.md)
- [User Interface](docs/user-interface.md)
- [Open Questions](docs/open-questions.md)
- [Naming](docs/naming.md)
- [Autonomy Model](docs/autonomy-model.md)
- [User Profile](docs/user-profile.md)
- [Intent Verification Protocol](docs/intent-verification.md)
- [Project Lifecycle](docs/project-lifecycle.md)
- [Multi-Session Safety](docs/multi-session-safety.md)
- [Decision Records](docs/decisions/)
