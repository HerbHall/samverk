# Intent Verification Protocol

**ADR**: [ADR-021](decisions/ADR-021-intent-verification.md)

## The Problem

When one entity delegates work to another, two distinct failure modes exist:

1. **Permission failure** — the agent does something it shouldn't be allowed to do. (Solved by the [Autonomy Model](autonomy-model.md).)
2. **Understanding failure** — the agent does the wrong thing because it misunderstood the instruction. (Solved here.)

Understanding failures are insidious because the agent proceeds confidently in the wrong direction. The work may look correct in isolation but doesn't match what was actually needed. The result is rework, wasted tokens, wasted time, and eroded trust.

In a multi-agent hierarchy, understanding failures compound. A misunderstood directive at the orchestrator level propagates to every worker in the dependency tree. By the time anyone notices, the entire work stream may need to be discarded.

## The Solution

Every agent performs a mandatory pre-execution verification step calibrated to the complexity and ambiguity of the task. This is not optional and not skippable — even trivial tasks get Tier 1 verification (a one-sentence restatement).

## Verification Tiers

### Tier 1 — Restate and Execute

**When**: Low complexity, clear intent, unambiguous instructions.

**Behavior**: The agent restates the task in one sentence and immediately begins work. The delegating entity only intervenes if the restatement is wrong.

**Blocking**: Non-blocking. Work begins immediately after restatement.

**Examples**:

- "Adding MIT LICENSE to PacketDeck." → executes
- "Renaming variable `foo` to `userCount` in router.go." → executes
- "Running test suite for dispatcher package." → executes

**Triggers**: Single-file changes, well-defined transformations, continuation of in-progress work, tasks where the output is immediately visible and easily reversible.

### Tier 2 — Restate and Confirm

**When**: Moderate complexity, mild ambiguity, or multiple reasonable approaches exist.

**Behavior**: The agent restates the planned approach in 2-3 sentences and waits for explicit confirmation before starting work.

**Blocking**: Lightly blocking. Agent waits for a go-ahead but the exchange should be brief.

**Examples**:

- "Both project folders already exist with scaffolds. I'll audit each against the DevKit template pattern, identify missing files, and create only the gaps. Sound right?"
- "I'll implement the registry checker using Docker Hub v2 API with anonymous auth first, then add token auth as a follow-up. The backend will cache digests for 6 hours. Confirm?"

**Triggers**: Multi-step tasks, setup/scaffold/configure work, tasks where existing state matters, tasks where the agent is choosing among several valid approaches, any instruction containing ambiguous verbs ("set up", "handle", "improve", "clean up").

### Tier 3 — Scope Assessment

**When**: High complexity, multiple valid interpretations, significant rework cost, or the instruction is broad or open-ended.

**Behavior**: The agent provides a structured assessment before touching anything:

1. **Interpreted goal**: What the agent believes the objective is.
2. **Proposed approach**: How the agent would accomplish it, including sequence and key decisions.
3. **Questions**: Specific ambiguities that must be resolved before work can begin responsibly.

**Blocking**: Deliberately blocking. Agent does not begin work until all questions are answered and the approach is confirmed.

**Examples**:

- "Here's how I'm reading this — goal is network segmentation across IoT, lab, and trusted devices. Approach: audit current topology, propose VLAN scheme, then walk through switch config changes. Questions: Are we keeping the current TP-Link switches or is new hardware on the table? Does Tailscale access need to survive the change?"
- "This looks like a refactor of the dispatcher routing logic. Goal: replace regex matching with label-based routing. Approach: define a new RoutingRule type, migrate existing routes, update tests, deprecate old path. Questions: Should the old regex path remain as a fallback during migration, or hard-cut? Is there a performance budget for the routing hot path?"

**Triggers**: Architecture decisions, multi-session work, anything touching production systems, broad or open-ended language in the instruction, tasks where rework cost exceeds the verification cost by a large margin.

## Tier Classification Heuristics

Agents classify tasks using these signals:

| Signal | Tier 1 | Tier 2 | Tier 3 |
|--------|--------|--------|--------|
| Number of files affected | 1 | 2-10 | 10+ or unknown |
| Estimated duration | Minutes | Hours | Days or sessions |
| Ambiguous verbs in instruction | None | Some ("set up", "fix") | Many ("improve", "restructure", "handle") |
| Existing state matters | No | Yes | Critical |
| Multiple valid approaches | No | Yes | Yes, with tradeoffs |
| Reversibility | Easily | With effort | Difficult or impossible |
| Scope of impact | Local | Module | Cross-cutting |
| Prior art / pattern to follow | Clear | Exists but needs adaptation | Novel |

When signals conflict, the agent rounds UP to the higher tier. It is always cheaper to over-verify than to rework.

## In-Execution Concern Flagging

Verification doesn't end when work begins. During execution, agents may discover that reality contradicts the assumptions behind the original instruction. When this happens, the agent flags the concern rather than silently working around it or unilaterally changing direction.

### What Gets Flagged

An agent flags a concern when it encounters:

