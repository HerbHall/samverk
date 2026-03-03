# Shared Agent Memory

- **Status**: Concept
- **Date**: 2026-03-03
- **Author**: Herb Hall, Claude
- **Related**: Open Questions (Multi-Session Coordination, Project Lifecycle), ADR-012 (git issues protocol)

## Problem Statement

When Samverk agents research a topic, evaluate options, or discover constraints, that knowledge exists only in the session that produced it. The next agent -- or even the same agent type running weeks later -- starts from zero. There is no mechanism for agents to deposit findings into a shared, queryable store that other agents (regardless of model or provider) can draw from during future work.

## The Gap

Samverk currently stores **task execution state** (what happened) but not **project knowledge** (what was learned):

| What exists today | What it captures | What it misses |
|---|---|---|
| Git issues + comments | Task status, agent output transcripts | Why decisions were made, what alternatives were rejected |
| SQLite sessions table | Which agent ran, how long, what it cost | What the agent discovered that's reusable |
| Profile store | User preferences, conventions | Project-specific technical knowledge |
| Frontmatter | Structured task metadata | Unstructured research findings |
| Closed issues | That work was done | The reasoning and context behind it |

The result: agents re-research solved problems, re-evaluate rejected options, and miss context that would have changed their approach. As projects grow across phases, this knowledge loss compounds.

## Why This Matters More Than It Appears

The Open Brain concept originated from a single developer using AI tools -- one person, one model at a time. Samverk's context is fundamentally more demanding. In Samverk:

- **The user interacts with many models** -- Claude, GPT-4, Ollama, Gemini, across mobile, desktop, and CLI. Each session produces knowledge that dies when the session ends.
- **Models interact with each other** -- The dispatcher routes work to different agent types (code-gen, test, docs, research, QC). A research agent's findings should directly inform the code-gen agent's implementation without the user having to relay context between them.
- **The efficiency multiplier is compounding** -- When Agent A discovers that "the Gitea API returns 422 for duplicate labels instead of 409" and deposits that into shared memory, Agents B through Z never hit that wall. Across hundreds of agent invocations per phase, this compounds dramatically.

This is not just "notes for AI." It's the connective tissue between agents that currently doesn't exist.

### Selective Access: Bias-Free Evaluation

Not all agents should always see all memory. When a QC agent evaluates another agent's output, it should form an independent assessment **before** seeing what the producing agent "thought." Shared memory must support **selective access** -- the ability to block specific memory scopes during evaluation tasks so one model can assess another's results without inheriting its reasoning biases.

Use cases for access control:

- **QC review**: QC agent evaluates code without seeing the code-gen agent's rationale, then compares its independent assessment against the stored reasoning
- **Cross-model validation**: ADR-030 specifies cross-model QA. The validating model should not see the producing model's chain of thought
- **Research convergence**: Two research agents investigate the same question independently, then their findings are compared for consensus

## Concept: Shared Memory Layer

A persistent, model-agnostic knowledge store where:

1. **Any agent can write** -- Claude, GPT-4, Ollama, Gemini, or a human session can deposit findings
2. **Any agent can query** -- Retrieve relevant prior knowledge before starting work, regardless of which model or session originally produced it
3. **Knowledge is structured** -- Not raw transcripts, but distilled findings with source attribution
4. **Knowledge is searchable by meaning** -- Semantic retrieval, not just keyword matching
5. **Knowledge is scoped to projects** -- Each project has its own memory; cross-project knowledge is explicit
6. **Access is controllable** -- Specific memory scopes can be blocked for bias-free evaluation tasks

### What Gets Stored

Not everything. The memory layer captures **durable project knowledge**, not ephemeral execution state:

