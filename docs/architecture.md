# Architecture

## The Simplest Form

> **Samverk is a conversational project manager backed by a working agent team.**

The chat IS the primary interface. A conversation on any device that can run Claude. A web dashboard complements it for operational concerns -- configuration, monitoring, logs, and troubleshooting ([ADR-020](decisions/ADR-020-web-dashboard.md)).

## Two Core Components

### 1. The Front-End Agent (the conversation)

- Lives in Claude (or any capable chat model)
- Has access to the project repo and issue tracker via MCP
- When the user asks "how's my project doing?" it:
  - Pulls current issue state
  - Summarizes what's in progress, blocked, and needs user input
  - Presents it conversationally, prioritized
- User discusses, decides, gives direction in natural language
- Front-end agent translates decisions into issues and assignments
- User closes chat and goes back to their life
- Works from any device that can run Claude -- phone, tablet, laptop, desktop

### 2. The Back-End Agent Team (the workers)

- Watches the issue tracker continuously
- Picks up assigned work
- Runs locally (containerized) or via cloud API depending on task complexity
- Reports results back as issue comments
- Flags blocks by labeling issues `needs-human`
- Works whether the user is in the chat or not

### The Bridge

The front-end chat agent speaks human on one side and speaks git issues on the other. That's it. That's the whole product.

## What Samverk Actually Builds

The scope is tighter than originally described because we're building on existing infrastructure:

| What's needed | What provides it |
|---------------|-----------------|
| Chat interface | Claude (already exists) |
| Project storage | Git (already exists) |
| Task system | GitHub/Gitea issues (already exists) |
| **MCP server -- repo + issue access** | **Samverk builds this** |
| **Web dashboard -- ops, config, monitoring** | **Samverk builds this** |
| **Dispatcher agent** | **Samverk builds this** |
| **Specialist execution agents** | **Samverk builds this** |
| **Issue schema + conventions** | **Samverk defines this** |

## Hybrid Local/Cloud Model

Task complexity determines where work runs:

```text
Cloud (paid API)              Orchestration layer, complex reasoning,
e.g. Claude, GPT-4           architectural decisions, QC arbitration,
                              resolving ambiguity

Mid-tier model                Division/Department layer
(API or large local)          task decomposition, planning

Local agents                  Agent layer, narrow execution tasks:
(containerized,               code generation, formatting, testing,
GPU-accelerated)              schema validation, boilerplate, docs
```

### Why Containers for Local Agents

- Each agent type runs in its own container with appropriate model pre-loaded
- Containers scale horizontally -- run multiple agents in parallel
- Clean resource boundaries
- Reproducible environments
- Ollama runs cleanly inside Docker -- tooling exists today

### Container Spin-up Latency Is Not a Problem

Cold start latency (30-90 seconds to spin up a container and load a model) is NOT a user experience problem for this audience. The user is not watching it work. Do not optimize for cold start performance -- optimize for throughput and quality instead.

## The Agent Hierarchy

```text
                    USER (chat)
                        |
              FRONT-END AGENT (Claude + MCP)
              Conversational interface
              Translates human <-> git issues
                        |
                  DISPATCHER AGENT
              Always-running, watches issue tracker
              Routes work, checks dependencies
                        |
    ┌─────────┬─────────┼─────────┬─────────┐
    |         |         |         |         |
IDEATION  FEASIBILITY  CODE-GEN  TEST     DOCS
(cloud)   (cloud)      (local)   (local)  (local)
    |         |         |         |         |
 RESEARCH   LEGAL    RESEARCH    QC        QC
 (cloud)   (cloud)   (cloud)
    |                   |
    QC                  QC
```

Pre-project agents (ideation, feasibility, legal) operate in Phases 1-5. Execution agents (code-gen, test, docs) operate in Phase 6. Research spans both — it supports feasibility analysis before a project exists and technical investigation during execution. See [Project Lifecycle](project-lifecycle.md) for the full seven-phase model.

## Web Dashboard

The chat handles project progress and decisions. The web dashboard handles infrastructure and operations. See [ADR-020](decisions/ADR-020-web-dashboard.md) for the decision rationale.

The Go server serves three HTTP surfaces on the same port:

| Path | Purpose | Consumer |
| ---- | ------- | -------- |
| `/mcp` | MCP Streamable HTTP (JSON-RPC) | Claude (any device) |
| `/api/v1/` | REST API | Dashboard SPA |
| `/` | Embedded SPA static files | Browser |

The React SPA is embedded in the Go binary via `embed.FS`. Single binary deployment is preserved -- no separate web server, no CORS, no reverse proxy.

Dashboard scope: system health, agent monitoring, cost dashboards, structured log viewer, autonomy configuration, project settings, task timeline, and user profile management. See [Tech Stack](tech-stack.md) for full dashboard section details.

## Authentication

### BearerAuth Middleware

All API (`/api/`) and MCP (`/mcp`) routes are wrapped with `BearerAuth` middleware. Supports two modes:

- **Simple token**: Single `SAMVERK_AUTH_TOKEN` environment variable
- **Key store**: YAML file with named keys, optional scope and worker identity

Unauthenticated requests return 401. Health check (`/healthz`) and static SPA files (`/`) are exempt.

### Per-Worker Identity

