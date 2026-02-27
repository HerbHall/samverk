# ADR-018: Release Versioning and V1 Scope

## Status

Accepted

## Context

The project had 17 ADRs, 22 issues, and no defined release milestones. "V1 scope" was listed as an open decision blocking prioritization of all downstream work.

Discussion clarified that V1 is not the minimum viable loop -- it is a stable, feature-complete system ready for public use. The minimum viable loop is alpha. Between alpha and public release sits a beta that proves all major subsystems work together.

## Decision

Samverk uses a three-phase release model:

### v0.0.1 -- Alpha (Single Loop Proof)

The minimum that demonstrates the core async loop works end-to-end.

**In scope:**

- Go project scaffold (module, directory structure, Makefile, CI)
- Forge abstraction interface with Gitea implementation (primary target)
- Issue schema as machine-readable spec
- Dispatcher agent (watches issues, routes to agents)
- One specialist agent (code-gen) + its QC mirror
- Autonomy config loader (.samverk/autonomy.yaml)
- Basic check-in digest (Tier 3 pending, Tier 2 summary)
- Claude API as the cloud model provider
- Self-hosted deployment on home server

**Out of scope:** Multi-model failover, local agents, GitHub support, user profile, multiple specialist types, mobile-specific features.

**Success criteria:** User creates a task via Gitea issue, dispatcher routes it, code-gen agent produces output, QC validates, result appears on the issue. User reviews at next check-in.

### v0.1 -- Beta (Multi-System Integration)

Proves all major subsystems work together with real workloads.

**Adds to alpha:**

- Multi-model capability (Claude + at least one alternative cloud provider)
- Local agent execution via Ollama (RTX 3090 Ti, 24GB VRAM)
- Model routing: local for narrow tasks, cloud for complex reasoning
- GitHub forge implementation (second platform, validates abstraction)
- Multiple specialist agents (code-gen, test, docs, research)
- Basic user profile (project conventions, tech preferences)
- Cost tracking and budget awareness

**Success criteria:** Samverk builds a non-trivial feature across multiple agent types, using local models for routine work and cloud for complex decisions, on a self-hosted Gitea instance. Cost per task is tracked and reported.

### v1.0 -- Public Release

Stable, polished, all features at production quality.

**Adds to beta:**

- All specialist agent types operational and tested
- User profile fully functional with Devkit ingestion
- Check-in digest polished for 10-minute review sessions
- Multi-device access validated (phone, tablet, desktop)
- Documentation, onboarding flow, installation guide
- Security hardening (API key management, network isolation)
- Performance optimization (cold start, throughput)
- Error recovery and resilience (forge outage, model unavailability)

**Success criteria:** A new user can install Samverk on their own server, connect their repos, and have agents working on their project within an hour. The system runs unattended for days without intervention.

## Consequences

- Alpha is achievable with focused effort -- limited scope, one forge, one agent type
- Beta requires real hardware deployment (home server with GPU)
- V1 is a high bar -- stability and polish take longer than features
- Milestones on GitHub align to these release phases
- Server platform decision (Unraid/Proxmox/Windows) must be made before beta

## References

- [ADR-019: Self-Hosted-First Development](ADR-019-self-hosted-first.md)
- [Architecture](../architecture.md)
- [Autonomy Model](../autonomy-model.md)
