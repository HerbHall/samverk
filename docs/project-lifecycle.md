# Project Lifecycle

**ADR**: [ADR-022](decisions/ADR-022-full-project-lifecycle.md)

## The Core Principle

**Casual input, rigorous process.**

The user texts a half-baked idea from their phone at a red light. Samverk turns it into a fully researched, properly scoped, evidence-based project plan — or kills it early with a clear rationale. The user provides creative direction and approval. Samverk provides the discipline, structure, and legwork that a solo developer doesn't have the time or skills to manage.

This is the "project management office in a box" for people who would never build one themselves.

## The Seven Phases

```text
 ┌──────────────┐
 │  1. INTAKE   │  ← Napkin sketch, voice note, chat message
 └──────┬───────┘
        │ auto
 ┌──────▼───────┐
 │ 2. RESEARCH  │  ← Competitive analysis, technical feasibility, market gap
 └──────┬───────┘
        │ gate
 ┌──────▼───────┐
 │ 3. GO/NO-GO  │  ← Evidence-based decision: proceed, pivot, or kill
 └──────┬───────┘
        │ gate
 ┌──────▼───────┐
 │ 4. REQUIRE-  │  ← Requirements, architecture, key decisions
 │    MENTS     │
 └──────┬───────┘
        │ gate
 ┌──────▼───────┐
 │ 5. SCAFFOLD  │  ← Repo, issues, structure, handoff documents
 └──────┬───────┘
        │ auto
 ┌──────▼───────┐
 │ 6. EXECUTION │  ← Existing Samverk pipeline (dispatcher, agents, QC)
 └──────┬───────┘
        │ gate
 ┌──────▼───────┐
 │ 7. DELIVERY  │  ← Publishing, deployment, marketplace, announcement
 └──────────────┘
```

Transitions marked "auto" proceed without human approval when the phase completes successfully. Transitions marked "gate" require an approval decision — the tier of that approval depends on risk and cost (see Phase Gates below).

## Phase 1 — Idea Intake

### Purpose

Capture raw ideas from any source, any device, any level of completeness. The user should never lose an idea because they didn't have time to write it up properly.

### Input Flexibility

Samverk accepts ideas in any form the user can provide:

- A sentence in a chat message ("what if I built a Docker extension that shows container network traffic")
- A link to something inspiring ("check out this tool: [url] — could we do something like this for Docker?")
- A bullet list of features
- A comparison ("I want X but for Y")
- A problem statement ("it bugs me that Docker Desktop doesn't show update availability")
- A forwarded article, screenshot, or conversation snippet
- Voice transcription (when supported by the chat interface)

The system does NOT require structured input at this stage. That's the whole point.

### What the System Does

The **Ideation Agent** receives the raw input and produces a structured Idea Brief:

```markdown
---
schema_version: "1.0.0"
type: idea
phase: intake
priority: normal
---

## Idea Brief

**Working Title**: [generated from user input]
**Origin**: [date, source — chat message, link, etc.]
**Raw Input**: [preserved exactly as provided by user]

## Interpreted Problem
[What problem does this solve? One paragraph.]

## Interpreted Solution
[What would the product/feature look like? One paragraph.]

## Initial Signals
- **Similar to**: [known products, projects, or patterns]
- **Target user**: [who benefits]
- **Novelty assessment**: [is this new, or does it exist already?]

## Suggested Research Questions
1. [Does this already exist?]
2. [Is there demand / a market gap?]
3. [What are the key technical risks?]
4. [Are there legal/trademark concerns?]

## Status
Awaiting research phase or user direction.
```

### Phase Gate: Intake → Research

**Default: Auto-advance.** Every captured idea proceeds to at least a lightweight research pass unless the user explicitly says otherwise. The research phase has its own tiering to control depth.

The user can also:

- Explicitly kill an idea at intake ("never mind, scratch that")
- Prioritize an idea ("this one first")
- Merge ideas ("combine this with that other idea I had")
- Park an idea ("interesting but not now — save it for later")

Parked ideas are stored as issues with a `phase:parked` label and can be resurrected at any check-in.

## Phase 2 — Research & Feasibility

### Purpose

Transform the Idea Brief into an evidence-based Feasibility Assessment. This is where Samverk earns its keep — doing the legwork the solo developer doesn't have time for.

### Research Tiers

Not every idea warrants a full feasibility study. Research depth is tiered:

**Quick Scan** (1-2 hours agent time, Tier 1 auto-advance)

- Does this already exist? (GitHub, Docker Hub, npm, app stores)
- Is there an obvious fatal flaw?
- Quick competitive landscape (top 3-5 existing solutions)
- Output: 1-page summary with go/no-go recommendation

**Standard Feasibility** (4-8 hours agent time, Tier 2 confirmation to start)

