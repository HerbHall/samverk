# Samverk

Async background development engine -- keeps your project building while you live your life.

## Quick Start

```bash
make build     # Build binary to bin/samverk
make test      # Run all tests
make lint      # Run golangci-lint
make lint-md   # Run markdownlint
make ci        # Run build + test + lint (full CI locally)
make hooks     # Install pre-push git hook
make run       # Build and start samverk serve
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
│   ├── digest/                # Check-in digest builder and formatting
│   ├── forge/                 # IssueTracker interface + GitHub/Gitea impls
│   ├── agent/                 # Agent runtime, container management
│   ├── provider/              # AI provider clients (Claude, OpenAI, Ollama)
│   ├── autonomy/              # Trust tier evaluation engine
│   ├── profile/               # User profile management
│   ├── cost/                  # Token tracking, budget, attribution
│   ├── store/                 # SQLite persistence layer
│   ├── hostmetrics/           # Host resource collection (disk, RAM, CPU)
│   ├── logstore/              # Structured log persistence to SQLite
│   ├── loganalyst/            # AI log summarization via Ollama
│   ├── logging/               # Structured logging (zap tee)
│   ├── metrics/               # Dispatcher and pool metrics
│   ├── scaling/               # Adaptive worker scaling (1-N)
│   ├── synapset/              # Synapset MCP client for memory enrichment
│   ├── prwatcher/             # PR watcher for auto-merge
│   ├── status/                # Health and status subsystem
│   └── version/               # Build version injection (ldflags)
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
│   └── decisions/             # Architecture Decision Records (31 total)
├── .samverk/                  # Project state (status.md) and runtime config
├── .github/workflows/         # GitHub CI/CD pipelines
├── .gitea/workflows/          # Gitea Actions CI (gitea-ci.yml, security.yml)
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
- **CI/CD:** GitHub Actions (primary) + Gitea Actions (mirror CI, security.yml)
- **Linting:** golangci-lint (Go), ESLint + TypeScript (frontend), markdownlint (docs)

For full details including specific libraries, what NOT to use, and project structure rationale, see [docs/tech-stack.md](docs/tech-stack.md).

## Session Start (Required)

Before any work in a new session:

1. Read `.samverk/status.md` -- current phase, in-flight issues, last session summary
2. Call `samverk get_digest --since 168h` if Samverk MCP is configured and available
3. Check open issues if the task involves issue triage or project direction
4. Proceed without asking the user to explain project state -- the repo has it

**Rule**: Never ask the user "what's the current state?" or "where did we leave off?"
Read the repo first. Ask only if something is genuinely ambiguous after reading.

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
- **Two-location rule (ADR-039):** If a value, format, or behavioral contract
  appears in 2+ locations, extract it to a single authoritative source. Before
  defining a constant or format string, search for an existing definition. Key
  sources of truth: `internal/agent/format.go` (output format), `pkg/models/`
  (labels, types), `overlay/labels.json` (label definitions).
- Validate at system boundaries, not internal calls

## Known Constraints

- Infrastructure complete -- all phases through 6 done
- Current focus: agent autonomy (prompt quality, planning workflow)
- Free-first routing: 3 Ollama GPU hosts tried before Claude API
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
- Failure recovery strategy ([ADR-027](docs/decisions/ADR-027-failure-recovery.md))
- Cross-model QA validation ([ADR-030](docs/decisions/ADR-030-cross-model-qa.md))
- Dual-forge operational model ([ADR-031](docs/decisions/ADR-031-dual-forge-operational-model.md))
- Adaptive worker scaling ([ADR-032](docs/decisions/ADR-032-adaptive-worker-scaling.md))
- Multi-machine free agent runtime ([ADR-033](docs/decisions/ADR-033-multi-machine-free-agent-runtime.md))
- Solo developer agent model ([ADR-035](docs/decisions/ADR-035-solo-developer-agent-model.md))
- Two-location centralization rule ([ADR-039](docs/decisions/ADR-039-two-location-centralization-rule.md))
- Unified execution plan -- Q2 2026 ([docs/unified-execution-plan.md](docs/unified-execution-plan.md))

## Infrastructure

- **Proxmox host:** `root@192.168.1.203` (SSH key auth configured)
- **Samverk container:** CT 202 `root@192.168.1.162:8080` (SSH key auth)
- **Staging container:** CT 203 `root@192.168.1.199:8080` (STOPPED -- decommissioned 2026-03-17, spin up if needed)
- **Gitea instance:** CT 200 `192.168.1.160:3000` (`gitea.herbhall.net`) -- 40GB disk
- **Ollama VM 300:** `192.168.1.207:11434` (RTX 3090 Ti, qwen2.5-coder:14b)
- **Ollama HDH-NZXT:** `192.168.1.202:11434` (RTX 5090, qwen3-coder:30b)
- **Ollama CM-ASUS:** `100.88.37.47:11434` via Tailscale (RTX 2080 Ti, qwen2.5-coder:7b)
- **Health check:** `curl http://192.168.1.162:8080/healthz`
- **Deploy:** `make redeploy` -- cross-compiles, deploys, restarts, verifies health
- **Model check:** `bash scripts/deploy-models.sh --check` (run from CT 202)
- Claude Code has SSH access to all hosts

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
- [Samverk Overlay Spec](overlay/README.md)
- [DevKit Boundary Contract](https://github.com/HerbHall/devkit/blob/main/docs/samverk-boundary.md)
- [Unified Execution Plan](docs/unified-execution-plan.md)
- [Gitea Migration Plan](docs/gitea-migration-plan.md)
- [Adaptive Scaling Plan](docs/adaptive-scaling-plan.md)
