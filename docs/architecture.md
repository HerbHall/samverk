# Architecture

## The Simplest Form

> **Samverk is a conversational project manager backed by a working agent team.**

The chat IS the interface. Not a dashboard, not a web portal, not a special app. A conversation. On any device that can run Claude.

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
          +---------+---------+---------+
          |         |         |         |
      CODE-GEN   TEST      DOCS     RESEARCH
      (local)    (local)   (local)   (cloud)
          |         |         |         |
          QC        QC        QC        QC
      (validates each agent's output)
```

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

## Implementation Stack

- **Language:** Go (consistent with Subnetree project)
- **Primary platform:** Windows (developer's primary environment)
- **AI Providers:** Anthropic Claude API (primary), multi-provider failover
- **Local models:** Ollama in Docker containers
- **Git forge:** GitHub (primary), Gitea (self-hosted option)
- **Agent communication:** Git issues (see [Communication Protocol](communication-protocol.md))
- **Orchestration:** Custom -- not built on LangChain/LangGraph/CrewAI
