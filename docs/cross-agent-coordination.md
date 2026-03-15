# Cross-Agent Coordination Protocol

## Problem Statement

Samverk manages multiple independent projects (Samverk, DevKit, Synapset,
and future projects), each with its own repository and issue tracker on
potentially different forges (GitHub, Gitea). Today, coordination between
agents working in separate projects is entirely manual: the user creates
issues in target repos via `gh issue create` and relays context between
agent sessions. There is no machine-to-machine coordination path.

As the number of managed projects grows, manual relay becomes a
bottleneck. The user -- a hobbyist developer checking in every 1-2 days
-- should not need to be the message bus between their own agents.

## Design Goals

1. **Async-first** -- no synchronous RPC between agents (ADR-006)
2. **Issue-native** -- build on the existing issue-based communication
   protocol rather than introducing a new message transport (ADR-012)
3. **Forge-agnostic** -- work across GitHub and Gitea equally (ADR-013,
   ADR-031)
4. **Autonomy-aware** -- cross-project actions respect the three-tier
   autonomy model (ADR-015)
5. **Incrementally adoptable** -- start with the minimum viable protocol;
   add complexity only when proven necessary

## Survey of Existing Art

### CrewAI

CrewAI uses three coordination patterns: sequential (pipeline),
hierarchical (manager delegates to workers), and consensual (agents vote).
Agents share a conversation context within a "crew." Coordination is
in-process and synchronous -- agents run in the same Python process and
share memory. Not applicable to Samverk's distributed, async model where
agents are separate OS processes on different machines.

**Relevant takeaway:** The hierarchical pattern maps to Samverk's
dispatcher. The key difference is that CrewAI crews are ephemeral
in-process groups, while Samverk's agents are persistent, independently
deployed workers.

### AutoGen (Microsoft Agent Framework)

AutoGen v0.4 uses the actor model: each agent manages its own state and
communicates only through asynchronous messages. Agents communicate in
machine-readable JSON. Microsoft is converging AutoGen into the broader
Agent Framework with A2A support.

**Relevant takeaway:** The actor model aligns with Samverk's architecture.
Issue comments are effectively async messages. The constraint that agents
communicate only through messages (not shared memory) is already how
Samverk works.

### LangGraph

LangGraph uses a graph-based state machine. All agents read and write to
a shared state object, which acts as the coordination medium. State
persists across the workflow. This is centralized coordination -- a single
state store that all nodes access.

**Relevant takeaway:** Samverk's issue tracker already serves as a form
of shared state. The graph-based dependency model in the dispatcher
(topological sort, critical path) is analogous to LangGraph's directed
graph approach. The key difference is that LangGraph's state is in-memory
or a single database, while Samverk's state is distributed across forge
APIs.

### OpenAI Swarm / Agents SDK

Swarm is built on two primitives: agents (instructions + tools) and
handoffs (explicit control transfer returning another agent). Handoffs
carry all context the next agent needs -- there is no hidden state. Swarm
is now superseded by the OpenAI Agents SDK for production use.

**Relevant takeaway:** The handoff pattern is directly applicable.
Samverk's cross-project coordination is fundamentally a handoff: Agent A
in Project X creates work for Agent B in Project Y, passing all necessary
context in the issue body. No hidden state.

### Google Agent-to-Agent (A2A) Protocol

A2A is an open protocol (now under the Linux Foundation) enabling
communication between opaque agent systems. Core concepts: Agent Cards
(JSON capability discovery), Tasks (with lifecycle states), and Messages
(context sharing). Supports multiple modalities and transports (HTTP,
gRPC). Adopted by Google, Microsoft, Amazon, and 50+ partners.

**Relevant takeaway:** Agent Cards for capability discovery is a strong
pattern. Samverk's project registry (`ProjectRegistry`) already stores
per-project metadata -- extending it with agent capability information
(available agent types, forge endpoints) would enable discovery without
a new protocol. The Task lifecycle model (submitted, working, completed,
failed) maps directly to Samverk's status labels.

