# ADR-011: Chat as Primary Interface

**Status**: Accepted
**Date**: 2026-02-27

## Context

The original architecture assumed a custom web UI or dashboard for the user-facing interface. Session 2 brainstorming revealed this adds unnecessary scope -- Claude (or compatible models) already provides a conversational interface on every device.

## Decision

The user-facing interface is a conversational chat agent, not a custom web UI or dashboard. This means Claude (or compatible model) with MCP access to the project's git forge.

## Consequences

**Positive:**

- Works on every device today (phone, tablet, laptop, desktop)
- No custom UI to build for V1
- Anthropic is actively expanding MCP to mobile, closing the remaining gap
- Natural language is the most flexible input method for async direction-giving
- Reduces V1 scope significantly

**Negative:**

- Dependent on Claude/MCP ecosystem for the primary UX
- Less control over the check-in experience than a custom UI
- Mobile MCP support is not yet shipped -- gap until it arrives