- Full competitive analysis (existing tools, their strengths/weaknesses, market gaps)
- Technical feasibility assessment (APIs, dependencies, platform constraints)
- Effort estimation (MVP timeline, full product timeline)
- Naming and trademark conflict check
- Output: Full feasibility document (comparable to DockPulse/PacketDeck research)

**Deep Research** (1-3 days agent time, Tier 3 approval to start)

- Everything in Standard, plus:
- Legal and licensing analysis (target market regulations, open source license compatibility)
- Cost-benefit model (development cost vs. revenue/value potential)
- User research synthesis (forum discussions, feature requests, community sentiment)
- Technical proof-of-concept (build a minimal spike to validate the hardest technical risk)
- Output: Comprehensive research package with financial model

The Ideation Agent recommends a research tier based on the idea's complexity and the user's apparent intent. The user can override up or down.

### Research Deliverables (Standard)

A Standard Feasibility assessment produces:

1. **Competitive Landscape** — What exists, what's dead, what's thriving, where the gaps are
2. **Technical Assessment** — Can it be built? What are the hard parts? What's the stack?
3. **Effort Estimate** — MVP scope and timeline, full product scope and timeline
4. **Risk Register** — Technical risks, market risks, legal risks, dependency risks
5. **Name Research** — Candidate names checked against GitHub, Docker Hub, npm, trademarks
6. **Recommendation** — Proceed / pivot / kill, with specific rationale

### Agents Involved

| Agent | Role in Research Phase |
|-------|----------------------|
| `agent:research` | Web search, competitive analysis, community sentiment, market gaps |
| `agent:feasibility` | Technical assessment, effort estimation, risk identification |
| `agent:legal` | Trademark/naming conflicts, license compatibility, regulatory concerns |
| `agent:ideation` | Synthesis — combines research into coherent feasibility document |

### Phase Gate: Research → Go/No-Go

**Default: Tier 2** — Research agent presents findings and recommendation, waits for user confirmation before advancing to formal go/no-go review.

If research reveals a clear fatal flaw (existing identical product, insurmountable technical barrier, legal blocker), the agent can recommend killing the idea. The kill decision itself is always **Tier 3** — the user makes the final call on whether an idea dies.

## Phase 3 — Go/No-Go Decision

### Purpose

Explicit decision point where the user (or the orchestrator with delegated authority) decides: proceed as-is, pivot the concept, or kill it.

### Decision Framework

The Go/No-Go review presents:

```markdown
## Go/No-Go Review: [Project Name]

### Feasibility Summary
[2-3 sentence synopsis]

### Scorecard
| Factor | Rating | Notes |
|--------|--------|-------|
| Market gap | Strong / Moderate / Weak | [why] |
| Technical feasibility | High / Medium / Low | [why] |
| Competitive advantage | Clear / Marginal / None | [why] |
| Effort vs. value | Favorable / Neutral / Unfavorable | [why] |
| Risk level | Low / Medium / High | [key risks] |

### Recommendation
[Proceed / Pivot / Kill — with specific rationale]

### If Proceed
- Suggested MVP scope: [brief]
- Estimated effort: [range]
- Key decisions needed: [list]

### If Pivot
- Suggested direction: [what changes]
- What's preserved: [reusable research/work]

### If Kill
- Rationale: [clear reason]
- Salvageable: [any reusable components or insights]
```

### Decision Rules

| Decision | IVP Tier | Who Decides |
|----------|----------|-------------|
| Proceed (recommendation aligned) | Tier 2 | User confirms |
| Proceed (against recommendation) | Tier 3 | User overrides with rationale |
| Pivot | Tier 3 | User directs new scope, re-enters research |
| Kill | Tier 3 | User confirms — ideas only die by user choice |
| Park (defer) | Tier 1 | User or agent can park at any time |

### Key Principle: Ideas Only Die by User Choice

An agent can recommend killing an idea. An agent can present overwhelming evidence that an idea won't work. But the actual kill decision is always the user's. This prevents agents from prematurely discarding ideas that the user has personal conviction about — sometimes the "irrational" idea is the one worth building.

## Phase 4 — Requirements & Architecture

### Purpose

Translate the approved concept into buildable specifications. Produce the documents that the execution pipeline needs to begin work.

### Deliverables

