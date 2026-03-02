# Market Validation

## Verdict: GO -- Build for Me First

Samverk is validated by the founder's own need. The target user is the founder. If Samverk helps ship projects that would otherwise stall, it has achieved its purpose. External users are a bonus, not a requirement.

This is not a market-driven product. It is a tool-driven product that may find a market.

## Key Findings

### The Target User Exists (Sample Size: 1)

The concept doc's target user -- hobbyist developer with a full-time job, family, 10-15 min/day, projects that stall from lost momentum -- is the founder. The pain is real, lived, and ongoing.

Validation does not require proving that millions of developers share this pain. It requires proving that the tool solves the pain for its primary user.

### Competitive Positioning is Misframed

Samverk is **not** competing with AI coding tools (Claude Code, Cursor, Copilot). It is an orchestration layer that uses them.

| Layer | Examples | Samverk's Relationship |
|-------|----------|----------------------|
| AI providers | Anthropic, OpenAI, Google, Ollama | Samverk consumes their APIs |
| AI coding tools | Claude Code, Cursor, Copilot | Samverk orchestrates tools like these |
| Dev orchestration | **Samverk** | The structured environment |
| Developer | The user | Provides ideas, direction, approval |

Everything incumbents build (better models, better code tools, async features) enhances Samverk's capabilities. They are suppliers, not competitors. The moat is not "better AI coding" -- it is the full lifecycle orchestration from idea intake through delivery, running on user-controlled infrastructure.

### Trust is the Real Barrier

Confidence in user trust is moderate, not high. The founder is unusually comfortable with AI. Most hobbyist developers may need:

