# ADR-014: Dispatcher Agent as Always-Running Process

**Status**: Accepted
**Date**: 2026-02-27

## Context

The back-end agent team needs coordination -- something must watch the issue tracker, evaluate incoming tasks, check dependencies, and route work to the right specialist agents.

## Decision

A dedicated dispatcher agent runs continuously watching the issue tracker. It is not a specialist agent -- it does no execution work. Its only job is routing: evaluate incoming issues, check dependencies, assign to appropriate agent pools.

## Consequences

**Positive:**

- Clean separation of routing logic from execution logic
- Single point of responsibility for task assignment
- Specialist agents remain simple -- they just watch for their assignments
- Dependency checking and timeout handling centralized in one place

**Negative:**

- Single point of failure -- if dispatcher is down, no work gets routed
- Must be highly reliable and recoverable
- Adds a process that must always be running (resource cost)
