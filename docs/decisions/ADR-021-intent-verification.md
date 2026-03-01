# ADR-021: Intent Verification Protocol

## Status

Accepted

## Context

During development of Docker Desktop extension projects (DockPulse, PacketDeck), a recurring failure mode was observed: the assisting agent misinterpreted a setup/scaffold request as "build from scratch" when the correct interpretation was "audit existing state and fill gaps." This caused wasted effort reading reference material that wasn't needed and producing work that duplicated what already existed.

The root cause was not a lack of capability but a lack of verification. The agent proceeded with its interpretation of the instruction without confirming that interpretation was correct. This is a general class of error that will recur in any system where one entity delegates work to another — human-to-agent, agent-to-agent, or director-to-worker.

The existing autonomy model (ADR-015) governs what actions agents may take and at what trust level. The communication protocol (communication-protocol.md) governs how agents exchange information via issues. Neither addresses the gap between receiving an instruction and beginning execution — specifically, verifying that the instruction is understood as intended.

This gap becomes critical in multi-agent Samverk, where a misunderstood instruction can propagate through dependent work streams before anyone catches it. The cost of misalignment compounds with hierarchy depth: a misunderstood directive at the orchestrator level wastes exponentially more work than one at a leaf worker.

## Decision

Samverk implements an Intent Verification Protocol (IVP) as a mandatory pre-execution step for all agents. The protocol uses a three-tier verification model calibrated to task complexity and ambiguity:

**Tier 1 — Restate and Execute:** Agent restates the task in one sentence and begins work. Used for low-complexity, unambiguous tasks. No blocking wait.

**Tier 2 — Restate and Confirm:** Agent restates the planned approach in 2-3 sentences and waits for confirmation before starting. Used for moderate complexity or tasks with mild ambiguity.

**Tier 3 — Scope Assessment:** Agent provides a structured assessment including interpreted goal, proposed approach, and specific questions to resolve ambiguities. Does not begin work until all questions are answered. Used for high complexity, multiple valid interpretations, or tasks where rework cost is high.

The IVP is complementary to and independent from the autonomy tier model. An action may be Autonomy Tier 1 (always allowed) but IVP Tier 2 (needs understanding confirmed). Conversely, a well-understood action may be IVP Tier 1 (clear intent) but Autonomy Tier 3 (requires permission to execute).

Additionally, agents implement In-Execution Concern Flagging: when an agent discovers during execution that reality contradicts the assumptions behind its instructions, it surfaces the conflict to the delegating entity with evidence rather than silently working around it or unilaterally changing direction.

Concerns escalate up the hierarchy only. Workers flag to directors, directors flag to orchestrators, orchestrators flag to the user. No agent changes scope without authorization from the level that set the scope.

## Consequences

- Catches misalignment at the cheapest possible moment (before work begins)
- Adds a small latency cost to task initiation that scales with ambiguity
- Requires agents to have tier classification heuristics (risk of miscalibration)
- Concern flagging preserves scope integrity across the agent hierarchy
- Creates a natural feedback loop: frequent Tier 2/3 verifications on similar tasks suggest the instruction patterns need refinement
- Integrates with check-in digest: verification exchanges and flagged concerns become part of the audit trail

## References

- [Intent Verification Protocol](../intent-verification.md)
- [Autonomy Model](../autonomy-model.md)
- [Communication Protocol](../communication-protocol.md)
- [ADR-015: Three-Tier Autonomy Model](ADR-015-three-tier-autonomy.md)
- [ADR-006: Async-First Architecture](ADR-006-async-first.md)
