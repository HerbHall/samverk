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

- ~~**Primary interface.**~~ Resolved: Chat (Claude + MCP) is the primary interface ([ADR-011](decisions/ADR-011-chat-as-interface.md)). Web dashboard handles operations ([ADR-020](decisions/ADR-020-web-dashboard.md)). See [Tech Stack](tech-stack.md).
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
- ~~**Unanticipated action classification.**~~ Partially resolved: Intent Verification Protocol (ADR-021) defines concern flagging for discovered conflicts during execution, and tier classification heuristics default to rounding UP when signals are ambiguous. Remaining question: should unclassifiable actions hard-block or default to Tier 3? See [Intent Verification Protocol](intent-verification.md).
- **Audit log format.** What is the audit log format for Tier 1 and Tier 2 actions reviewed at check-in?

## Multi-Session Coordination

*Discovered 2026-03-01: Two sessions (claude.ai + Claude Code) worked on the same repo simultaneously. No file conflicts occurred by luck (docs vs code), but there was no mechanism to detect, prevent, or coordinate this. The uncommitted docs from claude.ai could have been overwritten or missed entirely if the CC session had also edited docs.*

- **Parallel session awareness.** When multiple AI sessions (or human + AI) work on the same project concurrently, how does Samverk detect and coordinate this? Git branch isolation helps for code, but docs and config files are shared surfaces.
- **Uncommitted work handoff.** When one session ends with uncommitted changes, how is the next session informed? CLAUDE.md handoff sections work but are fragile — the receiving session has to read CLAUDE.md before touching anything, and nothing enforces that.
- **Session state persistence.** A chat session (claude.ai, mobile) has no persistent state beyond memory. Design decisions, context, and rationale from that session evaporate unless explicitly written to files. How does Samverk ensure that session-generated knowledge is captured before the session ends?
- **"I'll forget" safeguard.** The user explicitly said "I will forget" about uncommitted work. This is a predictable failure mode for the target user (solo dev with limited time, context-switching between devices). Samverk should detect and surface uncommitted work, pending decisions, and stale handoff notes at check-in.
- **Cross-session conflict prevention.** Should Samverk use advisory locks, branch conventions, or a "session registry" to prevent two agents (or an agent and user) from editing the same files concurrently?
- **User as participant, not superuser.** Long-term, the user should work through the same issue checkout / branch / PR / QC pipeline as agents. This eliminates the "most dangerous actor" problem where the user has the most access and the least structure. Design questions: When should this be enforced vs. optional? How does the user "break glass" for emergencies? Does the user's QC gate differ from an agent's?
- **Uncommitted work detection at check-in.** The check-in digest should scan active project repos for uncommitted changes, stale branches, and pending handoff notes. This catches the "I'll forget" failure mode automatically.

## Multi-Project Architecture

*Decided in ADR-023: per-project repos with coordination layer. These implementation questions remain.*

- **Project registry format.** Config file (YAML/TOML) or database table? The registry maps project names to forge URLs, lifecycle phases, and autonomy overrides. It must survive a Samverk reinstall.
- **Dispatcher multi-repo polling.** The dispatcher must watch issue trackers across multiple forges simultaneously. What's the polling strategy? Per-project poll intervals? Webhook registration where supported?
- **Gitea-to-GitHub issue migration.** When a project moves from Gitea (development) to GitHub (release), how are issues migrated? Samverk's forge abstraction can read from one and write to the other, but what about issue cross-references, comment history, and label recreation?
- **Pre-repo idea storage.** Ideas (Phase 1) and research (Phase 2) exist before a project repo does. They currently live in `D:\Project Ideas\` as local files. When should they move into the coordination layer, and what does that storage look like?
- **Samverk bootstrap dependency.** Samverk managing its own development creates a circular dependency: the framework must be functional enough to manage its own backlog. At what point does Samverk "eat its own dog food" vs. using manual processes?

## Project Lifecycle

- **Idea intake from multiple devices.** How does a text message or voice note from a phone get into the Samverk idea pipeline? MCP from mobile? Dedicated intake endpoint? Email-to-issue gateway?
- **Research agent web access.** Research and feasibility agents need web search, GitHub/npm/Docker Hub queries, and possibly API exploration. What tools and permissions do they need? How are search costs tracked?
- **Feasibility document standards.** What's the minimum viable feasibility deliverable? How do we prevent research from becoming unbounded scope creep? What signals tell the research agent "you have enough — produce the assessment"?
- **Multi-project resource allocation.** When multiple ideas are in the pipeline at different phases, how does the dispatcher prioritize research tokens and agent time across them?
- **Parked idea revival.** How does a parked idea get back into the pipeline? User request only, or can the system suggest reviving a parked idea when new information makes it more viable?
- **Kill rationale preservation.** When an idea is killed, how is the rationale preserved so the user (or a future agent) doesn't re-research the same dead end?
- **Research-to-execution handoff.** The feasibility and requirements phases produce documents (HANDOFF.md, ARCHITECTURE.md, etc.). How do execution agents consume these — full context injection, or structured references?

## Intent Verification

- **Tier classification accuracy.** How do we measure whether agents are classifying tasks into the correct IVP tier? What's the feedback mechanism when a Tier 1 classification leads to rework?
- **Verification latency budget.** What's the acceptable time cost for Tier 2 and Tier 3 verification exchanges in an async system? How long should a director wait for a worker's Tier 2 confirmation before timing out?
- **Concern flag noise.** How do we prevent workers from over-flagging routine complexity as concerns? What's the threshold between "expected difficulty" and "genuine assumption conflict"?
- **Calibration data collection.** What telemetry is needed to track IVP tier classifications vs. outcomes (rework events, successful completions) for heuristic refinement?

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
