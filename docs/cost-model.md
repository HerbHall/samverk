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

## Cost Transparency

Cost must be a first-class concern in the UI:

- Real-time burn rate visible in check-in digest
- Per-task cost attribution (how much did this feature cost?)
- Budget alerts before limits are hit, not after
- Historical trends (is this project getting more or less expensive?)
- Cost breakdown by provider and tier