1. **Requirements Document** — What the product does, framed as user stories or acceptance criteria. Uses MoSCoW prioritization (Must/Should/Could/Won't) to define MVP boundary.

2. **Architecture Design** — Technical structure, component breakdown, data model, API design, technology choices with rationale. Key decisions documented as ADRs.

3. **Issue Backlog** — The full set of implementation tasks, properly sequenced with dependencies, labeled by agent type and priority. Ready for the dispatcher.

4. **HANDOFF Document** — Context document for execution agents: what exists, what's decided, what's still open, what NOT to do. (Pattern established in RunNotes/DockPulse/PacketDeck.)

5. **CLAUDE.md** — Project context file for Claude Code sessions.

### Agents Involved

| Agent | Role in Requirements Phase |
|-------|---------------------------|
| `agent:orchestrator` | Decomposes approved concept into requirements and architecture |
| `agent:research` | Fills technical knowledge gaps (API docs, SDK capabilities, library options) |
| `agent:ideation` | Validates that requirements align with original user intent |
| `agent:legal` | License selection, dependency license audit |

### Phase Gate: Requirements → Scaffold

**Default: Tier 2** — Orchestrator presents the requirements package, user confirms before the repo is created and issues are filed.

For projects following an established template (e.g., another Docker Desktop extension following the RunNotes pattern), this gate can be **Tier 1** — restate and proceed.

## Phase 5 — Project Scaffolding

### Purpose

Create the physical project: repository, directory structure, configuration files, issue backlog, CI/CD, and all the infrastructure needed for the execution phase.

### Deliverables

- Git repository (local + remote)
- Standard file scaffold (README, LICENSE, CONTRIBUTING, CHANGELOG, .gitignore, .editorconfig, etc.)
- Project-specific files (Dockerfile, metadata.json, Makefile, etc.)
- GitHub/Gitea issues created from backlog with proper labels
- Workspace file for VS Code
- SETUP-INSTRUCTIONS.md for agent handoff

### Phase Gate: Scaffold → Execution

**Default: Auto-advance.** Scaffolding is mechanical — once the checklist completes, execution begins. The scaffold completion is reported in the next check-in digest.

## Phase 6 — Execution

### Purpose

Build the thing. This is the existing Samverk pipeline.

### What Already Exists

This phase is fully specified in the existing documentation:

- [Architecture](architecture.md) — agent hierarchy, dispatcher, QC mirror
- [Communication Protocol](communication-protocol.md) — issue schema, label taxonomy
- [Autonomy Model](autonomy-model.md) — trust tiers for agent actions
- [Intent Verification Protocol](intent-verification.md) — pre-execution understanding checks
- [Dispatcher Design](dispatcher-design.md) — task routing and dependency management

### Lifecycle Additions

The execution phase gains awareness of the upstream phases:

- Agents can reference feasibility research and architecture docs for context
- If an agent discovers during execution that a requirement is unimplementable, the concern flagging protocol (IVP) escalates back to the requirements level — not just to the parent task
- The orchestrator can trigger a mini research cycle mid-execution if an unexpected technical question arises

## Phase 7 — Delivery

### Purpose

Get the finished product to users. This is more than "push to main" — it includes publishing, deployment, documentation, and announcement.

### Deliverables (vary by project type)

- **Open source library**: Package registry publishing (npm, PyPI, crates.io), README finalization, release notes, GitHub Release
- **Docker extension**: Docker Hub publishing, marketplace submission, screenshots, description
- **Web application**: Deployment configuration, domain setup, monitoring
- **All projects**: CHANGELOG update, version tag, announcement draft

### Agents Involved

| Agent | Role in Delivery Phase |
|-------|----------------------|
| `agent:code-gen` | Build scripts, CI/CD configuration, deployment automation |
| `agent:docs` | Release notes, README updates, marketplace descriptions |
| `agent:qc` | Final validation, smoke testing |
| `agent:research` | Marketplace requirements, publishing prerequisites |

### Phase Gate: Delivery → Done

**Default: Tier 3.** Publishing is irreversible (or costly to undo). The user confirms before the product goes live.

## New Agent Types

### `agent:ideation`

**Purpose**: Interpret raw user input, produce structured Idea Briefs, synthesize research into coherent assessments, validate that downstream documents align with original user intent.

**Key capability**: Translate casual, incomplete, or ambiguous input into structured knowledge without losing the user's original intention.

**Operates in**: Phases 1, 2, 3, 4

### `agent:feasibility`

**Purpose**: Technical assessment, effort estimation, risk identification, cost-benefit analysis. The engineering-minded analyst.

**Key capability**: Evaluate whether something can be built, how hard it will be, and what could go wrong.

**Operates in**: Phases 2, 3

### `agent:legal`

**Purpose**: Trademark and naming conflict research, license compatibility analysis, regulatory concern identification. Operates as an external contractor pattern — called when needed, not permanently staffed.

**Key capability**: Systematic conflict checking across registries, trademark databases, and license compatibility matrices. NOT legal advice — surfaces concerns for human review.

**Operates in**: Phases 2, 4

**Important constraint**: This agent identifies potential issues and flags them for human review. It does not make legal determinations. Output always includes the disclaimer that the user should consult a qualified professional for actual legal decisions.

## New Issue Types

Extend the communication protocol schema:

| Type | Phase | Description |
|------|-------|-------------|
| `idea` | 1 | Raw idea capture, structured into Idea Brief |
| `research` | 2 | Research task (competitive analysis, technical spike, naming check) |
| `feasibility` | 2 | Feasibility assessment deliverable |
| `gate` | 3, 4, 7 | Approval gate — requires decision to proceed |
| `requirement` | 4 | Individual requirement or user story |
| `architecture` | 4 | Architecture decision or design document |
| `scaffold` | 5 | Project setup task |

Existing types (`task`, `question`, `result`, `block`) continue to operate in Phase 6 (Execution) unchanged.

## New Phase Labels

| Label | Color | Description |
|-------|-------|-------------|
| `phase:intake` | `#D4C5F9` | Idea capture and initial structuring |
| `phase:research` | `#C5DEF5` | Research and feasibility investigation |
| `phase:gate` | `#B60205` | Awaiting go/no-go or approval decision |
| `phase:requirements` | `#0075CA` | Requirements and architecture definition |
| `phase:scaffold` | `#C2E0C6` | Project setup and infrastructure |
| `phase:execution` | `#0E8A16` | Active development (existing pipeline) |
| `phase:delivery` | `#FBCA04` | Publishing and deployment |
| `phase:parked` | `#EDEDED` | Deferred — saved for later |
| `phase:killed` | `#000000` | Abandoned with documented rationale |

## The Casual-to-Structured Pipeline

This is the heart of the design. The transformation path from raw idea to structured project:

```text
User input (casual)          Samverk output (rigorous)
─────────────────           ──────────────────────────
"docker update checker"  →  Idea Brief with interpreted problem/solution
                         →  Competitive landscape (Watchtower, WUD, Diun, ...)
                         →  Technical feasibility with registry API analysis
                         →  Effort estimate (MVP: 3-5 days, full: 2-3 weeks)
                         →  Risk register (rate limits, multi-registry auth)
                         →  Name candidates with conflict checks
                         →  Go/No-Go scorecard with recommendation
                         →  Requirements with MoSCoW prioritization
                         →  Architecture doc with settled decisions
                         →  Issue backlog with dependencies and labels
                         →  Scaffolded repo ready for execution
```

The user's contribution to this pipeline: the initial idea, answers to clarifying questions at check-ins, and approval at gates. Everything else is Samverk's job.

## Multi-Project Pipeline

Samverk manages multiple ideas and projects simultaneously at different phases:

```text
Idea A: ████████████░░░░░░░░░░  Phase 6 (Execution — 60% complete)
Idea B: ████████░░░░░░░░░░░░░░  Phase 4 (Requirements — in progress)
Idea C: ████░░░░░░░░░░░░░░░░░░  Phase 2 (Research — competitive analysis)
Idea D: █░░░░░░░░░░░░░░░░░░░░░  Phase 1 (Intake — just captured)
Idea E: ░░░░░░░░░░░░░░░░░░░░░░  Parked
```

The check-in digest shows all active projects with their current phase, blocking issues, and next actions needed from the user.

## Integration with Existing Protocols

### Intent Verification (ADR-021)

Phase gates are IVP tier decisions:

- Auto-advance gates = IVP Tier 1 (restate and proceed)
- Standard gates = IVP Tier 2 (present plan, wait for confirmation)
- High-stakes gates = IVP Tier 3 (full scope assessment with questions)

Concern flagging during any phase can trigger re-evaluation of earlier phase decisions.

### Autonomy Model (ADR-015)

Pre-project phases introduce new actions that need autonomy tier classification:

| Action | Autonomy Tier | Rationale |
|--------|--------------|-----------|
| Capture idea | 1 | No side effects, preserving user input |
| Conduct research | 1 | Read-only, no project impact |
| Produce feasibility doc | 1 | Creating analysis, not committing to action |
| Recommend kill/pivot | 2 | Significant recommendation, user should see it |
| Create repository | 2 | Visible external action |
| File issues | 1 | Proposals, not code changes |
| Proceed past gate | varies | Depends on gate tier (see Phase Gates) |
| Publish/deploy | 3 | Irreversible external action |

### Communication Protocol

New issue types and phase labels extend the existing schema. The `schema_version` field increments to `1.1.0` (new optional fields, backward compatible).

## Related Documents

- [ADR-022: Full Project Lifecycle](decisions/ADR-022-full-project-lifecycle.md)
- [Architecture](architecture.md) — system structure (updated to reference lifecycle)
- [Communication Protocol](communication-protocol.md) — issue schema (extended with new types)
- [Autonomy Model](autonomy-model.md) — trust tiers for new phase actions
- [Intent Verification Protocol](intent-verification.md) — gate decisions use IVP tiers
- [Concept](concept.md) — value proposition (strengthened by lifecycle scope)