### Temporal / Cadence

Temporal uses centralized workflow orchestration with durable state.
Workflows define the coordination logic; Activities perform the work.
The orchestrator manages retries, timeouts, and saga-pattern rollbacks
across distributed services. Workflows are language-agnostic.

**Relevant takeaway:** Temporal's model is heavyweight for Samverk's
needs but validates the pattern of a central coordinator (dispatcher)
managing distributed workers. The saga pattern (compensating actions on
failure) is relevant for cross-project work that partially completes.

## Protocol Design

### Addressing: Cross-Project Issue References

Agents address each other through **cross-project issue references**
using the format `owner/repo#number`. This extends the existing
`depends_on` field in the issue frontmatter.

Current schema (single-project):

```yaml
depends_on: [121, 122]
```

Extended schema (cross-project):

```yaml
depends_on: [121, "HerbHall/devkit#45"]
```

The `depends_on` field accepts both integers (same-project) and strings
(cross-project references). The string format follows GitHub's native
cross-repo reference syntax, which Gitea also supports.

### Schema Changes

The communication protocol schema (v1.1.0) adds one new optional field
and extends `depends_on` to accept cross-project references:

| Field | Type | Required | Description |
| ----- | ---- | -------- | ----------- |
| `source_project` | string | No | `owner/repo` of the originating project |
| `depends_on` | (int or string)[] | No | Issue numbers or `owner/repo#number` refs |

The `source_project` field enables the receiving dispatcher to route
completion notifications back to the originating project.

### Coordination Issue Type

A new issue type `coordination` is added to the existing type enum.
Coordination issues are created in the **target** project's tracker and
carry full context from the source project:

```yaml
---
schema_version: "1.1.0"
type: coordination
agent_type: code-gen
priority: normal
source_project: HerbHall/samverk
depends_on: ["HerbHall/samverk#400"]
estimated_tokens: 2000
---

## Summary

Update DevKit CI templates to support golangci-lint v2.10 schema.

## Context

Samverk issue HerbHall/samverk#400 identified that the DevKit CI
templates use outdated golangci-lint config. This coordination issue
requests the update in DevKit's own repo.

## Acceptance Criteria

- [ ] golangci-lint config template updated to v2.10 schema
- [ ] CI workflow template updated to use golangci-lint-action v7

## Handoff Context

- Source issue: HerbHall/samverk#400
- Decision context: See ADR-034 for coordination protocol
- Files of interest: project-templates/golangci.yml
```

### Coordination Flow

```text
Project A (Samverk)                    Project B (DevKit)
────────────────────                   ────────────────────

1. Agent identifies cross-project
   dependency during task execution

2. Dispatcher checks autonomy tier:
   - Tier 1 (auto): create issue
   - Tier 2 (notify): create issue,
     notify user
   - Tier 3 (approve): request user
     approval first

3. Dispatcher creates coordination    4. Dispatcher B receives new issue
   issue in Project B via forge API       event (webhook or poll)

5. Source issue in Project A gets      6. Dispatcher B classifies, checks
   status:blocked with cross-ref          deps, routes to agent pool

                                       7. Agent B executes task

                                       8. Agent B completes, issue closed

9. Dispatcher A polls/watches          10. (optional) Completion comment
   Project B for closure                   posted on source issue in A

11. Source issue in Project A
    unblocked, resumes routing
```

## Discovery

### Project Registry as Discovery Service

The existing `ProjectRegistry` (in `internal/mcp/projects.go`) serves as
the discovery mechanism. Each registered project already carries:

- Name, owner, repo
- Forge type (github/gitea)
- Forge connection (IssueTracker, RepoReader, PullRequestManager)

To support cross-project coordination, the registry is extended with
an **agent capabilities** field per project:

```yaml
# server.yaml projects section
projects:
  - name: samverk
    owner: HerbHall
    repo: samverk
    forge: github
    agent_types: [code-gen, test, research, qc, infra, dispatcher]

  - name: devkit
    owner: HerbHall
    repo: devkit
    forge: github
    agent_types: [code-gen, docs, research]

  - name: synapset
    owner: HerbHall
    repo: synapset
    forge: github
    agent_types: [code-gen, test, research]
```

When an agent in Project A needs work done in Project B, the dispatcher
looks up Project B in the registry to confirm:

1. The project is registered and reachable
2. The required agent type is available in that project
3. The forge connection is healthy (circuit breaker state)

No additional discovery protocol is needed. The registry is the single
source of truth for "what projects exist and what can they do."

### Agent Cards (Future Extension)

If Samverk later integrates with external agent systems (not managed by
this Samverk instance), the A2A Agent Card pattern could be adopted: each
project publishes a `/.well-known/agent.json` describing its capabilities.
This is not needed for the initial implementation where all projects are
registered in the same Samverk instance.

## Conflict Prevention

### File-Level Conflict Avoidance

Cross-project coordination inherently avoids file conflicts because agents
work in separate repositories. The risk surface is:

1. **Same repo, different agents** -- already handled by the dispatcher's
   optimistic locking (comment-based claiming with 10s window)
2. **Cross-repo shared files** -- not applicable; repos are independent
3. **Cross-repo shared dependencies** -- e.g., both projects depend on
   the same Go module. Handled by standard dependency management (go.mod),
   not the coordination protocol

### Issue-Level Conflict Avoidance

The dispatcher prevents double-routing through:

- Optimistic locking on issue claiming (existing mechanism)
- The `source_project` field prevents circular coordination (a
  coordination issue cannot create a coordination issue back to its
  source for the same work item)
- Cross-project `depends_on` references are validated: the dispatcher
  confirms the referenced issue exists before creating the dependency

### Race Condition: Concurrent Cross-Project Issue Creation

Two dispatchers could theoretically create duplicate coordination issues
in the same target project. Mitigation:

- Each coordination issue includes a `source_project` and source issue
  number in its frontmatter
- The target dispatcher deduplicates on `(source_project, source_issue)`
  before routing
- If duplicates slip through, the QC agent catches them during validation

## Handoff Protocol

A structured handoff between projects follows this contract:

### Handoff Issue Contract

The source agent creates an issue in the target project with:

1. **Type**: `coordination`
2. **Source project**: `owner/repo` of the originator
3. **Source issue**: the issue number that triggered the handoff
4. **Full context**: everything the target agent needs to execute without
   accessing the source project's repo. No hidden state.
5. **Acceptance criteria**: testable conditions, same as any task issue
6. **Autonomy tier**: inherited from the source issue or overridden by
   the coordination policy

### Completion Notification

When the target issue is closed with `status:done`:

1. The source project's dispatcher detects the closure (via polling or
   webhook on the target project's tracker)
2. A comment is posted on the source issue:
   `COORDINATION_COMPLETE [dispatcher] [timestamp] Target issue owner/repo#N closed.`
3. If the source issue was `status:blocked` on this dependency, it
   transitions to `status:queued`

### Failure Handling

If the target issue fails (3x retry exhaustion or escalation):

1. The target dispatcher adds `status:needs-human` per the standard
   failure recovery protocol (ADR-027)
2. The source issue remains `status:blocked`
3. The user sees both the blocked source issue and the failed target
   issue in their next check-in digest

No automatic retry across project boundaries -- failures that exhaust
the target project's retry budget escalate to the user.

## Scope Boundaries

### What Crosses the Coordination Boundary

| Action | Crosses Boundary | Mechanism |
| ------ | ---------------- | --------- |
| Create task in another project | Yes | Coordination issue via forge API |
| Read issue status from another project | Yes | Forge API query via registry |
| Modify code in another project | No | Not allowed; target agent does its own work |
| Read files from another project | No | Not directly; context is passed in issue body |
| Share token budget | Yes | Coordination layer (SQLite) tracks cross-project spend |
| Priority decisions | Yes | Coordination layer ranks across projects |
| Dependency blocking/unblocking | Yes | Cross-project `depends_on` references |

