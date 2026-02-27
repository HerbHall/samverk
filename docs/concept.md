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

## What This Is Not

- Not a synchronous AI coding assistant
- Not a general-purpose chatbot wrapper
- Not another LangChain clone
- Not an enterprise orchestration platform
- Not trying to replace AI providers (Claude, GPT-4, etc.)

Samverk lives at the **application layer**, built on top of existing AI infrastructure -- not competing with it.
