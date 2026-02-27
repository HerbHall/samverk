# Architecture Decision Records

Decisions are recorded as ADR files in this directory.

| ADR | Title | Status |
|-----|-------|--------|
| [001](ADR-001-project-name.md) | Project Name = Samverk | Accepted |
| [002](ADR-002-application-layer.md) | Application Layer, Not Infrastructure | Accepted |
| [003](ADR-003-claude-only-v1.md) | Claude-Only for V1 | Partially Superseded by ADR-008 |
| [004](ADR-004-custom-orchestration.md) | Custom Orchestration | Accepted |
| [005](ADR-005-go-language.md) | Go as Implementation Language | Accepted |
| [006](ADR-006-async-first.md) | Async-First Architecture | Accepted |
| [007](ADR-007-hybrid-local-cloud.md) | Hybrid Local/Cloud Agent Model | Accepted |
| [008](ADR-008-multi-model-default.md) | Multi-Model by Default | Accepted |
| [009](ADR-009-device-flexibility.md) | Device Flexibility Is Non-Negotiable | Accepted |
| [010](ADR-010-success-metric.md) | The Right Success Metric | Accepted |
| [011](ADR-011-chat-as-interface.md) | Chat as Primary Interface | Accepted |
| [012](ADR-012-git-issues-protocol.md) | Git Issues as Agent Communication Protocol | Accepted |
| [013](ADR-013-forge-abstraction.md) | Abstract Git Forge Behind Interface | Accepted |
| [014](ADR-014-dispatcher-agent.md) | Dispatcher Agent as Always-Running Process | Accepted |

## Open Decisions

- Depth calibration -- who decides how deep the agent tree goes?
- "Good enough" threshold -- when is QC satisfied?
- Cost management -- token budget tracking and enforcement
- Licensing -- BSL 1.1 / Apache 2.0 dual or different?
- Domain selection -- samverk.io vs samverk.ai
- V1 scope -- minimum viable framework
- Hosting model -- self-hosted, hosted service, or both?
