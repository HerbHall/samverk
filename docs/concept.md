# Concept and Problem Space

## The Real Problem

The problem isn't capability. It's time and momentum.

Solo dev projects die for predictable reasons:

- **Life interrupts.** You have a job, a family, obligations. The project goes cold for a week, then a month, then forever.
- **Restarting is hard.** After a break, getting back into context is psychologically expensive. Many projects never recover from a two-week gap.
- **Getting stuck kills motivation.** Hit something outside your expertise -- infrastructure, security, frontend design -- and the project stalls.
- **There's no one to hand off to.** You can't delegate the boring parts, the parts you're bad at, or the parts that just need time.

The result: projects that should take 12-18 months take 5-10 years, or never ship at all.

## The Insight

Every AI development tool in 2026 is synchronous. You sit at your keyboard, prompt, wait, review, repeat. This works for full-time developers. It doesn't work for someone with 10-15 minutes between obligations.

The async gap is the opportunity. What if the project kept building while you were away?

## Target User

**Primary: The hobbyist developer with a full life.**

- Has a day job (example: a Controls Technician at a university)
- Has family and obligations
- Has genuine programming interest -- it's a hobby, not a career
- Has an idea they want to build
- Has maybe 10-15 minutes at a time to focus on it
- Has watched their own side projects die from loss of momentum, not loss of skill

**Not targeting (initially):**

- Enterprise teams with dedicated dev staffing
- AI engineers who want low-level orchestration control
- Full-time developers who can sit with synchronous tools all day

## The Value Proposition

Samverk is an async background development engine. It works while the user lives their life.

The check-in model:

1. User opens Samverk on whatever device is available
2. Sees a digest: what's done, what's in progress, what needs input
3. Spends 5-15 minutes answering questions and providing direction
4. Closes the app and goes back to their life
5. Samverk keeps working

**The success metric is project completion rate and time-to-ship.** Not response speed, not API cost efficiency, not code quality scores. Everything else is a means to that end.

## Time vs. Cost

The latency of an agent spinning up or completing a task is irrelevant to this user. If an agent takes 5 minutes to do something that would take the user 4 hours, that's the entire value -- the user isn't watching it work.

Time comparisons should always be against human effort, not against other AI tools.

Cost comparisons should never be against zero. The right comparisons:

- A freelance developer ($50-150/hr)
- A year of the user's own evenings and weekends
- The opportunity cost of a project that never ships

A $50/month Samverk bill that ships a product in 12 months instead of never is the cheapest employee the user will ever have.

## The Personal Connection

The name Samverk comes from Icelandic/Old Norse -- "cooperative work." The founder lived in Iceland, and the name carries personal meaning while being a perfect description of what the framework does: many agents working together toward a shared goal.

## Full Lifecycle, Not Just Execution

Samverk doesn't just build projects -- it helps decide which projects to build in the first place. The lifecycle starts at the moment an idea enters the user's head and ends when the product ships:

1. **Idea Intake** -- capture a half-baked thought from any device, any format
2. **Research & Feasibility** -- competitive analysis, technical assessment, market gap identification
3. **Go/No-Go** -- evidence-based decision to proceed, pivot, or kill
4. **Requirements & Architecture** -- translate approved concepts into buildable specs
5. **Scaffolding** -- create the repo, issues, and project infrastructure
6. **Execution** -- the agent team builds it (dispatcher, QC, specialist agents)
7. **Delivery** -- publish, deploy, announce

The user provides ideas, creative direction, and approval at gates. Samverk provides the discipline, research, and legwork that a solo developer doesn't have time to manage. See [Project Lifecycle](project-lifecycle.md) for the full specification.

## Long-Term Vision

Samverk is not a finished product with a fixed scope. It is a living, growing development house -- a personal mini software company that evolves as needs and capabilities evolve.

### DevKit Incorporation

DevKit currently exists as a separate project providing cross-session learning (autolearn), CI templates, rules governance, project scaffolding, and subagent coordination patterns. Long-term, DevKit's capabilities merge into Samverk. The result: one ecosystem that manages its own tooling, its own rules, and its own improvement loop. Samverk becomes self-improving -- the autolearn patterns that help agents avoid mistakes become part of the shared memory that all agents can query.

### Beyond Software

The scope is intentionally open-ended. The same lifecycle (idea -> research -> validation -> build -> deliver -> maintain) applies to physical products, hardware prototypes, and workshop projects -- not just code. Modular architecture is an investment: each capability (shared memory, provider registry, cost tracking, agent pool) is a building block that can be composed for use cases that don't exist yet.

### Modularity as Strategy

Every component should be:

- **Independently useful** -- Shared memory works without the dispatcher. The provider registry works without the agent pool. Any piece can be used in contexts we haven't imagined yet.
- **Replaceable** -- If a better embedding model appears, swap it. If SQLite hits a wall, the store interface abstracts the backend. No component should be load-bearing in a way that prevents growth.
- **Composable** -- New capabilities plug in without rewriting existing ones. A future "hardware project tracker" module should be able to use the same shared memory, the same provider registry, and the same cost tracking.

### The Gitea-to-GitHub Flow

Projects live in Gitea (self-hosted, private, development) until ready for public release, then move or mirror to GitHub. Samverk manages both sides through the forge abstraction. This isn't a temporary workaround -- it's the intended workflow. Private iteration with full autonomy, public release when ready.

## What This Is Not

- Not a synchronous AI coding assistant
- Not a general-purpose chatbot wrapper
- Not another LangChain clone
- Not an enterprise orchestration platform
- Not trying to replace AI providers (Claude, GPT-4, etc.)

Samverk lives at the **application layer**, built on top of existing AI infrastructure -- not competing with it.
