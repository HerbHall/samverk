# Samverk

> *Samverk* — Old Norse/Icelandic: "cooperative work" (sam = together, verk = work/deed)

Samverk is a multi-agent AI development framework designed to give solo developers and small teams the organizational capability of a full software company — powered by coordinated AI agents.

## Vision

Most AI frameworks are built for AI engineers. Samverk is built for builders.

The goal is a system where you describe what you want to build, and a structured hierarchy of specialized AI agents — organized like a real company — researches, plans, produces, and validates the work across every domain: code, documentation, legal considerations, QA, and more.

**Think less "prompt engineering" and more "I have a team now."**

## Status

🟡 Early concept / research phase

This repository captures the initial design thinking, competitive research, and architectural decisions made during the concept development phase. Active development has not yet begun.

## Core Concept

Samverk organizes AI agents into a hierarchical structure mirroring how a real company operates:

```
User Input
    └── Orchestration Layer (intake, clarification, planning)
            └── Division Layer (Research | Production | Legal | QA)
                    └── Department Layer (domain-specific breakdown)
                            └── Agent Layer (narrow-scope execution)

QC runs as a parallel mirror hierarchy validating output at every level.
```

See [Architecture](docs/architecture.md) for full detail.

## Key Differentiators

- **Business-owner mindset** — not AI-engineer mindset
- **Parallel QC structure** — validation built into the hierarchy, not bolted on
- **Cross-provider validation** — use Claude to validate GPT-4 output, or vice versa (planned V2)
- **Solo-developer scale** — designed for one person to run a "company"

## Target Users

- Solo developers / indie hackers
- Small teams without organizational bandwidth
- Builders who want AI to handle the parts they're least skilled at

## Documentation

- [Concept & Problem Space](docs/concept.md)
- [Architecture](docs/architecture.md)
- [Competitive Landscape](docs/competitive.md)
- [Naming & Background](docs/naming.md)

## Related Projects

- [Subnetree](https://github.com/HerbHall/subnetree) — Network monitoring tool; intended proof-of-concept project to be built *using* Samverk once the framework exists
- [Devkit](https://github.com/HerbHall) — Windows development environment setup; the infrastructure layer Samverk runs on top of

## License

TBD — likely BSL 1.1 / Apache 2.0 dual license (consistent with Subnetree approach)

---

*Started: February 2026 | Author: Herb Hall*
