# ADR-013: Abstract Git Forge Behind Interface

**Status**: Accepted
**Date**: 2026-02-27

## Context

Samverk needs issue tracker operations for agent communication. Committing to a single git forge (GitHub) creates vendor lock-in and prevents self-hosted deployments.

## Decision

All issue tracker operations go through a platform-agnostic interface layer. First implementations: GitHub, Gitea.

## Consequences

**Positive:**

- Avoids GitHub lock-in
- Supports self-hosted deployments (Gitea on home server)
- Founder already runs Gitea instance for testing
- Gitea = no API rate limits, full control
- GitLab support can be added later without architecture changes

**Negative:**

- Interface must be lowest-common-denominator across forges
- Testing requires maintaining instances of each supported forge
- Forge-specific features (GitHub Actions, Gitea runners) can't be relied on in the core
