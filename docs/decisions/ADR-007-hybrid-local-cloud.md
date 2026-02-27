# ADR-007: Hybrid Local/Cloud Agent Model

**Status**: Accepted
**Date**: 2026-02-27

## Context

The original architecture assumed cloud-only operation. Cloud API costs scale linearly with agent activity, and the target user is budget-conscious.

## Decision

Low-level execution tasks route to local containerized agents. Complex reasoning, orchestration, and QC arbitration route to cloud models. The boundary is determined by task complexity, not by cost alone.

## Consequences

**Positive:**

- High-volume low-complexity work (code gen, tests, formatting) runs at near-zero marginal cost
- Cloud budget reserved for high-value reasoning tasks
- Local agents can run continuously without billing anxiety
- Containerized agents provide clean resource boundaries and reproducibility

**Negative:**

- Requires user to have capable local hardware (or accept slower local execution)
- Container orchestration adds infrastructure complexity
- Model quality gap between local and cloud must be managed carefully