| Store | Don't store |
|---|---|
| "OAuth2 PKCE is required for mobile clients per RFC 7636" | "Agent started at 14:32, finished at 14:35" |
| "We evaluated Redis, chose SQLite because single-binary" | "Running go test on 17 packages" |
| "The Gitea API requires token auth, not OAuth" | "PR #143 merged successfully" |
| "sqlite-vec requires Go 1.22+ for generics" | "Build passed on Linux amd64" |
| "User prefers table-driven tests" | "Commit hash abc123" |

### Relationship to DevKit Autolearn

DevKit already implements a form of cross-session learning: the autolearn system captures patterns, gotchas, and corrections, then stores them in rules files (`autolearn-patterns.md`, `known-gotchas.md`) and MCP Memory. These are injected into every Claude Code session.

This works, but has limitations:

- **Claude-only** -- Rules files are loaded by Claude Code. Samverk agents running via the provider registry (Ollama, OpenAI) never see them.
- **Flat text** -- Rules files are markdown lists, not semantically searchable. An agent can't ask "what do we know about SQLite on Windows?" -- it gets all 100+ patterns loaded into context whether relevant or not.
- **Manual promotion** -- Learnings must be explicitly promoted from MCP Memory to rules files via the reflect workflow. Knowledge that stays in MCP Memory is invisible to non-Claude sessions.

A properly designed shared memory layer in DevKit could replace the file-based autolearn storage with something faster, more robust, and model-agnostic. The autolearn *workflow* (detect -> classify -> validate -> store) stays the same, but the *backend* becomes queryable by any model, retrievable by meaning, and scoped appropriately.

**Strategic direction**: DevKit is intended to be incorporated into Samverk. The long-term vision is that Samverk is the single ecosystem for all project work -- from vague idea through research, validation, implementation, ongoing support, updates, and new features. DevKit's cross-session learning, CI templates, rules governance, and project scaffolding all become Samverk capabilities. Shared memory is the first module designed with this convergence in mind: built as a reusable Go library that works standalone today and becomes a native Samverk subsystem when DevKit merges in.

### How It Differs from Existing Systems

- **Not git issues** -- Issues track tasks. Memory tracks knowledge.
- **Not the profile store** -- Profile tracks user preferences. Memory tracks project facts.
- **Not MCP Memory (Claude Code)** -- MCP Memory is Claude-specific and session-scoped. Shared memory is model-agnostic and project-scoped.
- **Not DevKit autolearn rules files** -- Rules files are flat text loaded into every session. Shared memory is semantically searchable and returns only relevant knowledge.
- **Not a vector database** -- Vector search is a retrieval mechanism, not the concept. The concept is institutional memory for AI agents.

## Inspiration and Prior Art

This concept draws from several sources that should be researched in depth before designing the implementation:

### Direct Inspiration

- **Nate Jones "Open Brain" concept** -- PostgreSQL + pgvector as an AI-readable database, with MCP as the interface. Key insight: moving from passive storage (notes) to active memory (AI-queryable). Uses nightly synthesis to find missing links.

### Adjacent Concepts to Research

- **Retrieval-Augmented Generation (RAG)** -- The established pattern for grounding LLM responses in external knowledge. How do production RAG systems handle knowledge freshness, deduplication, and relevance ranking?
- **Knowledge graphs for AI agents** -- Graph-structured knowledge (entities + relations) vs. flat document stores. What are the tradeoffs for agent retrieval? How do systems like Mem0, Zep, and LangMem approach this?
- **Multi-agent memory architectures** -- How do frameworks like AutoGen, CrewAI, and MetaGPT handle shared state between agents? What works, what doesn't?
- **Embedding models for code/technical content** -- General-purpose embeddings (OpenAI ada-002) vs. code-specific embeddings (CodeBERT, StarCoder). Which perform better for the kind of technical knowledge Samverk agents produce?
- **MCP-native memory servers** -- The MCP ecosystem is developing memory-focused servers. What exists? What patterns are emerging?
- **Personal knowledge management (PKM) with AI** -- Tools like Obsidian + AI plugins, Notion AI, Mem.ai. How do they structure knowledge for AI retrieval? What can we learn from their UX?
- **Cognitive architecture patterns** -- ACT-R, SOAR, and other cognitive architectures distinguish working memory, long-term memory, and procedural memory. Do these distinctions apply to AI agent systems?

