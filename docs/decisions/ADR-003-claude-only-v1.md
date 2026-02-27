# ADR-003: Claude-Only for V1

**Status**: Partially Superseded by [ADR-008](ADR-008-multi-model-default.md)
**Date**: 2026-02-26

## Context

Cross-provider validation (using Claude to validate GPT-4 output) is a compelling differentiator, but adds significant complexity.

## Decision

V1 ships Claude-only. Provider abstraction is built in from day one to enable V2 multi-provider support cleanly.

## Consequences

**Positive:**

- Reduces V1 scope to something shippable
- Anthropic's Claude API is the developer's primary tool already
- Clean abstraction means V2 doesn't require a rewrite

**Negative:**

- V1 limited to single provider pricing and capabilities
- Cross-provider validation (key differentiator) deferred to V2
