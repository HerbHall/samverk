# ADR-004: Custom Orchestration

**Status**: Accepted
**Date**: 2026-02-26

## Context

Existing frameworks like LangGraph and CrewAI could be used as a base.

## Decision

Custom orchestration layer.

## Consequences

**Positive:**

- Orchestration logic is Samverk's core IP
- Building on others' abstractions constrains the design
- Acquisition-readiness requires owning the core differentiation
- No risk of third-party breaking changes cascading into Samverk

**Negative:**

- More initial development work
- Must build primitives that existing frameworks provide for free

## Alternatives Considered

- **LangGraph**: Mature but constrains agent hierarchy design
- **CrewAI**: Closer mental model but less control over orchestration
