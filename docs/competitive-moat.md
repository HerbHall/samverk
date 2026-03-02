# Competitive Moat

## Reframing: There Is No Competition

Issue #74 asked "what if incumbents add async features?" The owner's answer during market validation (#70) reframed the question entirely:

> "I don't view them as competition. Everything they build enhances our capabilities and makes Samverk more useful. We are providing a structured dev environment -- they provide the tools we use to get the work done."

Samverk is an **orchestration layer**, not an AI coding tool. It sits above the tools that incumbents build. When Claude Code gets better, Samverk gets better. When Copilot adds background tasks, Samverk can use that as an agent runtime.

## The Layer Model

```text
┌──────────────────────────────────┐
│           User (hobbyist)        │  Provides ideas, direction, approval
├──────────────────────────────────┤
│         Samverk (orchestration)  │  Lifecycle, routing, QC, cost, digest
├──────────────────────────────────┤
│     AI Tools (execution)         │  Claude Code, Copilot, Cursor, Devin
├──────────────────────────────────┤
│     AI Providers (models)        │  Anthropic, OpenAI, Google, Ollama
└──────────────────────────────────┘
```

Samverk competes at the orchestration layer. Incumbents operate at the execution and model layers. These are complementary, not competitive.

## Why This Reframing Holds

### Incumbents Build Hammers, Samverk Runs the Construction Company

Claude Code can write code. It cannot:

- Decide which project to work on based on user priorities
- Route tasks to the cheapest capable model
- Manage a multi-week project lifecycle (idea -> research -> scaffold -> build -> deploy)
- Enforce budget caps across providers
- Generate a digest summarizing 3 days of autonomous work across 4 projects
- Run on user-controlled infrastructure with user-chosen forges

Even if Claude Code adds "background mode," it becomes a better agent runtime for Samverk -- not a replacement for it.

### The Self-Hosted Dimension

Incumbents are cloud-first by business model. They profit from hosting. Samverk is self-hosted-first by design choice (ADR-019). This is not a feature gap to close -- it is a philosophical divergence:

| Dimension | Incumbents | Samverk |
|-----------|-----------|---------|
| Infrastructure | Their servers | Your servers |
| Data | Transits through their API | You control transit (ADR-026) |
| Forge | GitHub only | Any forge (GitHub, Gitea, GitLab) |
| Models | Their model only | Any provider (ADR-008) |
| Cost | Hidden in subscription | Per-task attribution (ADR-025) |
| Availability | Depends on their uptime | Runs on your hardware |

### The Lifecycle Dimension

Incumbents solve "write code faster." Samverk solves "ship projects that would otherwise never ship." The lifecycle spans seven phases (ADR-022):

1. Idea intake
2. Research and feasibility
3. Go/no-go decision
4. Requirements and architecture
5. Scaffolding
6. Execution (this is where incumbents operate)
7. Delivery

Even a perfect AI coding tool only covers phase 6. Samverk covers phases 1-7.

## What Could Actually Threaten Samverk

The real threats are not incumbents adding features. They are:

| Threat | Likelihood | Mitigation |
|--------|-----------|------------|
| Someone builds a full-lifecycle orchestrator (same layer) | Low -- niche market, complex to build | First-mover, self-hosted, forge-agnostic |
| AI tools become so good that orchestration is unnecessary | Low -- complexity grows with capability | Samverk's value shifts to lifecycle management |
| Target user doesn't exist at scale | Irrelevant -- founder is the user | Dogfood-first (ADR-029) |
| Orchestration becomes a commodity feature of IDEs | Medium -- 2-3 year horizon | Community, ecosystem, self-hosted moat |

## Durable Advantages

These advantages persist regardless of what incumbents do:

1. **Full lifecycle** -- idea to delivery, not just code generation
2. **Self-hosted** -- runs on user infrastructure, no vendor lock-in
3. **Forge-agnostic** -- GitHub, Gitea, GitLab (and future forges)
4. **Model-agnostic** -- any AI provider, including local-only
5. **Cost-transparent** -- per-task attribution, budget controls, no hidden fees
6. **Async-native** -- designed for users with 10-15 min/day, not full-time developers

## Conclusion

The competitive moat question was premature. Samverk is not competing with AI tools -- it is an orchestration layer that benefits from their improvement. The original issue framing ("what if incumbents add async?") assumed Samverk and AI tools are in the same market. They are not.

The real competitive question -- "will someone build a full-lifecycle orchestrator before Samverk ships?" -- is less urgent because: (a) the market is niche, (b) the founder is the user, and (c) the product's value is validated by dogfooding, not by market share.