### Key Questions for Research

1. **Storage format**: Documents with embeddings? Knowledge graph (triples)? Hybrid? What does the evidence say about retrieval quality for each?
2. **Embedding strategy**: Which models work best for technical/code content? Local (Ollama) vs. API? Embedding dimensions vs. retrieval quality tradeoffs?
3. **Knowledge lifecycle**: How do you handle stale or contradicted knowledge? Versioning? Confidence decay? Explicit invalidation?
4. **Retrieval patterns**: Pure vector similarity? Hybrid (vector + keyword)? Re-ranking? What context window budget should retrieval consume?
5. **Write patterns**: Should agents write to memory explicitly (tool call) or should the system extract knowledge from agent outputs automatically?
6. **Deduplication**: How do you prevent the same finding from being stored 50 times by 50 agents?
7. **Trust and attribution**: If Ollama deposits a finding and Claude later retrieves it, how confident should Claude be in knowledge from a less capable model?
8. **Scope boundaries**: Per-project memory is clear. What about cross-project knowledge (e.g., "Go's `time.Duration` causes swagger drift on Windows" applies everywhere)?
9. **Access control for bias-free evaluation**: How do existing multi-agent systems handle selective memory access? What patterns exist for "evaluate independently, then compare"?
10. **DevKit as the home**: Should this be a DevKit library (reusable across all projects) with Samverk as a consumer, or a Samverk-only feature? What are the tradeoffs for each ownership model?
11. **Autolearn migration path**: How would existing DevKit autolearn data (100+ patterns, 80+ gotchas in flat markdown) be migrated into a structured memory store without losing fidelity?
12. **Multi-model write conflicts**: When two agents running different models write contradictory findings about the same topic concurrently, how is the conflict detected and resolved?
13. **Agent performance metrics**: What metrics best predict agent suitability for a task type? First-pass CI success rate? Error category distribution? Cost per quality unit? How many data points are needed before a routing preference is statistically meaningful?
14. **Recurring research scheduling**: How frequently should agent landscape scans run? What triggers a re-scan (new model release, price change, capability benchmark)? How do we avoid stale agent profiles influencing dispatch decisions?
15. **Prompt enrichment from memory**: When the dispatcher enriches an agent's prompt with warnings from past failures, what's the optimal format? How many warnings before context pollution degrades performance?
16. **Query performance at scale**: What access patterns does the dispatcher need? Can SQLite with proper indexing handle sub-50ms multi-query dispatch decisions, or does this require an in-memory layer? What materialized views or pre-aggregated summaries eliminate expensive runtime queries?
17. **Confidence scoring and decay**: What confidence models exist in knowledge management systems? How do systems like Wikidata, knowledge graphs, or recommendation engines handle confidence decay over time? What decay curves fit different knowledge categories (pricing vs. architectural patterns)?
18. **Confirmation bias in feedback loops**: How do reinforcement learning systems handle explore/exploit tradeoffs? What exploration budget percentage balances learning with efficiency? How do recommendation systems avoid filter bubbles -- same structural problem as routing bias?
19. **Contradiction resolution**: When two agents produce conflicting findings, what resolution strategies exist? Voting? Evidence weighting? Human escalation? How do collaborative knowledge bases (Wikipedia, Wikidata) handle contradictory edits?
20. **Validation pipeline for incoming data**: What gates should new findings pass before entering trusted memory? Independent corroboration? Source quality scoring? Automated fact-checking against external references?

## The Dispatch Feedback Loop

Shared memory's most powerful application is closing the loop between agent selection, execution, and improvement. The dispatcher doesn't just route tasks -- it learns which agent handles which task types best, based on accumulated evidence.

