# ADR-012: Git Issues as Agent Communication Protocol

**Status**: Accepted
**Date**: 2026-02-27

## Context

Agents need a communication mechanism for task assignment, results, handoffs, and escalation. Options considered: custom message queue, internal database, or leveraging existing git forge issue trackers.

## Decision

Inter-agent communication is built on git issues rather than a custom message queue or internal database. Issues are the task unit, comment threads are working memory, labels are the routing schema, webhooks are the event system.

## Consequences

**Positive:**

- Human-readable audit trail by default
- Universal device access (any browser can read issues)
- User check-in interface comes for free (the issue list IS the digest)
- Existing webhook/event infrastructure -- no new systems to build
- No new infrastructure to build or maintain

**Negative:**

- No atomic "claim" operation -- requires optimistic locking pattern
- API rate limits on hosted forges (GitHub) constrain polling frequency
- Issue tracker was not designed for machine-to-machine communication -- may hit scaling limits
- Schema enforcement is convention-based, not enforced by the platform

## References

- [Communication Protocol](../communication-protocol.md)
