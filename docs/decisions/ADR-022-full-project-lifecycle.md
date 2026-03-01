# ADR-022: Full Project Lifecycle — Idea to Delivery

## Status

Accepted

## Context

Samverk's architecture as originally scoped assumes the user already knows what they're building. The agent hierarchy, dispatcher, communication protocol, and autonomy model all govern the execution of a defined project — but nothing governs the phases that determine whether a project should exist, what it should do, or how it should be structured.

In practice, the pre-project phases are where the highest-leverage decisions happen. During development of Docker Desktop extensions (DockPulse, PacketDeck, RunNotes), the research and feasibility phase produced decisions that saved more effort than any amount of good execution could have recovered:

- Watchtower was found to be effectively dead — the project pivoted to a native registry checker instead of wrapping a dying tool
- n8n extension was found to have weak demand and an abandoned predecessor — the idea was killed entirely, saving weeks of development
- PacketDeck's tiered capability model (topology first, capture later) emerged from competitive analysis of Edgeshark and netshoot — not from coding

These decisions — pivot, kill, scope, sequence — all happened before a single line of code existed. The current Samverk framework has no structured way to produce them.

The target user (solo developer with limited time) is exactly the person least equipped to manage rigorous project governance. They have ideas but not the bandwidth to run competitive analysis, feasibility studies, legal trademark checks, cost-benefit analysis, or structured go/no-go reviews. Enterprise teams have entire departments for this. Solo developers skip it and pay the cost later — or their projects die from poor scoping before they ever start.

## Decision

Samverk's scope expands from "project execution framework" to "full idea-to-delivery lifecycle." The framework now covers:

1. **Idea Intake** — capturing raw ideas from any source, any device, any level of polish
2. **Research & Feasibility** — structured investigation of viability, competition, technical approach
3. **Go/No-Go Gating** — evidence-based decisions to proceed, pivot, or kill
4. **Requirements & Architecture** — translating approved concepts into buildable specifications
5. **Project Scaffolding** — creating the repo, issues, structure, and handoff documents
6. **Execution** — the existing Samverk pipeline (dispatcher, agents, QC, check-in)
7. **Delivery** — publishing, deployment, marketplace submission

The critical design principle: **casual input, rigorous process.** The user interface for idea intake and brainstorming is deliberately informal — a text message, a voice note transcription, a half-sentence from a phone. Samverk's agents transform that informal input into structured, enterprise-grade project governance behind the scenes.

New agent types are introduced for the pre-project phases. New issue types extend the communication protocol. Approval gates between phases use the Intent Verification Protocol's tiered model (ADR-021) — routine phase transitions proceed autonomously, pivots require confirmation, kills escalate to the user.

## Consequences

- Samverk becomes a complete project lifecycle manager, not just an execution engine
- The value proposition strengthens significantly: the solo developer gets a "project management office in a box"
- New agent types (ideation, feasibility, requirements) must be designed and specified
- The communication protocol needs new issue types for pre-project phases
- Phase gate logic adds complexity to the dispatcher
- Research deliverables need formalized standards to ensure consistency
- The casual-input-to-structured-output pipeline is a significant design challenge
- Cost model must account for research-phase token usage (potentially high for web search, competitive analysis)

## References

- [Project Lifecycle](../project-lifecycle.md)
- [Intent Verification Protocol](../intent-verification.md)
- [Architecture](../architecture.md)
- [Communication Protocol](../communication-protocol.md)
- [Concept](../concept.md)
