# Competitive Landscape

## Market Context (Early 2026)

The multi-agent AI framework space is moving fast:
- 61% of organizations began agentic AI development as of January 2025
- Gartner predicts 33% of enterprise software will include agentic AI by 2028
- Gartner also predicts 40% of agentic AI deployments will be canceled by 2027 due to cost, unclear value, and poor risk controls

The failure rate prediction is a signal: the market needs frameworks that make value clear and risks manageable — not just frameworks that are technically impressive.

---

## Existing Frameworks

### Infrastructure / Orchestration Layer
*These are being built by Google, Microsoft, OpenAI, Anthropic. Not viable targets for a solo builder to compete against directly.*

| Framework | Owner | Notes |
|-----------|-------|-------|
| Microsoft Agent Framework | Microsoft | Converges AutoGen + Semantic Kernel; enterprise-grade |
| Google ADK | Google | Multi-agent orchestration for Gemini ecosystem |
| OpenAI Agents SDK | OpenAI | Native tooling for GPT-4 agent workflows |
| Semantic Kernel | Microsoft | Enterprise AI orchestration |
| Akka | Lightbend | Actor-model framework adapted for agents |

### Application / Developer Layer
*This is where the opportunity is.*

| Framework | Approach | Gap |
|-----------|----------|-----|
| CrewAI | Role-based agent "crews" | Requires AI engineering knowledge to configure |
| LangGraph | Graph-based precise control | High complexity, steep learning curve |
| AutoGen | Microsoft research project | Research-oriented, not production-ready |
| LangChain | Pipeline/chain composition | Abstraction leaks, high maintenance burden |

---

## Samverk's Position

Samverk is **not** an infrastructure framework. It lives at the application layer, built on top of existing providers and orchestration primitives.

The key differentiator is the **mental model**:

| Existing Frameworks | Samverk |
|---------------------|---------|
| "Configure your agent graph" | "Describe your goal" |
| Requires prompt engineering expertise | Handles prompt engineering internally |
| User thinks like an AI engineer | User thinks like a business owner |
| Agents are technical constructs | Agents are team members with roles |

---

## What's Not Being Done Well

### Cross-Provider Validation
No major framework uses provider diversity as a quality mechanism. Using Claude to review GPT-4 output (or vice versa) introduces genuine independence in the review chain. This is underexplored and potentially a strong differentiator in V2.

### Cost Transparency
Most frameworks treat cost as an afterthought. For a solo developer running complex agent hierarchies, token cost visibility and budget controls are critical. No current framework makes this first-class.

### Solo Developer UX
Existing frameworks are designed either for enterprise (too complex) or for experimentation (too fragile). The solo developer building a real product is underserved.

---

## Why Not Build on Existing Frameworks?

The orchestration logic — how agents delegate, when results are "good enough," how QC escalation works, how cost is managed — is Samverk's actual IP. Building that logic *on top of* LangGraph or CrewAI means:
1. Their abstractions constrain Samverk's design
2. Their breaking changes become Samverk's breaking changes
3. The differentiation is buried under someone else's API

Custom orchestration layer is the right call for a project trying to own its own identity and be acquisition-ready.
