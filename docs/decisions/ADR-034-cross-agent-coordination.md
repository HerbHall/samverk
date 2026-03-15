# ADR-034: Cross-Agent Coordination Protocol

**Status:** Proposed
**Date:** 2026-03-14

## Context

Samverk manages multiple independent projects, each with its own
repository and issue tracker (ADR-023). The dispatcher already handles
intra-project dependencies: topological sort, cycle detection, critical
path analysis, and automatic unblocking when dependencies close. However,
all of this operates within a single project's issue space.

Cross-project coordination is currently manual. When an agent working on
Samverk identifies a needed change in DevKit, the user must:

1. Manually create an issue in the DevKit repo
2. Relay context from the Samverk session to the DevKit session
3. Monitor the DevKit issue for completion
4. Manually unblock the Samverk issue

This manual relay is the exact failure mode Samverk exists to eliminate.
The target user -- a hobbyist developer checking in every 1-2 days --
cannot be the message bus between their own agents.

### Options Considered

**Option A: Shared Event Bus**

Introduce a message broker (Redis Pub/Sub, NATS, or RabbitMQ) as the
cross-project communication channel. Each dispatcher publishes events;
others subscribe.

Pros:

- Real-time event delivery
- Decoupled producers and consumers
- Battle-tested infrastructure

Cons:

- Adds infrastructure dependency (contradicts self-hosted simplicity)
- Duplicates the forge's existing durable message storage (issues)
- No audit trail visible to the user without a separate UI
- Requires the message broker to be always-on alongside the dispatcher

**Option B: MCP Tool-to-Tool Calls**

Samverk instances expose MCP tools that other instances can call directly.
Cross-project coordination becomes synchronous tool invocation.

Pros:

- Low latency for simple queries
- Reuses existing MCP infrastructure

Cons:

- Creates synchronous coupling (if instance B is down, instance A blocks)
- Violates async-first principle (ADR-006)
- Every instance must be reachable from every other instance
- Not compatible with the user's current setup of separate CC sessions

**Option C: Cross-Project Issue References (Selected)**

Extend the existing issue-based communication protocol to support
cross-project references. The `depends_on` field accepts
`owner/repo#number` strings alongside local issue numbers. A new
`coordination` issue type enables structured handoffs between projects.
The existing `ProjectRegistry` serves as the discovery mechanism.

Pros:

- Builds on proven infrastructure (forge APIs, issue tracker)
- Async by nature (issues are durable, ordered messages)
- Human-readable audit trail (the user can see coordination in their
  issue tracker)
- No new infrastructure dependencies
- Works across forges (GitHub and Gitea both support the reference syntax)
- Incrementally adoptable (Phase 1 is read-only dependency tracking)

Cons:

- Higher latency than direct RPC (polling interval)
- Forge API rate limits apply (mitigated by Gitea for primary runtime)
- Cross-project dependency resolution adds complexity to the dispatcher

**Option D: Full A2A Protocol**

Implement Google's Agent-to-Agent protocol for inter-project
communication, with Agent Cards, task lifecycle management, and
multi-modal message support.

Pros:

- Industry standard with broad adoption (Google, Microsoft, Amazon)
- Designed for heterogeneous agent ecosystems
- Future-proof for external integrations

Cons:

- Significant implementation overhead for a homogeneous, single-operator
  system
- Requires HTTP server endpoints per project (Agent Card hosting)
- Capability negotiation is unnecessary when all projects are registered
  in the same Samverk instance
- Over-engineered for current needs

## Decision

**Option C: Cross-Project Issue References.** Extend the existing
issue-based communication protocol to support cross-project dependencies
and coordination issues. The forge issue tracker remains the single
communication channel. The `ProjectRegistry` provides discovery. The
dispatcher's dependency resolution is extended to query other registered
projects' trackers.

### Key Design Points

**Addressing:** Cross-project references use `owner/repo#number` syntax
in the `depends_on` field, matching the native GitHub/Gitea cross-repo
reference format.

**Discovery:** The `ProjectRegistry` is extended with per-project
`agent_types` metadata. No separate discovery protocol is needed.

**Conflict prevention:** Issues cross boundaries, code does not. An
agent in Project A never modifies files in Project B. It creates a
coordination issue, and Project B's agents execute the work.

**Handoff protocol:** Coordination issues carry full context in the
issue body. No hidden state. The source project is recorded in a
`source_project` frontmatter field for completion notification routing.

**Autonomy:** Cross-project issue creation defaults to Tier 2
(notify user). The user can override per project or globally.

**Failure handling:** Failures in the target project follow the standard
retry and escalation protocol (ADR-027). No automatic retry across
project boundaries.

### Schema Extension

The communication protocol schema is bumped to v1.1.0:

- `depends_on` accepts `(int | string)[]` (strings are cross-project
  refs)
- `source_project` (optional string): `owner/repo` of the originator
- `coordination` added to the `type` enum

### Implementation Phases

1. **Cross-project dependency tracking** -- `depends_on` resolution
   across registered projects (read-only, no issue creation)
2. **Coordination issue creation** -- dispatcher creates issues in target
   projects with autonomy checks
3. **Cross-project digest** -- check-in aggregation across all projects
4. **Agent Card discovery** -- deferred until inter-instance coordination
   is needed

## Consequences

**Positive:**

- The user is no longer the message bus between projects
- Cross-project dependencies are tracked and automatically unblocked
- The audit trail (issues, comments, labels) remains human-readable
- No new infrastructure dependencies
- Incremental adoption: Phase 1 is backward-compatible

**Negative:**

- The dispatcher's polling loop must cover all registered projects,
  increasing API calls proportional to the number of projects
- Cross-project dependency cycles are harder to detect (requires
  building a graph that spans multiple trackers)
- Forge API rate limits on GitHub may constrain polling frequency for
  projects on that forge (Gitea has no rate limits)
- The `depends_on` field becomes a union type, adding parsing complexity

**Risks:**

- **Stale cross-project references**: if a target project is
  deregistered or its repo is deleted, dangling `depends_on` references
  will block the source issue indefinitely. Mitigation: periodic
  reference validation with escalation to user.
- **Polling latency**: cross-project unblocking depends on the polling
  interval (default 30s). For the target user checking in every 1-2
  days, this latency is negligible.

## Related

- [Cross-Agent Coordination Design](../cross-agent-coordination.md)
- [ADR-006: Async-First](ADR-006-async-first.md)
- [ADR-012: Git Issues as Communication Protocol](ADR-012-git-issues-protocol.md)
- [ADR-013: Forge Abstraction](ADR-013-forge-abstraction.md)
- [ADR-015: Three-Tier Autonomy](ADR-015-three-tier-autonomy.md)
- [ADR-023: Per-Project Repos with Coordination Layer](ADR-023-per-project-repos.md)
- [ADR-027: Failure Recovery](ADR-027-failure-recovery.md)
- [ADR-031: Dual-Forge Operational Model](ADR-031-dual-forge-operational-model.md)
