# ADR-002: Application Layer, Not Infrastructure

**Status**: Accepted
**Date**: 2026-02-26

## Context

The multi-agent framework space in 2026 is dominated by Google ADK, Microsoft Agent Framework, OpenAI Agents SDK, and others at the infrastructure layer.

## Decision

Samverk will be an application-layer framework built on top of existing AI providers -- not competing with them.

## Consequences

**Positive:**

- Solo builder cannot compete with Google/Microsoft/OpenAI at infrastructure
- Application layer has clear unmet need (solo developer / indie hacker segment)
- Differentiation is in UX and mental model, not low-level orchestration primitives

**Negative:**

- Dependent on upstream provider APIs and pricing
- Feature ceiling defined by what providers expose