### How It Works

```text
1. Research agents scan the agent landscape on a recurring schedule
   -> Store: capabilities, pricing, benchmarks, model updates
   -> Memory type: "agent_profile"

2. Dispatcher classifies an incoming issue
   -> Query memory: "which agents performed best on Go test generation?"
   -> Query memory: "what's the current pricing for Claude vs GPT-4?"
   -> Select agent based on evidence, not configuration

3. Agent executes the task, produces a result (PR, comment, report)

4. Autolearn evaluates the outcome
   -> Quality: did CI pass first try? Were there lint violations?
   -> Cost: tokens consumed, Actions minutes, wall time
   -> Errors: what went wrong, was it agent-specific or task-specific?
   -> Store evaluation as "task_outcome" in shared memory

5. Next dispatch decision uses steps 1-4 as input
   -> Agent X scored 95% on Go test tasks but 60% on React components
   -> Agent Y is 3x cheaper but needs 2 iterations on average
   -> Agent Z consistently misses golangci-lint patterns
```

### What Gets Learned Over Time

- **Agent strengths**: "Copilot handles test expansion well (87% first-pass CI success) but struggles with multi-file refactors (42%)"
- **Error patterns**: "Ollama models consistently miss named return conventions -- add to instruction prompts"
- **Cost efficiency**: "For docs tasks, Copilot at $0 (Actions minutes) outperforms Claude at $0.12/task with equivalent quality"
- **Task classification**: "Issues with 3+ file changes should never route to Copilot -- historical failure rate is 71%"
- **Recovery patterns**: "When Agent X fails on task type Y, reassigning to Agent Z resolves 89% of the time"

### Error Prevention Through Institutional Memory

Every mistake becomes a permanent learning:

1. Agent produces a lint violation -> autolearn captures the pattern
2. Pattern is stored in shared memory with agent ID, task type, and error details
3. Next time a similar task is dispatched, the dispatcher either:
   - Routes to a different agent that doesn't make this mistake
   - Enriches the agent's prompt with the specific warning
   - Both -- route to a better agent AND include the warning

This is the "once found, always fix, never leave" principle applied to agent orchestration. The system prevents errors by remembering them, not by hoping agents read instruction files.

## Design Constraints

### Query Performance

Every dispatch decision queries shared memory. If dispatch happens dozens of times per sprint, and each dispatch requires agent profile lookups, historical performance queries, and error pattern checks, the storage layer must handle this without becoming a bottleneck.

Design implications:

- **Indexed access patterns** -- The most common queries are predictable: "agent performance by task type," "recent errors for agent X," "cost comparison across providers." These should be indexed, not full scans.
- **Materialized summaries** -- Don't compute "Copilot's Go test success rate" by scanning every historical outcome at query time. Maintain pre-aggregated summaries that update incrementally as new outcomes arrive.
- **Tiered storage** -- Hot data (recent outcomes, active agent profiles) in fast-access structures. Cold data (historical trends, archived findings) can be slower. The dispatcher only needs hot data for routing decisions.
- **Query budget** -- Define a latency budget per dispatch decision (e.g., < 50ms for all memory queries combined). If a query pattern exceeds this, it belongs in a pre-computed summary, not a live query.

The data model must be designed around access patterns, not just storage convenience. Schema decisions that optimize writes at the expense of reads will cripple the feedback loop.

### Data Integrity and Confirmation Bias

A self-learning system that trusts its own output uncritically will converge on wrong answers with high confidence. The feedback loop is powerful when it works, but it can corrupt shared memory with faulty logic and bad data disguised as solutions. Three categories of risk:

**1. Poisoned data -- bad findings entering memory**

An agent misdiagnoses a problem, stores the wrong conclusion, and future agents retrieve and reinforce it. Example: Agent records "SQLite doesn't support concurrent writes" (wrong -- WAL mode handles this), and subsequent agents avoid SQLite for concurrent workloads based on a false premise.