### What Stays Internal

- Code changes, branch management, PR creation
- Agent assignment and pool management
- Heartbeat monitoring and timeout detection
- QC validation of work output
- Local dependency graphs (intra-project)

The principle: **issues cross boundaries, code does not.** An agent in
Project A never directly modifies files in Project B. It creates a
coordination issue, and Project B's own agents do the work.

## Implementation Plan

### Phase 1: Cross-Project Dependency Tracking (MVP)

**Goal:** The dispatcher can resolve `depends_on` references that point
to issues in other registered projects.

Changes:

- Extend `models.IssueFrontmatter.DependsOn` from `[]int` to a union
  type that accepts both `int` and `string` (cross-project ref)
- Add `ParseCrossRef(ref string) (owner, repo string, number int, err error)`
  to parse `owner/repo#number` format
- Modify `dispatcher.checkDependencies()` to resolve cross-project refs
  by looking up the target project in the registry and querying its
  forge tracker
- Modify `dispatcher.unblockDependents()` to watch for closures in
  other registered projects
- Add `source_project` field to `IssueFrontmatter`

### Phase 2: Coordination Issue Creation

**Goal:** The dispatcher can create coordination issues in target
projects when an agent identifies cross-project work.

Changes:

- Add `coordination` to the issue type enum
- Add MCP tool `create_coordination_issue(target_project, issue_body)`
- Add autonomy check: cross-project issue creation requires at least
  Tier 2 (notify user) by default
- Add deduplication check on `(source_project, source_issue)` in the
  target dispatcher

### Phase 3: Cross-Project Digest

**Goal:** The check-in digest aggregates status across all registered
projects, including coordination issues.

Changes:

- Extend digest builder to query all registered project trackers
- Group coordination issues by source/target relationship
- Show cross-project dependency chains in the digest view

### Phase 4: Agent Card Discovery (Future)

**Goal:** Support discovery of agent capabilities in projects not
registered in the local Samverk instance.

This phase is deferred until there is a concrete need for inter-instance
coordination (multiple Samverk instances managing different project
sets).

## Rejected Alternatives

### Shared Event Bus (Redis, NATS, RabbitMQ)

Introducing a message broker adds infrastructure complexity incompatible
with the self-hosted hobbyist target. The forge issue tracker already
provides durable, ordered, observable message storage. A message broker
would duplicate this capability without adding meaningful value.

### MCP Tool-to-Tool Calls

Direct MCP calls between Samverk instances would create synchronous
coupling. If instance B is down, instance A blocks. This violates the
async-first principle and creates availability dependencies that the
issue-based approach avoids.

### Shared Database

A single SQLite database shared across projects would couple their
deployment. Projects on different machines or forges could not
participate. The forge API is the natural shared medium -- it is already
accessible from anywhere.

### Full A2A Protocol Implementation

Google's A2A protocol is designed for heterogeneous agent ecosystems
with different vendors, frameworks, and languages. Samverk manages a
homogeneous set of projects under a single operator. The overhead of
Agent Cards, capability negotiation, and multi-modal message formats
is not justified. However, the Agent Card concept is noted as a future
extension point if Samverk ever coordinates with external systems.

## Related Documents

- [Communication Protocol](communication-protocol.md)
- [ADR-012: Git Issues as Communication Protocol](decisions/ADR-012-git-issues-protocol.md)
- [ADR-023: Per-Project Repos with Coordination Layer](decisions/ADR-023-per-project-repos.md)
- [ADR-027: Failure Recovery](decisions/ADR-027-failure-recovery.md)
- [ADR-031: Dual-Forge Operational Model](decisions/ADR-031-dual-forge-operational-model.md)
- [ADR-034: Cross-Agent Coordination](decisions/ADR-034-cross-agent-coordination.md)
- [Architecture](architecture.md)
- [Autonomy Model](autonomy-model.md)
