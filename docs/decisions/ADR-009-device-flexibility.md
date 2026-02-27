# ADR-009: Device Flexibility Is Non-Negotiable

**Status**: Accepted
**Date**: 2026-02-27

## Context

The async model only works if the user can check in from wherever they are. A tool that requires sitting at a specific machine defeats the purpose.

## Decision

The user must be able to check in from any device -- desktop, laptop, phone, tablet -- without friction. File transfer and copy-paste workflows between devices are explicitly a UX failure state.

## Consequences

**Positive:**

- User can check in during lunch from their phone, before bed from a tablet, or from their desk
- Removes the single biggest friction point for the async model
- Forces good API-first architecture (mobile clients need clean APIs)

**Negative:**

- Requires web-based or cross-platform native UI from day one
- Mobile UX for development decisions is a hard design problem
- Sync/offline behavior must be designed carefully
- Significantly increases V1 scope compared to a desktop-only tool
