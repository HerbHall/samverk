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
| [015](ADR-015-three-tier-autonomy.md) | Three-Tier Autonomy Model | Accepted |
| [016](ADR-016-user-profile.md) | User Profile as First-Class Concept | Accepted |
| [017](ADR-017-devkit-reference.md) | Devkit as Reference Implementation | Accepted |
| [018](ADR-018-release-versioning.md) | Release Versioning and V1 Scope | Accepted |
| [019](ADR-019-self-hosted-first.md) | Self-Hosted-First Development | Accepted |
| [020](ADR-020-web-dashboard.md) | Web Dashboard for Operations | Accepted |
| [021](ADR-021-intent-verification.md) | Intent Verification Protocol | Accepted |
| [022](ADR-022-full-project-lifecycle.md) | Full Project Lifecycle — Idea to Delivery | Accepted |
| [023](ADR-023-per-project-repos.md) | Per-Project Repos with Coordination Layer | Accepted |
| [027](ADR-027-failure-recovery.md) | Failure Recovery and State Reconciliation | Proposed |
| [030](ADR-030-cross-model-qa.md) | Cross-Model QA Validation | Proposed |
| [031](ADR-031-dual-forge-operational-model.md) | Single-Forge-Per-Project Model | Revised |
| [032](ADR-032-adaptive-worker-scaling.md) | Adaptive Worker Scaling | Accepted |
| [033](ADR-033-pc-agent-worker-node.md) | PC Agent Worker Node with Isolated Workspaces | Accepted |

## Open Decisions

- Depth calibration — who decides how deep the agent tree goes?
- "Good enough" threshold — when is QC satisfied?
- Cost management — token budget tracking and enforcement
- Licensing — BSL 1.1 / Apache 2.0 dual or different?
- Domain selection — samverk.io vs samverk.ai
- Server platform selection — Unraid, Proxmox, or Windows for the project server
