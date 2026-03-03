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