Mitigations:

- **Confidence tiers** -- New findings start as `provisional` (low confidence). They graduate to `verified` only after independent corroboration (a different agent or model reaches the same conclusion, or a human confirms). The dispatcher weights `verified` knowledge higher than `provisional`.
- **Source attribution** -- Every finding records which agent, which model, and what evidence. A finding from a capable model with cited sources ranks higher than an unsourced assertion from a less capable model.
- **Contradiction detection** -- When a new finding contradicts an existing one, don't silently overwrite. Flag the conflict, record both with evidence, and escalate for resolution (human review, or a dedicated validation agent).

**2. Stale data -- correct findings becoming wrong over time**

Agent performance changes as models are updated. Pricing changes. Libraries release breaking versions. A finding that was true six months ago may be dangerously wrong today.

Mitigations:

- **Confidence decay** -- Findings lose confidence over time unless refreshed. A 6-month-old performance benchmark carries less weight than a 1-week-old one. Decay rate varies by category: pricing data decays fast (weeks), architectural patterns decay slowly (months).
- **Recurring validation** -- Scheduled research tasks don't just discover new information -- they re-validate existing high-value findings. "Is it still true that Copilot struggles with multi-file refactors?" Run a test, update the record.
- **Staleness alerts** -- When the dispatcher relies on a finding older than its decay threshold, flag it. Don't block the decision, but log it so research tasks know what to re-validate.

**3. Confirmation bias -- the system reinforcing its own mistakes**

If the dispatcher routes Go test tasks away from Copilot based on early poor results, Copilot never gets a chance to prove it improved (model updates, better instructions). The system locks into suboptimal routing because it stopped collecting data.

Mitigations:

- **Exploration budget** -- Reserve a percentage of tasks (e.g., 10%) for deliberate exploration: route to an agent the data says is suboptimal, measure the result, update the model. This is the explore/exploit tradeoff from reinforcement learning.
- **Periodic re-evaluation** -- After N weeks or after a model update, force a re-test of routing assumptions. "We haven't sent a Go task to Copilot in 30 days. Run a controlled test."
- **External benchmarks** -- Don't rely solely on internal task outcomes. Recurring research tasks bring in external data (published benchmarks, community reports, release notes) that can challenge internal assumptions.
- **Human override with tracking** -- When a human overrides a routing decision ("use Claude for this, not Copilot"), record the override and the outcome. If human overrides consistently outperform the system, the routing model is wrong and needs recalibration.

These constraints are not optional safety features. They are structural requirements. A shared memory system without data integrity guarantees is worse than no shared memory at all -- it provides false confidence in unreliable information.

## Non-Goals (at this stage)

- Specific technology choices (SQLite vs. Postgres, which embedding model)
- Implementation timeline or phase assignment
- API surface design
- Performance requirements

These follow from the research phase, not precede it.

## Proposed Next Steps

1. **Research phase** -- Investigate the prior art and adjacent concepts listed above. Produce a findings document covering what exists, what works, and what patterns apply to Samverk's constraints (single binary, self-hosted-first, model-agnostic).
2. **Design phase** -- Based on research findings, produce a concrete design doc with API surface, data model, and implementation plan.
3. **ADR** -- Record the architectural decision (storage format, embedding strategy, scope boundaries).
4. **Implementation** -- Phase TBD, after design approval.

## References

- [Nate Jones "Open Brain" concept](attached: Mar 3, 2026 0707.txt) -- Initial inspiration
- [Open Questions: Multi-Session Coordination](open-questions.md#multi-session-coordination) -- Related problem space
- [Open Questions: Project Lifecycle](open-questions.md#project-lifecycle) -- Research-to-execution handoff, kill rationale preservation
- [ADR-012: Git Issues as Protocol](decisions/ADR-012-git-issues-protocol.md) -- Current communication layer
