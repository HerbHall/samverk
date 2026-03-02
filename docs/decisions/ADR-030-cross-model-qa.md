# ADR-030: Cross-Model Quality Assurance

## Status

Proposed

## Context

Samverk agents generate code autonomously while the user is away. Code
quality assurance is essential because: (a) the user cannot review every
line in real time, (b) AI-generated code has documented hallucination
patterns (API fabrication, logic errors, security vulnerabilities), and
(c) the user's trust in Samverk depends on shipped code being correct.

The central assumption in Samverk's quality model is that cross-model
validation -- one model generates code, a different model reviews it --
produces better QA than self-review or single-model review. Research
from December 2025 (CodeX-Verify, arxiv 2511.16708) confirms this
assumption with strong evidence.

## Decision

Samverk uses a layered quality assurance pipeline with four mandatory
stages, in order:

### Stage 1 -- Deterministic Validation (Zero Token Cost)

- Compile/build the generated code
- Run the existing test suite
- Run linters and formatters
- Validate all imports against actual package registries

This stage catches syntax errors (~100%), dead code (~80-90%), and
package hallucinations (~95%) without spending any AI tokens.

### Stage 2 -- LLM-Based Review (Cross-Model)

- The QC agent must be a different model than the generating agent
- The QC agent must be Sonnet-class or better (reasoning-capable)
- Cross-provider review (Claude reviews DeepSeek output) is preferred
  over same-provider review (Claude Sonnet reviews Claude Haiku output)
- Review focuses on: logic correctness, security patterns, project
  context alignment, requirement conformance

### Stage 3 -- Test Execution

- Run any tests generated alongside the code
- Run integration tests if the change touches module boundaries
- Test results are the primary signal for logic correctness (Category 3
  errors), which LLM review catches only 40-60% of the time

### Stage 4 -- Structured Retry on Failure

- Maximum 3 retries per task after QC rejection
- Rejection feedback must include: error category, failing test case
  or reproduction step, expected vs actual behavior
- After 3 retries with the same model, switch to a different generator
- After model switch fails, escalate to user via Tier 3 autonomy block

### Confidence Policy

- LLM confidence scores are not trusted as quality gates (ICSE 2025
  demonstrated poor calibration)
- Deterministic validation (compile + test + lint) is the only binary
  quality gate
- Multi-agent consensus (2+ reviewers agree) is required for
  security-sensitive changes
- Confidence is surfaced in the check-in digest for user awareness

### Model Routing for QC

| Generator Tier | QC Reviewer | Rationale |
|----------------|-------------|-----------|
| Local 7B | Compile + lint (no LLM review) | Cost-efficient for trivial tasks |
| Local 32B | Cloud Sonnet or different local model | Cross-model diversity |
| Cloud Sonnet | Local 32B + tests, or different cloud | Cross-provider diversity |
| Cloud Opus | Different cloud provider + tests | Maximum QC for highest-complexity |

## Consequences

- Adds latency to the code generation pipeline (deterministic validation
  is fast; LLM review adds one round-trip per task)
- Increases token cost by 20-40% (QC review tokens) but prevents costly
  rework and user trust erosion
- Requires maintaining awareness of which model generated which code
  (for routing to a different reviewer)
- Package hallucination rate drops from 5-22% to near-zero with
  deterministic dependency validation
- Multi-agent review catches 72-76% of bugs vs 33% for single-agent
  review (2.3x improvement)
- Retry policy bounds worst-case token spend per task (3 retries + 1
  model switch = maximum 5 generation attempts)
- Confidence policy prevents false sense of security from overconfident
  LLM self-assessment

## References

- [Agent Quality Research](../agent-quality.md)
- [Multi-Agent Code Verification via Information Theory](https://arxiv.org/abs/2511.16708)
- [Calibration and Correctness of Language Models for Code (ICSE 2025)](https://www.software-lab.org/publications/icse2025_calibration.pdf)
- [ADR-015: Three-Tier Autonomy Model](ADR-015-three-tier-autonomy.md)
- [ADR-008: Multi-Model Default](ADR-008-multi-model-default.md)
- [Cost Model](../cost-model.md)
