# Open Questions

Problems that must be resolved before or during development.

## Architecture

- **Hierarchy depth calibration.** How does the orchestration layer determine appropriate depth for a given task? A bug fix doesn't need all layers. A new feature might. What's the decision logic?
- **State persistence across sessions.** If the system is down for 8 hours, how does it resume without losing context? How is agent state serialized and restored?
- **Production/QC deadlock arbitration.** When Production and QC agents disagree and neither will yield, what's the arbitration mechanism before escalating to the user?
- **Task dependency management.** Agent B may depend on Agent A's output. How does the system manage the DAG of dependencies? What happens when a dependency fails?

## Cost and Resources

- **Token budget granularity.** How are token budgets expressed? Per task? Per session? Per day? User-defined? Some combination?
- **Model tier selection.** How does the system decide which model tier to use for a given task? What signals determine "this needs cloud" vs. "local is fine"?
- **Cost reporting UX.** How is cost reported to the user in a way that's meaningful without being anxiety-inducing?
- **Minimum hardware spec.** What's the minimum viable hardware spec for a useful local agent setup?

## User Interface

- **Primary interface.** Web app, desktop app, CLI, API? What ships first?
- **Mobile experience.** Native app, PWA, or mobile web?
- **Developer tool integration.** How does Samverk integrate with VS Code, GitHub, etc.?
- **Check-in digest design.** What does the check-in digest look like? How is blocked work prioritized for display?
- **Direction input method.** How does the user provide direction asynchronously -- chat, structured forms, voice?

## Multi-Model and Providers

- **API key management.** How are provider API keys managed securely across devices?
- **Failover priority logic.** What's the failover priority logic when a provider's credits are exhausted?
- **Unavailable vs. slow detection.** How does the system detect that a provider is unavailable vs. just slow?
- **Rate limit handling.** How are provider-specific rate limits handled to avoid burning credits on retries?

## Quality and Validation

- **"Good enough" threshold.** What is the QC sign-off threshold? Is it configurable per task type?
- **QC retry budget.** How many QC retry cycles before escalating to user vs. shipping with a warning?
- **Cross-model validation mechanics.** How does cross-model validation work in practice? What does "Claude reviews GPT-4's output" look like architecturally?
- **Test coverage enforcement.** Can Samverk be configured to require tests before marking work complete?

## Project and Context

- **Long-term context maintenance.** How does Samverk maintain project context across long timescales (months)?
- **Codebase evolution handling.** How does the system handle breaking changes in the project's own codebase as it evolves?
- **Decision capture.** How are user decisions and rationale captured so agents don't re-ask resolved questions?
- **Existing project onboarding.** How does Samverk handle onboarding an existing project vs. greenfield?

## Communication Protocol

- **Maximum issue size.** What is the maximum useful issue size before context becomes unwieldy for agents?
- **Code artifact handling.** How are file attachments / code artifacts handled in issues? Links to repo paths? Inline code blocks?
- **Issue spike handling.** How does the dispatcher handle a sudden spike of 50 new issues? Priority queue design?
- **Polling interval.** What's the right polling interval for agents watching the issue tracker? Too fast = API rate limit risk. Too slow = latency on task pickup.
- **Webhook failure recovery.** How are webhook delivery failures handled? What's the polling fallback strategy?
- **Forge outage handling.** How does the system handle a git forge being unreachable (self-hosted Gitea down)?
- **Sub-task structure.** Should sub-tasks be child issues or checklist items within a parent issue? Child issues are trackable individually; checklists are simpler.
- **Mobile MCP authentication.** How does the front-end chat agent authenticate to the issue tracker MCP server from a mobile device securely?

## MCP and Front-End

- **Existing GitHub MCP servers.** Which MCP server implementation to use for GitHub? Existing open source options?
- **Gitea MCP server.** Does a Gitea MCP server exist or does one need to be built?
- **MCP auth across devices.** How does MCP authentication work across devices -- token stored per-device or central config the chat agent accesses?
- **Claude mobile MCP.** When Claude mobile gets MCP support, what will the configuration experience look like? Can we design for it now so zero migration is needed?
- **Front-end model flexibility.** Should the front-end agent be Claude-only or also support other chat models?

## Autonomy and Trust

- **Tier 3 block communication.** How does the system communicate a Tier 3 block to the user without creating anxiety? (The project is not broken -- one action is waiting.)
- **Trust tier override scoping.** How are trust tier overrides scoped -- per project, per agent type, per action?
- **Temporary tier promotion.** Can trust tiers be promoted temporarily? ("Auto-approve merges for the next 2 hours")
- **Unanticipated action classification.** How does the system detect when an agent is about to take an action that was not anticipated at design time -- and what tier does it default to?
- **Audit log format.** What is the audit log format for Tier 1 and Tier 2 actions reviewed at check-in?

## User Profile

- **Devkit ingestion.** How does Samverk ingest an existing Devkit-style repo on first setup?
- **Profile update flow.** Does the agent propose profile changes and the user approves, or can agents update the profile autonomously within certain bounds?
- **Config conflict resolution.** How are conflicts handled between project-level config and profile-level config? (Project overrides profile? Or profile overrides project?)
- **Profile versioning.** How is the profile versioned? If the user's preferences evolve, old projects should use the conventions they were started with.
- **Profile storage location.** Should the user profile be stored in a dedicated repo, a Samverk-hosted service, or locally? Each has tradeoffs for the multi-device use case.

## Business and Product

- ~~**V1 scope.**~~ Resolved in [ADR-018](decisions/ADR-018-release-versioning.md). Three-phase: v0.0.1 alpha, v0.1 beta, v1.0 public.
- ~~**Hosting model.**~~ Resolved in [ADR-019](decisions/ADR-019-self-hosted-first.md). Self-hosted-first, cloud as fallback.
- **Server platform.** Unraid vs Proxmox vs Windows for the dedicated project server. RTX 3090 Ti available. Needs research spike.
- **Dogfooding.** How does Samverk itself get built -- can Subnetree serve as the dogfood project?
- **Licensing model.** What fits the target user -- subscription, usage-based, open core?
