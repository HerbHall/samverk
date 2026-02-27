# ADR-005: Go as Implementation Language

**Status**: Accepted
**Date**: 2026-02-26

## Context

Consistent with the Subnetree project (same developer, same stack).

## Decision

Go.

## Consequences

**Positive:**

- Developer already learning Go via Subnetree
- Good performance characteristics for orchestration workloads
- Strong CLI tooling ecosystem
- Single-binary distribution fits the target user (solo dev, easy install)

**Negative:**

- Go's type system is less expressive than Rust or TypeScript for complex agent hierarchies
- AI/ML ecosystem is Python-dominated; Go SDK support may lag
