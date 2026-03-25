# Cost Model

## Tiered by Hardware and Subscriptions

| Tier | Setup | Result |
|------|-------|--------|
| 1 | Local models only | Free after hardware, slowest, still ships |
| 2 | One cloud subscription + local | Good balance of cost and speed |
| 3 | Multiple cloud subscriptions + local | Cloud for complexity, local for volume |
| 4 | Tier 3 + capable local GPU | Maximum throughput, minimum cost per task |

## Key Principles

- **No one gets locked out.** Every tier produces a working result.
- **More investment = faster results, not different features.** Tier 1 and Tier 4 build the same project -- Tier 4 just finishes sooner.
- **Local handles volume.** High-volume low-complexity work: code generation, tests, formatting, schema validation, boilerplate, documentation.
- **Cloud handles complexity.** Low-volume high-complexity work: architecture decisions, ambiguity resolution, QC arbitration, cross-domain reasoning.
- **Multiple subscriptions serve double duty.** Cost failover (never blocked by one provider's billing) + quality diversity (different models catch different bugs).

## The Right Cost Comparison

Never compare Samverk's cost to zero. The right comparisons:

| Alternative | Cost | Outcome |
|-------------|------|---------|
| Freelance developer | $50-150/hr | Fast but expensive, requires management |
| User's own time (evenings/weekends) | 5-10 years of fragmented effort | Usually never ships |
| Project abandoned | $0 | Nothing ships |
| **Samverk (Tier 2)** | **~$50/month** | **Ships in 12-18 months** |

A $50/month bill that ships a product in 12 months instead of never is the cheapest employee the user will ever have.

## Work Distribution by Tier

### Tier 1: Local Only

- All work runs on local containerized models (Ollama in Docker)
- Suitable for simpler projects or users with strong local hardware
- Slowest but completely free after initial hardware investment
- QC quality limited by local model capability

### Tier 2: One Cloud Provider + Local

- Cloud handles orchestration, complex reasoning, QC arbitration
- Local handles code generation, testing, formatting
- Best balance for most users
- Estimated $20-80/month depending on project complexity and activity

### Tier 3: Multiple Cloud Providers + Local

- Primary cloud provider for complex work
- Secondary providers for failover and cross-model validation
- Local for volume work
- Estimated $50-150/month

### Tier 4: Full Stack

- Multiple cloud providers + capable local GPU (RTX 3060+)
- Local models handle mid-tier reasoning (not just execution)
- Cloud reserved for highest-complexity decisions
- Maximum throughput with minimum per-task cost
- Estimated $50-100/month (local absorbs most volume)

## Provider Token Policy

### MAX Plan First (Design Standard)

All Claude providers in production use `type: claude-cli`, which spawns the Claude CLI
process authenticated via OAuth token (MAX plan subscription). The CLI explicitly strips
`ANTHROPIC_API_KEY` from the subprocess environment to prevent accidental API credit
consumption.

**Rules:**

1. **Claude CLI (MAX plan) is the only authorized Claude provider type in production.**
   The `type: claude` (API) provider exists in code but must not be configured in
   `providers.yaml` without an approved cost plan.
2. **API keys are disabled by default.** `ANTHROPIC_API_KEY` and `OPENAI_API_KEY` are
   commented out in the production env file with intent documentation. Re-enabling
   requires project owner approval and a documented cost justification.
3. **Ollama (free) handles volume.** Code-gen, docs, research, QC triage -- anything
   the local models can do at acceptable quality runs on Ollama first.
4. **Claude CLI handles complexity.** Tasks that exceed Ollama capability (complex
   architecture, multi-file refactors, cross-system reasoning) route to Claude CLI
   via the `complex` chain.
5. **MAX plan tokens are a budget.** They are cheaper than API tokens but not free.
   Use the lowest-cost provider that produces acceptable quality for each task type.

### Priority Tree

```text
Quality > Cost > Speed
```

Speed is only a factor when delay causes further harm (e.g., a broken pipeline
blocking all other work). Otherwise, slow and correct beats fast and wasteful.

### Future API Use

API credits (Anthropic, OpenAI) may be enabled in the future if:

- The project generates revenue that covers the cost
- A specific feature requires API-only capabilities (e.g., streaming, function calling
  patterns not available via CLI)
- A documented cost plan is approved showing expected spend and ROI

Until then, all AI provider spend is covered by subscriptions (MAX plan) and
local hardware (Ollama GPU fleet).

## Cost Transparency

Cost must be a first-class concern in the UI:

- Real-time burn rate visible in check-in digest
- Per-task cost attribution (how much did this feature cost?)
- Budget alerts before limits are hit, not after
- Historical trends (is this project getting more or less expensive?)
- Cost breakdown by provider and tier