- **Assumption conflicts**: The instruction assumed X, but the agent found Y. Example: "Task assumes SQLite, but the data model requires concurrent writes from multiple containers."
- **Missing prerequisites**: Something the instruction didn't mention is required first. Example: "The registry API requires authentication, but no credentials are configured."
- **Scope expansion signals**: Completing the task as described requires significantly more work than the instruction implies. Example: "Fixing this CSS bug requires refactoring the entire layout component because the bug is structural."
- **Risk discovery**: The agent identifies a risk that wasn't visible at instruction time. Example: "This migration would delete 200 rows that match the WHERE clause — significantly more than the 'handful' described in the task."

### What Does NOT Get Flagged

- Routine decisions within the agent's expertise (variable naming, code formatting, test structure)
- Expected complexity that was acknowledged in the instruction
- Preferences or opinions about approach when the instruction was specific
- Disagreement with settled architectural decisions

### Flag Format

When flagging a concern via an issue comment, the agent provides:

```markdown
## ⚠ Concern Flagged

**What the instruction assumed**: [specific assumption]
**What I found**: [specific evidence]
**Why it matters**: [impact on the task or broader project]
**Options**:
1. [Option A — description and tradeoff]
2. [Option B — description and tradeoff]
3. [Option C — if applicable]

**Recommended**: [which option and brief rationale]
**Awaiting direction before**: [specific action that is paused]
**Continuing with**: [work that is NOT blocked by this concern]
```

### Flag Behavior

- The agent pauses only the specific action affected by the concern
- All independent work continues — a flag is NOT a full stop
- The concern is posted as a comment on the current task issue
- The issue receives the `status:needs-human` label if the concern requires a user decision
- The issue receives the `status:blocked` label if the concern can be resolved by a director agent

## Escalation Rules

Concerns follow the hierarchy strictly:

```text
Worker Agent → Director Agent → Orchestrator → User
```

### Rules

1. **Concerns escalate up, never sideways or down.** A worker does not instruct peer workers to change course. It reports to its director.

2. **Escalation carries evidence, not opinions.** The flag format requires specific assumptions, specific findings, and specific impact. "I think we should use PostgreSQL instead" is not a valid concern. "The task assumes SQLite but I found concurrent write requirements in the acceptance criteria" is.

3. **Scope changes only propagate downward from the level that authorized them.** If a worker flags something and the director agrees, the director adjusts the plan and re-delegates. If the concern is project-level, the director escalates to the orchestrator rather than making a project-level decision.

4. **No agent unilaterally changes scope.** An agent that discovers a problem proposes a solution — it does not implement one without authorization from the appropriate level.

5. **Repeated flags on similar issues trigger process review.** If workers frequently flag the same type of concern, the instruction patterns at the director level need refinement. This is a signal, not a failure.

## Interaction With Other Protocols

### Autonomy Model (ADR-015)

IVP and the autonomy model are independent axes:

|  | Autonomy Tier 1 (allowed) | Autonomy Tier 3 (needs permission) |
|--|---|---|
| **IVP Tier 1 (clear)** | Execute immediately, log action | Understand clearly, queue for permission |
| **IVP Tier 3 (ambiguous)** | Clarify first, then execute freely | Clarify first, then queue for permission |

An agent must pass BOTH checks before executing: it must understand the task correctly (IVP) AND have permission to take the required actions (autonomy).

### Communication Protocol

IVP exchanges use the existing issue comment system. Tier 1 restatements appear as a one-line comment when work begins. Tier 2 confirmations are a brief comment exchange. Tier 3 assessments are structured comments using the scope assessment format.

Concern flags use the existing `status:needs-human` and `status:blocked` labels.

### Check-in Digest

The digest surfaces IVP activity in the appropriate section:

- **Tier 3 pending assessments** — shown with `needs-human` items (blocking decisions)
- **Flagged concerns** — shown with `needs-human` items
- **Tier 2 confirmation log** — shown in the Tier 2 audit section
- **Tier 1 restatement log** — available on request, not shown by default

## Human-to-Agent Application

This protocol originated from human-to-agent interaction patterns and applies there as well. When a human delegates work to Claude (or any agent), the same tiers apply:

- Simple, clear requests get a one-sentence restatement and immediate execution
- Moderate requests get a plan restatement and wait for confirmation
- Complex or ambiguous requests get a scope assessment with clarifying questions

This creates a consistent verification pattern regardless of whether the delegator is human or agent.

## Calibration and Adaptation

The tier classification heuristics will need refinement based on operational experience. Early deployments should:

1. Default to over-classification (round up to higher tiers)
2. Log all tier classifications and their outcomes
3. Track rework events and correlate with tier classification at task initiation
4. Adjust heuristics quarterly based on data

A task that was classified Tier 1 but resulted in rework is a calibration signal — that task type should be promoted to Tier 2. Conversely, a task type that is consistently Tier 2 with zero rework events can be demoted to Tier 1 after sufficient evidence.

## Related Documents

- [ADR-021: Intent Verification Protocol](decisions/ADR-021-intent-verification.md)
- [Autonomy Model](autonomy-model.md) — permission tiers (what agents may do)
- [Communication Protocol](communication-protocol.md) — message format and routing
- [Dispatcher Design](dispatcher-design.md) — task routing and dependency management
