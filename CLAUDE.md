# Samverk

Async background development engine -- keeps your project building while you live your life.

## Quick Start

```bash
# No build steps yet -- concept phase
# Future:
make install   # Install dependencies
make build     # Build the project
make test      # Run tests
make lint      # Run linters
make run       # Start the system
```

## Project Structure

```text
docs/
  architecture.md       # Hybrid local/cloud agent hierarchy
  competitive.md        # Market landscape and positioning
  concept.md            # Problem space, target user, value proposition
  cost-model.md         # Tiered cost model and comparisons
  user-interface.md     # Check-in model and device flexibility spec
  open-questions.md     # Unresolved design and business questions
  naming.md             # Name research and background
  decisions/            # Architecture Decision Records (ADR format)
    README.md           # Index of all ADRs + open decisions
    ADR-NNN-title.md    # Individual decision records (10 so far)
```

## Tech Stack

- **Language**: Go (orchestrator), local models via Ollama in Docker
- **AI Providers**: Multi-model -- Claude (primary), GPT-4, Gemini, local fallback
- **Testing**: TBD
- **Build**: Make
- **CI/CD**: GitHub Actions

## Development Workflow

- Branch-per-issue: `feature/issue-NNN-desc`, `fix/issue-NNN-desc`
- Conventional commits: `feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`
- Co-author tag: `Co-Authored-By: Claude <noreply@anthropic.com>`
- Never commit directly to `main` -- always PR
- Explore -> Plan -> Code -> Commit

## Code Style

- Follow language-specific idioms (Go: gofmt, Python: ruff)
- No over-engineering -- implement only what's needed now
- Comments only where logic isn't self-evident
- Validate at system boundaries, not internal calls

## Known Constraints

- Early concept phase -- no runnable code yet
- Async-first architecture (not synchronous tooling)
- Hybrid local/cloud agent model
- Target audience: hobbyist devs with limited time, not enterprise teams
- Success metric: project completion rate, not response speed

## Key Decisions

- Async-first check-in model ([ADR-006](docs/decisions/ADR-006-async-first.md))
- Hybrid local/cloud agents ([ADR-007](docs/decisions/ADR-007-hybrid-local-cloud.md))
- Multi-model from day one ([ADR-008](docs/decisions/ADR-008-multi-model-default.md))
- Device flexibility non-negotiable ([ADR-009](docs/decisions/ADR-009-device-flexibility.md))
- Ship rate is the success metric ([ADR-010](docs/decisions/ADR-010-success-metric.md))

## References

- [Architecture](docs/architecture.md)
- [Concept](docs/concept.md)
- [Competitive Landscape](docs/competitive.md)
- [Cost Model](docs/cost-model.md)
- [User Interface](docs/user-interface.md)
- [Open Questions](docs/open-questions.md)
- [Naming](docs/naming.md)
- [Decision Records](docs/decisions/)
