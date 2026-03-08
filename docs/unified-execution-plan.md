# Unified Execution Plan — Samverk Q2 2026

## Three Streams, One Goal

**61 issues** across three streams that converge into a single operational state:
Samverk running on dual forges, auto-scaling workers across CT 202 and your PC,
with full conversational oversight from any device.

| Stream | Issues | Milestone | Purpose |
|--------|--------|-----------|---------|
| B: Gitea Migration | B01–B28 | Dual-forge operation | Code runs on both GitHub and Gitea |
| W: Adaptive Scaling | W01–W20 | Smart resource use | Workers scale to match workload |
| P: PC Agent Worker | P01–P14 | Your PC as a build farm | CC processes issues in isolated worktrees |

## Cross-Stream Dependencies

```text
B-track (Gitea)              W-track (Scaling)         P-track (PC Agent)
═══════════════              ═════════════════         ══════════════════

B01-B09 Adapter code ──┐
B11 Create Gitea repo ─┤
B13 CI research ───────┤     W01-W03 Metrics ─────┐
                       │     (parallel start)      │
                       │                           │
B17 Dual-forge config ─┼──── W05 Metrics API ─────┤   P01 CC research
B19 Dispatcher on      │     W06 Dashboard ────────┼── P02 Design doc
    Gitea ─────────────┤     W09 Dynamic pool      │   P03 Forge poller
                       │          │                │   P04 CC launcher
B22 MCP dual-forge ────┤     W10 Policy engine     │   P05 Post-task
                       │     W11 Autoscaler ───────┤
                       │                           │
B27 Full agent loop ───┤     W14 Verify scaling ───┤   P06 Single issue ✓
B28 Dual-forge         │                           │   P07 Agent loop
    check-in ──────────┘     W07 Digest metrics ───┼── P09 Worker registration
                             W13 Scaling events ───┤   P08 Autonomy gate
                                                   │
                             W17 CLI controls       │   P10 2-hour soak ✓
                             W18 MCP controls ──────┤
                                                   │   P11 Multi-session
                             W20 24-hour soak ✓     │   P13 Full batch ✓
```

## Key Cross-Dependencies

| Downstream | Depends On | Why |
|-----------|------------|-----|
| P03 (forge poller) | B11 (Gitea repo) | PC agent needs a Gitea target to poll |
| P09 (worker registration) | W05 (metrics API) | Registration reports metrics to server |
| P11 (multi-session) | W10 (policy engine) | Reuses threshold logic for resource control |
| W06 (dashboard) | P09 (worker registration) | Dashboard shows PC agent alongside CT 202 |
| B19 (dispatcher on Gitea) | W02 (pool metrics) | Verify dispatch with observability |

## Execution Windows

### Window 1: Immediate Start (no blockers)

**12 issues launch in parallel.** This is the big bang.

| Issue | Stream | Agent | Task |
|-------|--------|-------|------|
| B01 | Gitea | code-gen | Gitea CreateBranch |
| B02 | Gitea | code-gen | Gitea CreateOrUpdateFile |
| B04 | Gitea | code-gen | Gitea CreatePR |
| B05 | Gitea | code-gen | Gitea GetPR + ListPRs |
| B06 | Gitea | code-gen | Gitea MergePR |
| B07 | Gitea | code-gen | Gitea GetPRChecks + ReviewComments |
| B09 | Gitea | code-gen | Gitea RepoReader (all 6 methods) |
| B13 | Gitea | research | Gitea Actions compatibility |
| **B11** | **Gitea** | **HUMAN** | **Create Gitea prod repo** |
| W01 | Scaling | code-gen | System metrics collector |
| W02 | Scaling | code-gen | Pool metrics instrumentation |
| W03 | Scaling | code-gen | Dispatcher metrics |
| P01 | PC Agent | research | CC headless invocation research |
| P02 | PC Agent | code-gen | Bare clone + worktree workspace scripts |

**Bottleneck:** 3-worker pool on CT 202 cycles through code-gen tasks.
You handle B11 and P01 in parallel. P01 is your task too — test CC
invocation modes on your PC while agents crank on adapter code.
P02 has no dependencies and can start immediately.

**Workspace isolation:** Agents work in git worktrees under
D:\bots\, provisioned from a bare clone.
Your working copy at D:\devspace\Samverk is never touched.

### Window 2: After Adapter + Metrics + Research

| Issue | Stream | Agent | Unlocked By |
|-------|--------|-------|-------------|
| B03 | Gitea | test | B01, B02 |
| B08 | Gitea | test | B01-B07 |
| B10 | Gitea | test | B09 |
| B12 | Gitea | qc | B11 |
| B14 | Gitea | code-gen | B13 |
| B17 | Gitea | code-gen | B11 |
| B18 | Gitea | code-gen | B11 |
| B20 | Gitea | code-gen | B11 |
| B24 | Gitea | HUMAN | B11 |
| W04 | Scaling | test | W01-W03 |
| W05 | Scaling | code-gen | W01-W03 |
| W09 | Scaling | code-gen | W02 |
| P03 | PC Agent | docs | P01 |
| P09 | PC Agent | code-gen | P03 |

### Window 3: Core Integration

| Issue | Stream | Agent | Unlocked By |
|-------|--------|-------|-------------|
| B15 | Gitea | code-gen | B13, B14 |
| B22 | Gitea | code-gen | B09, B17 |
| B21 | Gitea | qc | B20 |
| B25 | Gitea | docs | B17 |
| B19 | Gitea | test | B08, B12, B17 **(CRITICAL)** |
| W06 | Scaling | code-gen | W05 |
| W07 | Scaling | code-gen | W05 |
| W10 | Scaling | code-gen | W01, W02, W09 |
| P03 | PC Agent | code-gen | P02 |
| P04 | PC Agent | code-gen | P01, P02 |
| P08 | PC Agent | code-gen | P02 |

