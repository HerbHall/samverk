# ADR-010: The Right Success Metric

**Status**: Accepted
**Date**: 2026-02-27

## Context

AI tools are typically benchmarked on response speed, code quality scores, token efficiency, or cost per task. These metrics optimize for the wrong thing for the target user.

## Decision

Samverk is measured by project completion rate and time-to-ship for the target user. Everything else is a means to that end.

## Consequences

**Positive:**

- Every feature decision has a clear litmus test: does this help projects ship?
- Prevents premature optimization of latency, cost-per-token, or benchmark scores
- Aligns product development with the actual user problem (momentum, not capability)

**Negative:**

- Harder to market with quantitative benchmarks (competitors can show "X% faster")
- Success metric requires longitudinal data (months of usage) to measure
- May conflict with engineering instincts to optimize response time
