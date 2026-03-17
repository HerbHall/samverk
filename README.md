> **This repository has moved to Gitea.** Active development continues at [gitea.herbhall.net/samverk/samverk](https://gitea.herbhall.net/samverk/samverk). This GitHub repository is a read-only archive. Issues and PRs are disabled.

---

# Samverk

> *Samverk* -- Old Norse/Icelandic: "cooperative work" (sam = together, verk = work/deed)

**Your project keeps building while you're at work, with the kids, or asleep. You just need a few minutes a day to keep it moving.**

Samverk is an async background development engine. It coordinates AI agents to keep your project moving forward while you live your life -- then surfaces a digest when you have a few minutes to check in.

## Why

Every AI dev tool today is synchronous. You sit at your keyboard, prompt, wait, review, repeat. That works if coding is your full-time job. It doesn't work if you have 10-15 minutes between obligations.

Solo dev projects typically take 5-10 years or never ship at all. Not because of lack of skill -- because of lack of time and momentum. Life interrupts, the project goes cold, restarting is psychologically hard, and there's no one to hand work off to.

**Samverk's metric isn't faster responses or cheaper API calls. It's: did the project ship.**

A project that would take a solo dev 5-10 years (or never ship) ships in 12-18 months.

## How It Works

1. You describe what you want to build
2. Samverk's agent hierarchy researches, plans, builds, and validates -- continuously
3. When you have a few minutes, you check in from any device
4. Samverk shows you what's done, what's in progress, and what needs your input
5. You provide direction, answer questions, approve decisions
6. You close the app. Samverk keeps working.

## Architecture

Agents are organized in a hybrid local/cloud model:

```text
Cloud (Claude, GPT-4, etc.)     Complex reasoning, orchestration, QC arbitration
Mid-tier (API or large local)   Task decomposition, planning
Local (containerized, GPU)      Code generation, testing, formatting, docs
```

Multi-model by default. Provider failover on credit exhaustion. Different models catch different bugs -- diversity improves quality.

See [Architecture](docs/architecture.md) for full detail.

## Key Differentiators

- **Async-first** -- works while you're away, not while you're watching
- **Check-in model** -- 5-15 minute sessions from any device
- **Hybrid local/cloud** -- local handles volume, cloud handles complexity
- **Multi-model failover** -- cost management and quality diversity in one feature
- **Parallel QC** -- validation built into the hierarchy, not bolted on
- **No lock-in** -- every hardware tier produces a working result

## Target Users

The hobbyist developer with a full life. Has a job, a family, genuine programming interest, and an idea that keeps dying from loss of momentum.

Not targeting: enterprise teams, AI engineers, or full-time developers who can sit with tools all day.

## Documentation

- [Concept and Problem Space](docs/concept.md)
- [Architecture](docs/architecture.md)
- [Tech Stack](docs/tech-stack.md)
- [Communication Protocol](docs/communication-protocol.md)
- [Dispatcher Design](docs/dispatcher-design.md)
- [Autonomy Model](docs/autonomy-model.md)
- [User Profile](docs/user-profile.md)
- [System Requirements](docs/system-requirements.md)
- [Competitive Landscape](docs/competitive.md)
- [Cost Model](docs/cost-model.md)
- [User Interface](docs/user-interface.md)
- [Open Questions](docs/open-questions.md)
- [Naming and Background](docs/naming.md)
- [Decision Records](docs/decisions/)

## Status

**Phase 2 complete.** Core infrastructure operational with MCP server, check-in digest, and end-to-end validation (GO decision). 118 tests passing across 11 packages.

**Phase 1** delivered: dispatcher routing, frontmatter parser, profile store, SQLite persistence, GitHub and Gitea forge adapters, Ollama provider, autonomy policy engine.

**Phase 2** delivered: HTTP server with health endpoint, MCP handler (get_digest + get_cost_summary tools via Streamable HTTP), cost tracking adapter, check-in digest prototype, label taxonomy (47 labels), front-end agent system prompt design.

Next: Phase 3 -- wire remaining forge ops as MCP tools, session recording, Claude Desktop integration, authentication middleware, web dashboard.

## Related Projects

- [Subnetree](https://github.com/HerbHall/subnetree) -- Network monitoring tool; intended dogfood project to be built *using* Samverk
- [Devkit](https://github.com/HerbHall/devkit) -- Windows development environment setup; the infrastructure layer Samverk runs on top of

## License

TBD -- likely BSL 1.1 / Apache 2.0 dual license (consistent with Subnetree approach)

---

*Started: February 2026 | Author: Herb Hall*