- Transparent audit trails (what did the AI do while I wasn't watching?)
- Graduated autonomy (start conservative, earn trust over time)
- Easy rollback (undo anything the AI did)
- Clear boundaries (never touch production, never merge without approval)

The tiered autonomy model (ADR-015) and intent verification protocol (ADR-021) address this directly. The check-in digest must make trust tangible -- the user should never feel anxious about what happened overnight.

### The "Building IS the Hobby" Problem

The user motivation is **both** -- they enjoy building but hate getting stuck on parts outside their expertise. Samverk's sweet spot is:

- Handling the unfun parts (infrastructure, boilerplate, CI setup, dependency management)
- Handling the "stuck" parts (areas outside the user's expertise)
- Preserving the fun parts (architecture decisions, creative features, core logic)

This means Samverk should not be positioned as "AI builds your project." It should be positioned as "AI handles the parts you don't want to, so you can focus on the parts you do."

## Pricing Analysis

### Current State

Pricing model is undecided. The $50/month figure from the concept doc was a placeholder comparison against freelancer rates, not a committed price point.

### Models to Evaluate Before Release

| Model | Pros | Cons | Fit |
|-------|------|------|-----|
| Subscription ($X/month) | Predictable revenue, simple | Barrier to entry, pays even when idle | Good for committed users |
| Usage-based (per task/token) | Low barrier, scales with use | Unpredictable bills, complex metering | Good for casual users |
| Freemium (free local + paid cloud) | Maximum adoption, natural upgrade path | Free tier may be sufficient for many | Best for dogfooding phase |
| One-time purchase | Simple, no recurring commitment | No ongoing revenue, hard to sustain | Poor fit for SaaS-like product |

**Recommendation for alpha:** No pricing. Samverk is a personal tool first. Pricing decisions come after validating that the tool works, not before.

### Cost Floor

Regardless of pricing model, the user bears AI provider costs directly:

- Tier 1 (Ollama only): $0/month after hardware
- Tier 2 (one cloud sub): ~$20-50/month in API costs
- Tier 3 (multiple clouds): ~$50-150/month in API costs

Samverk's price (if any) sits on top of these costs. The total cost must remain below the "freelancer threshold" ($50-150/hr) to feel like good value.

## Market Sizing

### Bottom-Up Estimate

Not attempting a rigorous TAM calculation. Instead, identifying the user profile:

- GitHub users with repos inactive >6 months: millions (but most don't care)
- Developers who actively discuss abandoned side projects (Reddit, HN): thousands
- Developers who would pay for async AI development: unknown, likely hundreds to low thousands
- Developers who would self-host an AI orchestration engine: subset of above

The honest answer: the addressable market is small and unproven. This is fine because the minimum viable audience is one person (the founder).

### Analogous Markets

| Market | Product | User Profile | Validation |
|--------|---------|-------------|------------|
| Automated investing | Wealthfront, Betterment | Time-poor professionals | $500B+ AUM proves "delegate to automation" works |
| Meal kits | HelloFresh, Blue Apron | Busy families | Mixed -- high churn, cost sensitivity |
| AI writing tools | Jasper, Copy.ai | Content creators | Validated but commoditized fast |
| Home automation | Home Assistant | Self-hosted tech enthusiasts | 50k+ active users, strong self-hosted community |

The closest analog is **Home Assistant**: a self-hosted tool for enthusiasts who want control, built by a small team, with a passionate niche community. Samverk's path could follow a similar trajectory.

## Competitive Threat Assessment

### Why Incumbents Are Not Threats

Incumbents (Anthropic, GitHub, OpenAI) build AI coding tools. Samverk is not an AI coding tool. It is a structured development environment that orchestrates AI coding tools.

If Claude Code adds "background tasks":

- Samverk uses that feature as a better agent runtime
- The lifecycle (idea -> research -> feasibility -> scaffold -> build -> deploy) remains Samverk's domain
- Self-hosted control (your Gitea, your Ollama, your data) remains Samverk's domain

**Samverk benefits when AI tools get better.** This is a symbiotic relationship, not a competitive one.

### What Could Actually Threaten Samverk

| Threat | Likelihood | Impact | Mitigation |
|--------|-----------|--------|------------|
| Someone builds "async AI project manager" | Low (niche) | High | First-mover, self-hosted, full lifecycle |
| AI tools become so good that 10 min/day is enough without orchestration | Medium | High | Samverk's value shifts to lifecycle management, not just coding |
| Target user doesn't exist at scale | Medium | Low (founder is the user) | Dogfood-first approach absorbs this risk |
| AI provider costs make it unaffordable | Low (costs trending down) | Medium | Local-first tier, cost controls (ADR-025) |

## User Interview Plan

Not required for GO decision (founder is the user), but recommended before public release:

### Questions for Potential Users

1. Tell me about a side project that stalled. What caused it?
2. If an AI could work on your project while you slept, what would you worry about?
3. How much time per day could you spend reviewing AI-generated work?
4. Would you prefer to approve every change, or set rules and let it run?
5. What's the maximum you'd pay per month for a tool that ships your side projects?

### Where to Find Them

- Reddit: r/sideproject, r/programming, r/selfhosted
- Hacker News: "Show HN" for side project tools, "Ask HN" about abandoned projects
- Indie Hackers: community of solo developers building products
- Docker Hub / awesome-selfhosted: users already comfortable with self-hosting

### When to Conduct

After alpha is functional and the founder has used it to ship at least one project. Real usage data is more convincing than hypothetical interviews.

## Decisions Captured

| Question | Answer |
|----------|--------|
| Pricing model | Undecided -- evaluate before release |
| Competitive framing | Orchestration layer, not AI tool. Incumbents are suppliers. |
| Trust assumption | Moderate confidence. Design for trust-building (audit, autonomy tiers, rollback). |
| User motivation | Both -- enjoys building but hates getting stuck. Handle the unfun parts. |
| Minimum viable audience | Just the founder. External users are a bonus. |
| Incumbent response | Full lifecycle + self-hosted control. Two-axis differentiation. |
| Go/no-go | GO -- build for me first. Dogfooding is the validation. |

## ADR-029: Dogfood-First Market Validation

### Decision

Validate Samverk by using it to build real projects (dogfooding). Defer external market validation until the tool is functional and has proven its value for the founder.

### Context

Traditional market validation (user interviews, surveys, landing pages) is appropriate for venture-backed startups with burn rates. Samverk is a personal project that may become a product. The founder is the target user.

### Options Considered

1. **User interviews first** -- validate demand before building
2. **Landing page + waitlist** -- test interest with minimal investment
3. **Dogfood first (chosen)** -- build it, use it, share results later
4. **Build in public** -- develop openly on GitHub, attract early users

### Consequences

**Positive:**

- No premature optimization for a market that may not exist
- Real usage data beats hypothetical survey responses
- The founder gets a useful tool regardless of market outcome
- No pressure to ship before the tool is ready

**Negative:**

- Risk of building features nobody else wants (founder bias)
- No external feedback loop during development
- May miss market timing if competitors ship first
- Could build too much before discovering misalignment with broader users

### Mitigation

Conduct lightweight external validation (Reddit posts, HN discussions) after alpha ships and the founder has concrete usage stories to share. "I used this to ship X project in Y months" is more compelling than "imagine if you could..."
