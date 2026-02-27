# ADR-008: Multi-Model by Default

**Status**: Accepted
**Date**: 2026-02-27

## Context

ADR-003 specified Claude-only for V1 with multi-provider as a V2 feature. The brainstorm session reconsidered this -- provider failover on credit exhaustion is too important to defer.

## Decision

Samverk is model-agnostic from day one. Provider failover on credit exhaustion is a core feature, not a nice-to-have. This serves both cost management and quality diversity goals.

## Consequences

**Positive:**

- Never blocked by a single provider's billing cycle or outage
- Different models catch different bugs -- rotating providers improves output quality
- Users with multiple subscriptions get maximum value from all of them

**Negative:**

- V1 scope increases -- must handle multiple provider APIs from the start
- Provider-specific quirks (rate limits, token counting, output format) multiply
- Testing matrix grows significantly

## Supersedes

Partially supersedes [ADR-003](ADR-003-claude-only-v1.md) -- Claude remains the primary provider, but multi-model support ships in V1 rather than being deferred to V2.
