# Competitive Landscape

## Market Context (Early 2026)

The multi-agent AI framework space is moving fast:

- 61% of organizations began agentic AI development as of January 2025
- Gartner predicts 33% of enterprise software will include agentic AI by 2028
- Gartner also predicts 40% of agentic AI deployments will be canceled by 2027 due to cost, unclear value, and poor risk controls

The failure rate prediction is a signal: the market needs frameworks that make value clear and risks manageable -- not just frameworks that are technically impressive.

## The Sync/Async Divide

**Every existing AI dev tool is synchronous.** The user sits at their keyboard, prompts, waits, reviews, repeats. This is fine for full-time developers. It doesn't serve the hobbyist developer with 10-15 minutes of availability.

| Tool Category | Interaction Model | Samverk's Difference |
|---------------|-------------------|----------------------|
| AI code assistants (Copilot, Cursor) | Real-time, in-editor | Async, background |
| Agent frameworks (CrewAI, LangGraph) | Developer-configured, synchronous runs | User-directed, continuous operation |
| AI chatbots (ChatGPT, Claude) | Session-based Q&A | Project-scoped, persistent context |
| CI/CD systems (GitHub Actions) | Triggered by events | Continuously working, user checks in |

The async model is Samverk's primary differentiator. Everything else -- the hierarchy, the QC mirror, the multi-model support -- serves the goal of making async development reliable enough to trust.

## Existing Frameworks

### Infrastructure / Orchestration Layer

*Built by Google, Microsoft, OpenAI, Anthropic. Not viable targets for a solo builder to compete against directly.*

| Framework | Owner | Notes |
|-----------|-------|-------|
| Microsoft Agent Framework | Microsoft | Converges AutoGen + Semantic Kernel; enterprise-grade |
| Google ADK | Google | Multi-agent orchestration for Gemini ecosystem |
| OpenAI Agents SDK | OpenAI | Native tooling for GPT-4 agent workflows |
| Semantic Kernel | Microsoft | Enterprise AI orchestration |

### Application / Developer Layer

| Framework | Approach | Gap |
|-----------|----------|-----|
| CrewAI | Role-based agent "crews" | Synchronous, requires AI engineering knowledge |
| LangGraph | Graph-based precise control | Synchronous, steep learning curve |
| AutoGen | Microsoft research project | Research-oriented, not production-ready |
| LangChain | Pipeline/chain composition | Synchronous, abstraction leaks, high maintenance |

**None of these are async-first. None target the non-professional developer.**

## What's Not Being Done Well

### Async Operation

No framework operates as a persistent background engine. Every tool assumes the user is present and watching. This excludes the entire hobbyist/part-time developer segment.

### Cross-Provider Validation

No major framework uses provider diversity as a quality mechanism. Using Claude to review GPT-4 output (or vice versa) introduces genuine independence in the review chain. Underexplored and potentially a strong differentiator.

### Cost Transparency

Most frameworks treat cost as an afterthought. For a solo developer running agent hierarchies on a personal budget, token cost visibility and budget controls are critical. No current framework makes this first-class.

### Device Flexibility

Existing tools are desktop-only or IDE-integrated. A user who needs to check in from their phone during lunch has no options in the current market.

### Solo Developer UX

Existing frameworks are designed for enterprise (too complex) or experimentation (too fragile). The solo developer building a real product is underserved. The hobbyist developer building a side project is invisible.

## Why Not Build on Existing Frameworks

The orchestration logic -- how agents delegate, when results are "good enough," how QC escalation works, how cost is managed, how async operation is maintained -- is Samverk's actual IP. Building on LangGraph or CrewAI means:

1. Their abstractions constrain Samverk's design
2. Their breaking changes become Samverk's breaking changes
3. The differentiation is buried under someone else's API
4. Async-first behavior would have to be bolted onto a synchronous core

Custom orchestration layer is the right call for a project trying to own its identity and be acquisition-ready.