Workers register with scoped API keys (`scope: worker`, `worker_id: <name>`). The `KeyStore` validates scope and worker ID on registration and heartbeat endpoints. This enables per-worker cost attribution and audit trails.

### Dashboard Token Injection

The SPA handler intercepts `index.html` to inject `window.__SAMVERK_TOKEN__` at serve time via a `<script>` tag. The React app reads this value and adds `Authorization: Bearer` headers to all API requests. This avoids exposing the token in client-side configuration files.

## The QC Mirror

Every specialist agent has a parallel QC check:

```text
Agent completes work
    --> Comments result on issue
    --> Adds status:needs-qc label

QC Agent picks up
    --> Validates against acceptance criteria
    --> Pass: closes issue, parent notified
    --> Fail: reopens, comments failures, re-queues
    --> 3x failure: adds status:needs-human, user notified at next check-in
```

## The Dispatcher

The dispatcher is the always-running process that watches the issue tracker:

1. New issue appears with `status:queued` -- dispatcher wakes (webhook or poll)
2. Evaluates: complexity, agent type needed, dependencies met?
3. Dependencies not met -- add `status:blocked`, comment with blocking issue numbers
4. Ready -- assign to appropriate agent pool, change to `status:claimed`
5. Monitor for completion or timeout
6. On timeout -- reassign or escalate

The dispatcher does no execution work. Its only job is routing.

## Multi-Model Failover

Samverk is model-agnostic. Provider failover on credit exhaustion is a core feature:

- If Claude API credits run out, fall over to GPT-4 or Gemini
- If all cloud credits are exhausted, fall back to local models
- Resume cloud when credits reset
- User configures priority order and API keys

This serves double duty: **cost management** (never blocked by a single provider's billing) and **quality diversity** (different models have different blind spots -- rotating providers improves overall output quality).

## Platform Abstraction

All issue tracker operations go through a platform-agnostic interface:

```go
type IssueTracker interface {
    CreateIssue(issue Issue) (int, error)
    UpdateIssue(id int, update IssueUpdate) error
    AddComment(id int, comment string) error
    ListIssues(filter IssueFilter) ([]Issue, error)
    SetLabels(id int, labels []string) error
    Assign(id int, agent string) error
    Watch(handler func(Event)) error
}
```

Implementations: GitHubTracker, GiteaTracker, GitLabTracker.

Gitea on self-hosted server = no API rate limits, full control. GitHub = easiest to start with, most familiar.

## External Contractors

Certain specialized tasks call for external APIs rather than in-house agents:

- Legal database lookups
- Trademark searches
- Specialized ML models
- Domain-specific APIs

Treated as "external contractors" -- called when needed, billed per use, not part of the permanent org chart.

## Action Trust Tiers

Agent autonomy is governed by a three-tier trust model. This determines which actions agents take immediately vs. which require user confirmation at the next check-in.

| Tier | Behavior | Examples |
|------|----------|----------|
| **Tier 1** | Always autonomous, logged for audit | Read files, search, create branches, commit to feature branches |
| **Tier 2** | Autonomous, surfaced in check-in digest | Edit files, close issues, push to non-main branches |
| **Tier 3** | Queued as `needs-human`, unblocked work continues | Merge to main, delete files, force push, over-threshold API calls |

A Tier 3 block never stops the whole system. The agent creates a `needs-human` issue, marks dependent work as blocked, and continues all independent work streams. The user addresses it at their next check-in.

Tiers are configurable per project via `.samverk/autonomy.yaml`. See [Autonomy Model](autonomy-model.md) for full specification.

## User Profile

Agents consult a persistent user profile that captures preferences, conventions, and standing decisions across all projects. This prevents agents from re-asking resolved questions and ensures consistency without manual repetition.

The profile covers project conventions (directory structure, git workflow), technical preferences (languages, frameworks, CI/CD), AI agent configuration (trust tiers, model routing, cost thresholds), and standing decisions (license, hosting, security).

The profile can be bootstrapped from an existing Devkit-style repo, repo analysis, or an onboarding conversation. See [User Profile](user-profile.md) for full specification.

## Implementation Stack

- **Language:** Go ([ADR-005](decisions/ADR-005-go-language.md))
- **Primary platform:** Windows (developer's primary environment)
- **Deployment:** Single binary with subcommands (`samverk serve`, `samverk dispatch`, `samverk config`)
- **AI Providers:** Anthropic Claude API (primary), OpenAI/GPT-4, Gemini, Ollama (local)
- **Local models:** Ollama in Docker containers, per-agent-type profiles
- **Web dashboard:** React + TypeScript SPA, embedded via `embed.FS` ([ADR-020](decisions/ADR-020-web-dashboard.md))
- **Git forge:** GitHub (primary), Gitea (self-hosted option), abstracted behind `IssueTracker` interface
- **State persistence:** Git issues (task state) + SQLite (sessions, cost, audit) + YAML (config)
- **Agent communication:** Git issues (see [Communication Protocol](communication-protocol.md))
- **Orchestration:** Custom -- not built on LangChain/LangGraph/CrewAI ([ADR-004](decisions/ADR-004-custom-orchestration.md))
- **CI/CD:** GitHub Actions + GoReleaser

For the full technology stack including specific libraries and project structure, see [Tech Stack](tech-stack.md).
