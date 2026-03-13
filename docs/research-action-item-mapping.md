# Research Action Item Mapping

**Source:** [Multi-Agent Research Analysis](multi-agent-research.md)
**Assessed:** 2026-03-13
**Codebase state:** Post Phase 5, Q2 execution streams (B/W/P) in progress

## Status Legend

- **DONE** — Fully implemented, no further work needed
- **PARTIAL** — Foundation exists, specific gaps remain
- **NOT IMPL** — Not started, needs new work
- **DEFERRED** — Intentionally deferred (low priority or insufficient data)

## Comprehensive Mapping

### HIGH PRIORITY (Research Items 1-5)

#### Item 1: Cross-Model QC Enforcement

**Research says:** When code-gen uses Model A, QC must use Model B. Same-model QC catches
only ~80% of errors; cross-model catches more.

**Status:** PARTIAL

| What Exists | Location |
|---|---|
| ADR-030 approved and documented | [ADR-030](decisions/ADR-030-cross-model-qa.md) |
| Provider routing chains per agent type | [registry.go:76-96](../internal/provider/registry.go#L76-L96) |
| Complexity-based provider key selection | [router.go:99-142](../internal/dispatcher/router.go#L99-L142) |
| AgentTypeQC recognized in dispatcher | [issue.go:26](../pkg/models/issue.go#L26) |

| What's Missing | Effort |
|---|---|
| Route QC to a different provider than the code-gen that created the work | Config + router logic |
| Track which model generated an issue's code (consult `model_used` frontmatter during QC routing) | Router enhancement |
| Formalize cross-model policy in provider routing config | Config change only |

**Re-prioritized:** HIGH — Fits current phase. The routing infrastructure already exists in
the W-stream provider registry. Adding cross-model awareness to QC routing is a natural
extension of the existing `selectProviderKey()` logic. **Estimated: 1 issue, code-gen agent.**

---

#### Item 2: `files_touched` Field in IssueFrontmatter

**Research says:** File/artifact tracking scores 2.45/5.0 across all compression methods —
the weakest link. Agents lose file state during context resets.

**Status:** NOT IMPL

| What Exists | Location |
|---|---|
| IssueFrontmatter struct (no files_touched field) | [issue.go:66-77](../pkg/models/issue.go#L66-L77) |
| Frontmatter parser | [issue.go:79+](../pkg/models/issue.go#L79) |
| Communication protocol schema docs | [communication-protocol.md](communication-protocol.md) |

| What's Missing | Effort |
|---|---|
| Add `FilesTouched []string` field to IssueFrontmatter | Small schema change |
| Update ParseFrontmatter to handle the field | Parser update |
| Agent prompt instructions to populate it | Prompt engineering |
| QC agents verify files_touched against actual git diff | QC logic enhancement |

**Re-prioritized:** MEDIUM — Valuable for production quality but not blocking any current
stream. The checkpoint system (#243/#244) partially addresses context loss by persisting
partial output. `files_touched` adds value when orchestrator and multi-turn agents are
active (Phase 6+). **Estimated: 1 issue, code-gen agent.**

---

#### Item 3: PROGRESS Comment Protocol

**Research says:** Agents should post structured progress comments every 30 minutes during
long tasks. Creates resumable state that survives agent replacement.

**Status:** PARTIAL — CHECKPOINT exists, PROGRESS does not

| What Exists | Location |
|---|---|
| CHECKPOINT comment format and posting | [checkpoint.go:13-119](../internal/agent/checkpoint.go#L13-L119) |
| Checkpoint detection and resume prompt building | [checkpoint.go:25, 113](../internal/agent/checkpoint.go#L25) |
| Resume logic in runner (detect checkpoint, inject context) | [runner.go:102-116](../internal/agent/runner.go#L102-L116) |
| Checkpoint deduplication (hash-based) | [runner.go:397-435](../internal/agent/runner.go#L397-L435) |
| Heartbeat pulse during provider.Chat() | [runner.go:142-158](../internal/agent/runner.go#L142-L158) |
| Streaming activity detection | [runner.go:161-169](../internal/agent/runner.go#L161-L169) |

| What's Missing | Effort |
|---|---|
| Mid-task PROGRESS comments (separate from failure-triggered CHECKPOINTs) | Runner enhancement |
| Structured format: decisions made, files modified, tests status, remaining work | Format definition |
| Timer-based progress posting (every N minutes of active work) | Goroutine in runner |
| Agent prompt instructions to populate progress notes | Prompt engineering |

**Re-prioritized:** HIGH — The checkpoint infrastructure from #243/#244 provides 80% of the
foundation. Adding periodic PROGRESS comments is an incremental enhancement that directly
improves multi-hour task reliability. This is the current phase's biggest resilience gap.
**Estimated: 1 issue, code-gen agent.**

---

#### Item 4: Meta-QC on Orchestrator Output

**Research says:** Specification quality determines downstream success. Validate that
orchestrator-generated acceptance criteria are specific, testable, and measurable before
creating sub-issues.

**Status:** NOT IMPL

| What Exists | Location |
|---|---|
| AgentTypeOrchestrator constant defined | [issue.go:20](../pkg/models/issue.go#L20) |
| Orchestrator mentioned in communication protocol | [communication-protocol.md](communication-protocol.md) |

| What's Missing | Effort |
|---|---|
| Orchestrator agent implementation (not just constant) | Major feature |
| QC validation of acceptance criteria before sub-issue creation | New QC step |
| Acceptance criteria quality metrics (specific, testable, measurable) | Prompt engineering |
| Test suite for orchestrator behavior | Test development |

**Re-prioritized:** DEFERRED — The orchestrator agent is a Phase 6+ feature. Meta-QC on its
output depends on the orchestrator existing first. No current stream needs this.
**Track as future work.**

---

#### Item 5: API-Level Autonomy Enforcement

**Research says:** Tier boundaries must be enforced at the system level (API), not by agent
prompts. An agent instructed "don't delete files" may still delete files.

**Status:** DONE

| What Exists | Location |
|---|---|
| AutonomyPolicy interface with TierFor/RequiresConfirmation | [policy.go:5-51](../internal/autonomy/policy.go#L5-L51) |
| Context-bound policy (branch > agent > global > default) | [policy.go:28-34](../internal/autonomy/policy.go#L28-L34) |
| YAML config with tier overrides | [autonomy.yaml](../deploy/config/autonomy.yaml) |
| Policy checked before routing in dispatcher | [dispatcher.go](../internal/dispatcher/dispatcher.go) |

| Enhancement Opportunity | Effort |
|---|---|
| Wrap IssueTracker methods to check tier before executing (defense-in-depth) | Medium refactor |
| Currently caller is trusted to check; interface itself allows anything | Interface wrapper |

**Re-prioritized:** LOW for wrapper — Current implementation is architecturally correct.
The enhancement (wrapping IssueTracker methods) adds defense-in-depth but the risk is low
while agents are single-turn and dispatcher-controlled. **No issue needed now.**

---

### MEDIUM PRIORITY (Research Items 6-10)

#### Item 6: Daily Self-Audit Job

**Research says:** Multi-day autonomous systems need independent health monitoring: budget
vs. spend, queue trends, stalled streams, issues stuck >24h.

**Status:** NOT IMPL

| What Exists | Location |
|---|---|
| Health endpoint (/healthz) | [server.go](../internal/server/server.go) |
| Metrics collection (pool, dispatcher, system) | W-stream implementation |
| MCP digest with pressure indicator | [digest/](../internal/digest/) |
| Scaling events tracked and persisted | W-stream implementation |

| What's Missing | Effort |
|---|---|
| Scheduled background job (ticker or cron) | New goroutine in server |
| Audit checks: budget drift, queue depth trends, stalled issues | Logic in audit job |
| Health file output (machine-readable audit results) | File writer |
| Digest integration (surface audit results at check-in) | Digest enhancement |

**Re-prioritized:** MEDIUM — Becomes critical when the system runs unattended for 24h+.
The W-stream metrics infrastructure provides the raw data; the audit job is the consumer.
Natural fit after W20 (24h soak test) validates baseline stability.
**Estimated: 2 issues (audit job + digest integration).**

---

#### Item 7: Eager Model Initialization

**Research says:** Lazy prompt/model construction causes first-call latency and race
conditions. Eager initialization completes setup before returning.

**Status:** PARTIAL (lazy by design, mostly correct)

| What Exists | Location |
|---|---|
| Provider factory pattern (create on demand) | [main.go:150+](../cmd/samverk/main.go#L150) |
| Registry.Get() with health check | [registry.go:76-96](../internal/provider/registry.go#L76-L96) |
| Ollama local provider | [provider/ollama.go](../internal/provider/ollama.go) |

| What's Missing | Effort |
|---|---|
| Startup health probe for all configured providers | Provider enhancement |
| Pre-pull Ollama models at container start | Container config |
| Log warning if configured provider is unreachable at startup | Logging enhancement |

**Re-prioritized:** LOW — Cloud APIs (Claude, OpenAI) don't benefit from eager init. Ollama
is the only case where eager model loading matters, and that's a container config change.
**Estimated: 1 issue, infra agent.**

---

#### Item 8: Dependency Cycle Resolution UX

**Research says:** When users check in after 2 days and find a cycle, they need a
recommended resolution, not just an error message.

**Status:** PARTIAL

| What Exists | Location |
|---|---|
| Cycle detection via Kahn's algorithm | [graph.go:8-79](../internal/dispatcher/graph.go#L8-L79) |
| Critical path analysis | [graph.go:81-137](../internal/dispatcher/graph.go#L81-L137) |
| ErrCycleDetected error type | [graph.go:9](../internal/dispatcher/graph.go#L9) |

| What's Missing | Effort |
|---|---|
| Report which issues form the cycle | Graph traversal enhancement |
| Recommend lowest-cost dependency to break | Heuristic logic |
| Post structured comment on affected issues | Dispatcher enhancement |
| Digest integration (surface cycle info at check-in) | Digest enhancement |

**Re-prioritized:** LOW — Cycle detection fires correctly today. The UX improvement matters
for user experience but isn't blocking any stream. **Estimated: 1 issue, code-gen agent.**

---

#### Item 9: Per-Issue Token Budget

**Research says:** Monitor per-issue token spend to detect runaway loops or hallucination
cycles.

**Status:** PARTIAL

| What Exists | Location |
|---|---|
| Per-session cost recording | [tracker.go:54-67](../internal/cost/tracker.go#L54-L67) |
| Global rolling budget check (24h window) | [tracker.go:39](../internal/cost/tracker.go#L39) |
| Session links to issue via IssueNumber | [models/session.go](../pkg/models/session.go) |
| IssueFrontmatter.EstimatedTokens field | [issue.go:73](../pkg/models/issue.go#L73) |

| What's Missing | Effort |
|---|---|
| Per-issue token aggregation (sum across retries) | SQL query + tracker method |
| Per-issue budget limit (configurable in frontmatter) | Schema + enforcement |
| Outlier detection (alert when issue cost > 2x estimate) | Cost tracker enhancement |

**Re-prioritized:** MEDIUM — Per-issue aggregation is a SQL query away (sessions already
link to issues). Outlier alerting adds safety for multi-day unattended operation.
**Estimated: 1 issue, code-gen agent.**

---

#### Item 10: Structured Note-Taking Persistence

**Research says:** Agents should write notes persisted outside the context window. Issue
comments provide the natural persistence layer.

**Status:** PARTIAL (same as Item 3)

Overlaps significantly with Item 3 (PROGRESS comments). The checkpoint system provides
failure-recovery persistence. What's missing is proactive, mid-task note-taking that helps
the *same* agent (not just a replacement) maintain coherence over long sessions.

**Re-prioritized:** Merged with Item 3. A single PROGRESS comment implementation covers
both action items.

---

### LOW PRIORITY (Research Items 11-13)

#### Item 11: Schema-Level Tool Restrictions

**Research says:** Agents should not have access to APIs they shouldn't call. A code-gen
agent shouldn't manage issues; the dispatcher handles that.

**Status:** NOT IMPL

| What Exists | Location |
|---|---|
| Uniform IssueTracker interface for all agent types | [forge.go:40-64](../internal/forge/forge.go#L40-L64) |
| Agent types defined but not permission-scoped | [issue.go:18-30](../pkg/models/issue.go#L18-L30) |

**Re-prioritized:** DEFERRED — Current agents are single-turn and dispatcher-controlled.
They don't have direct forge access; the runner posts comments on their behalf. This
becomes relevant when agents gain multi-turn capability with direct tool access (Phase 6+).

---

#### Item 12: RL-Trained Orchestrator

**Research says:** Dynamic agent sequencing via reinforcement learning.

**Status:** NOT IMPL (correctly)

**Re-prioritized:** DEFERRED — Requires production data from hundreds of task completions
to train on. Not viable until Samverk has been running autonomously for months. The
heuristic routing in `selectProviderKey()` is sufficient for current scale.

---

#### Item 13: Persistent Approval Rules

**Research says:** Prevent approval fatigue by remembering user decisions for repeated Tier 3
actions.

**Status:** DONE

| What Exists | Location |
|---|---|
| autonomy.yaml with tier_overrides, agents, branches | [autonomy.yaml](../deploy/config/autonomy.yaml) |
| Config loader and precedence chain | [config.go:14-19](../internal/autonomy/config.go#L14-L19) |
| Branch glob matching for overrides | [policy.go:28-34](../internal/autonomy/policy.go#L28-L34) |

**Re-prioritized:** DONE. File-based YAML is appropriate for the current user base (solo
developer). A management UI would be nice-to-have but isn't needed.

---

## Re-Prioritized Action Items Against Current Streams

The original research prioritized items 1-5 as "High (before Phase 3)" but Samverk is now
post-Phase 5 with three active execution streams. Here's the re-prioritization based on
what's already done and what matters NOW:

### Tier 1: Implement Now (fits current phase, high impact)

| # | Action | Stream Fit | Why Now |
|---|---|---|---|
| 1 | Cross-model QC routing | W-stream extension | Provider registry is built; config-level change |
| 3+10 | PROGRESS comment protocol | Extends #243/#244 checkpoint | Checkpoint infra exists; incremental enhancement |
| 9 | Per-issue token aggregation + outlier alerts | W-stream extension | Cost tracker exists; SQL query + alerting |

### Tier 2: Next Phase (valuable but not blocking)

| # | Action | When | Why Wait |
|---|---|---|---|
| 2 | `files_touched` field | Phase 6 (multi-turn agents) | Single-turn agents don't accumulate file state across turns |
| 6 | Daily self-audit job | After W20 (24h soak) | Need baseline stability data first |
| 8 | Cycle resolution UX | After B-stream completes | Dependency graph is exercised more during Gitea migration |

### Tier 3: Deferred (future phase or not needed)

| # | Action | Why Defer |
|---|---|---|
| 4 | Meta-QC on orchestrator | Orchestrator agent doesn't exist yet (Phase 6+) |
| 5 | API-level autonomy wrapper | Already correct at policy level; wrapper is defense-in-depth |
| 7 | Eager model init | Low impact; container config change when needed |
| 11 | Schema-level tool restrictions | Agents don't have direct forge access currently |
| 12 | RL-trained orchestrator | Needs months of production data |
| 13 | Persistent approval rules | Already implemented via autonomy.yaml |
