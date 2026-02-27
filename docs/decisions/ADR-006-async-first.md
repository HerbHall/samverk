# ADR-006: Async-First Architecture

**Status**: Accepted
**Date**: 2026-02-27

## Context

Every existing AI development tool operates synchronously -- the user sits at their keyboard, prompts, waits, reviews, repeats. This model excludes the target user who has 10-15 minutes at a time.

## Decision

The primary interaction model is check-in based, not synchronous. Users are not expected to be present while agents work.

## Consequences

**Positive:**

- Unlocks the hobbyist/part-time developer segment that no competitor serves
- Agents can work continuously, maximizing throughput per dollar
- User's time investment drops from hours per session to minutes per check-in

**Negative:**

- UX priorities change fundamentally -- the check-in digest becomes the most critical surface
- Performance optimization targets shift from latency to throughput
- Agent autonomy decisions become critical (when to proceed vs. when to block for user input)
- Harder to demo/market than synchronous tools ("watch it code in real time")
