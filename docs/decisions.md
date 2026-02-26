# Decision Log

A running record of key decisions made during the project, including context and rationale. Captures the "why" that gets lost over time.

---

## 2026-02-26

### Decision: Project Name = Samverk
**Context:** 30+ candidates researched across Norse mythology, Latin/Greek forge concepts, and invented compounds. Nearly all viable options were taken by funded US tech companies.

**Decision:** Use the Icelandic word "samverk" (cooperative work).

**Rationale:**
- Perfect conceptual fit — literally means what the framework does
- No US tech/software conflicts found
- Personal connection to Iceland (founder lived there)
- All-standard Latin letters, keyboard-friendly
- Authentic rather than invented

**Risks accepted:** .com domain is taken by an unrelated Norwegian hotel co-op. Will use alternative TLD (.io or .ai recommended).

---

### Decision: Application Layer, Not Infrastructure Layer
**Context:** The multi-agent framework space in 2026 is dominated by Google ADK, Microsoft Agent Framework, OpenAI Agents SDK, and others at the infrastructure layer.

**Decision:** Samverk will be an application-layer framework built *on top of* existing AI providers — not competing with them.

**Rationale:**
- Solo builder cannot compete with Google/Microsoft/OpenAI at infrastructure
- Application layer has clear unmet need (solo developer / indie hacker segment)
- Differentiation is in UX and mental model, not in low-level orchestration primitives

---

### Decision: Claude-Only for V1
**Context:** Cross-provider validation (using Claude to validate GPT-4 output) is a compelling differentiator, but adds significant complexity.

**Decision:** V1 ships Claude-only. Provider abstraction is built in from day one to enable V2 multi-provider support cleanly.

**Rationale:**
- Reduces V1 scope to something shippable
- Anthropic's Claude API is the developer's primary tool already
- Clean abstraction means V2 doesn't require a rewrite

---

### Decision: Custom Orchestration, Not Built on LangChain/CrewAI
**Context:** Existing frameworks like LangGraph and CrewAI could be used as a base.

**Decision:** Custom orchestration layer.

**Rationale:**
- Orchestration logic is Samverk's core IP
- Building on others' abstractions constrains the design
- Acquisition-readiness requires owning the core differentiation
- Third-party breaking changes would cascade into Samverk

---

### Decision: Go as Implementation Language
**Context:** Consistent with the Subnetree project (same developer, same stack).

**Decision:** Go.

**Rationale:**
- Developer already learning Go via Subnetree
- Good performance characteristics for orchestration workloads
- Strong CLI tooling ecosystem
- Single-binary distribution fits the target user (solo dev, easy install)

---

## Open Decisions

These decisions have not yet been made and need research/design work:

- [ ] **Depth calibration** — Who/what decides how deep the agent tree goes for a given task?
- [ ] **"Good enough" threshold** — When is QC satisfied? How many cycles before escalating?
- [ ] **Cost management** — How are token budgets expressed, tracked, and enforced?
- [ ] **Licensing** — BSL 1.1 / Apache 2.0 dual (like Subnetree), or different?
- [ ] **Domain selection** — samverk.io vs samverk.ai (both need availability check)
- [ ] **V1 scope** — What's the minimum viable framework that proves the concept?