### Window 4: Agent Loop + Dashboard

| Issue | Stream | Agent | Unlocked By |
|-------|--------|-------|-------------|
| B16 | Gitea | qc | B14, B15 |
| B23 | Gitea | HUMAN | B22 |
| B26 | Gitea | docs | B25 |
| W08 | Scaling | HUMAN | W06, W07 |
| W11 | Scaling | code-gen | W09, W10 |
| P05 | PC Agent | code-gen | P03, P04 |

### Window 5: First Agent on PC + Auto-Scaling

| Issue | Stream | Agent | Unlocked By |
|-------|--------|-------|-------------|
| W12 | Scaling | code-gen | W11 |
| W13 | Scaling | code-gen | W06, W11 |
| **P07** | **PC Agent** | **HUMAN** | **P06 (CRITICAL GATE)** |

**P07 is the moment of truth:** first issue completed by CC in an isolated worktree on your PC.

### Window 6: Continuous Operation

| Issue | Stream | Agent | Unlocked By |
|-------|--------|-------|-------------|
| B27 | Gitea | qc | B08,B10,B16,B19,B21,B23 **(CRITICAL)** |
| W14 | Scaling | test | W11-W13 **(CRITICAL)** |
| P07 | PC Agent | code-gen | P06 |
| P09 | PC Agent | code-gen | P07 |

### Window 7: PC Agent at Scale

| Issue | Stream | Agent | Unlocked By |
|-------|--------|-------|-------------|
| B28 | Gitea | HUMAN | B27 **(FINAL GATE)** |
| W15 | Scaling | code-gen | W14 |
| W17 | Scaling | code-gen | W09, W05 |
| P10 | PC Agent | HUMAN | P07, P08 **(2-HOUR SOAK)** |

### Window 8: Refinement

| Issue | Stream | Agent | Unlocked By |
|-------|--------|-------|-------------|
| W16 | Scaling | code-gen | W15 |
| W18 | Scaling | code-gen | W17 |
| W19 | Scaling | docs | W14 |
| P11 | PC Agent | code-gen | P10 |
| P12 | PC Agent | docs | P10 |

### Window 9: Final Validation

| Issue | Stream | Agent | Unlocked By |
|-------|--------|-------|-------------|
| W20 | Scaling | HUMAN | W15-W19 (24-HOUR SOAK) |
| P13 | PC Agent | HUMAN | P11 (FULL BATCH TEST) |

## The Bootstrapping Moment

Once P07 passes (single issue in isolated worktree) and P08 is built (agent loop),
**the PC agent can work on its own remaining issues.** This is the
inflection point where the tool starts building itself:

- P08 (agent loop) processes B-track and W-track issues in isolated worktrees
- W-track issues give the loop observability into its own resource usage
- B-track issues give the loop a second forge to work against
- P10 registers the PC agent with the server, making it visible on the dashboard
- Your working copy stays clean throughout — you review PRs, not resolve merge conflicts

## Critical Path Analysis

The three streams have different critical paths:

**Gitea (fastest to complete):**

```text
B04-B07 → B08 → B19 → B27 → B28
   4d        1d    1d    1d    1d  = ~8 days
```

**Scaling (medium):**

```text
W01-W02 → W05 → W09 → W10 → W11 → W14 → W20
   2d       1d    2d    2d    1d    1d    1d  = ~10 days
```

**PC Agent (longest, but self-accelerating):**

```text
P01 → P03 → P05 → P06 → P07 → P08 → P11 → P12 → P14
 2d    1d    2d    1d    1d    2d    soak   2d    soak = ~14 days
```

(P02 bare clone setup runs in parallel with P01, no dependency)

After P08, the PC agent accelerates everything else.

## Priority Order for Human Tasks

You have **8 human tasks** across the three streams. Recommended order:

| Priority | Issue | Why |
|----------|-------|-----|
| 1 | B11 | Unblocks 8 downstream issues, 5 minutes of Gitea admin work |
| 2 | P01 | Research CC invocation on your PC — this is the knowledge gate |
| 3 | B24 | Configure dual-push remote — 2 minutes of git config |
| 4 | P07 | First PC agent test in isolated worktree — the bootstrap gate |
| 5 | W08 | Verify dashboard metrics — quick visual check |
| 6 | B23 | MCP from Claude Desktop against Gitea — validation |
| 7 | P11 | 2-hour PC agent soak — run while doing other things |
| 8 | B28 | Final dual-forge check-in — the ship-it gate |

W20 (24-hour soak) and P14 (full batch) run in the background.

## Summary Counts

| Category | Count |
|----------|-------|
| Total issues | 62 |
| code-gen (automatable) | 39 |
| test / qc | 11 |
| research | 2 |
| docs | 5 |
| **human** (needs you) | **8** |
| Execution windows | 9 |

54 of 62 issues are agent-executable. Once the PC agent loop is running
(Window 5), it can process most of the remaining work autonomously.

## Workspace Isolation Model

Agents NEVER touch your working copy. The entire PC agent stream uses:

```text
D:\devspace\Samverk\              ← YOUR repo. VS Code, uncommitted work. UNTOUCHED.
D:\bots\
├── samverk.git\                  ← Bare clone (shared object store, ~150 MB)
├── worker-1\                     ← Worktree: agent session 1 (created per task, destroyed after)
├── worker-2\                     ← Worktree: agent session 2
└── worker-3\                     ← Worktree: agent session 3
```

Each worktree is a full working directory with its own branch and index,
but shares the bare clone's object store (instant provisioning, zero extra disk).
CLAUDE.md, Makefile, go.mod — all inherited automatically.
Worktrees are created before each task and destroyed after.
