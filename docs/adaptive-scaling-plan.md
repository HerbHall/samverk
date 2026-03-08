# Adaptive Worker Scaling — Execution Plan

## Design Philosophy

Each phase delivers standalone value. You don't need auto-scaling to benefit
from observability. You don't need the full tune loop to benefit from basic scaling.

```
Phase 1: OBSERVE          Phase 2: EXPOSE           Phase 3: SCALE         Phase 4: TUNE
(see what's happening)    (show it to humans)       (act on it)            (get smarter)
                                                                           
W01 System metrics ──────► W05 /api/v1/metrics ────► W10 Policy engine    W15 Persist history
W02 Pool metrics ────────► W06 Dashboard page ──┐    W11 Autoscaler loop  W16 Task profiling
W03 Dispatcher metrics ──► W07 MCP digest       │    W12 Config/CLI       W17 Manual CLI
W04 Verify ──────────────► W08 Verify           │    W13 Dashboard events W18 MCP controls
                                                │    W14 Verify            W19 ADR-032
                           ┌────────────────────┘                          W20 Soak test
                           │
                           └──► W09 Dynamic pool ──► W10, W11
```

## Phase 1: Observe (4 issues)

**Value delivered:** You can SSH into CT 202 and see exactly what the
dispatcher is doing — CPU, memory, worker utilization, queue depth,
task durations. Even without auto-scaling, this tells you when to
manually bump `--workers`.

**Parallel work:** W01, W02, W03 have zero dependencies and run simultaneously.

| Ref | Title | Agent | Depends |
|-----|-------|-------|---------|
| W01 | System metrics (CPU, mem, goroutines) | code-gen | — |
| W02 | Pool metrics (active workers, queue, durations) | code-gen | — |
| W03 | Dispatcher metrics (claimed, poll latency) | code-gen | — |
| W04 | Verify metrics accuracy under load | test | W01,W02,W03 |

## Phase 2: Expose (4 issues)

**Value delivered:** Metrics visible on the web dashboard and in MCP
check-in digests. You ask "how's samverk doing?" and get resource
health alongside project status.

**Parallel work:** W05 unlocks W06 and W07 simultaneously. W09 (pool
refactor) starts here too since it only depends on W02.

| Ref | Title | Agent | Depends |
|-----|-------|-------|---------|
| W05 | /api/v1/metrics endpoint + pressure indicator | code-gen | W01,W02,W03 |
| W06 | Dashboard metrics page (charts, gauges) | code-gen | W05 |
| W07 | Metrics in MCP get_digest | code-gen | W05 |
| W08 | Verify: dashboard + MCP show real data | human | W06,W07 |

## Phase 3: Scale (6 issues)

**Value delivered:** The pool auto-scales workers based on system
resources and workload. Conservative by default — scales up eagerly
when there's work and headroom, scales down slowly when idle.

**Critical dependency:** W09 (pool refactor) is the foundation.

| Ref | Title | Agent | Depends |
|-----|-------|-------|---------|
| W09 | Refactor pool for dynamic add/remove | code-gen | W02 |
| W10 | Scaling policy engine (thresholds) | code-gen | W01,W02,W09 |
| W11 | Autoscaler loop (policy → pool) | code-gen | W09,W10 |
| W12 | Scaling config in YAML + CLI flags | code-gen | W11 |
| W13 | Scaling events on dashboard + digest | code-gen | W06,W11 |
| W14 | Verify: auto-scaling under real load | test | W11,W12,W13 |

## Phase 4: Tune (6 issues)

**Value delivered:** Scaling decisions improve over time. Persistent
history enables trend analysis. Task profiling enables predictive
scaling. CLI and MCP tools give manual override capability.

| Ref | Title | Agent | Depends |
|-----|-------|-------|---------|
| W15 | Persist metrics + events to SQLite | code-gen | W14 |
| W16 | Task-type duration profiling | code-gen | W15 |
| W17 | `samverk scale` CLI command | code-gen | W09,W05 |
| W18 | MCP tools for scaling control | code-gen | W17 |
| W19 | ADR-032: Adaptive Worker Scaling | docs | W14 |
| W20 | 24-hour soak test | human | W15-W19 |

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    cmd/samverk/main.go                     │
│                                                           │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────┐ │
│  │  Dispatcher  │  │  Agent Pool  │  │  Autoscaler     │ │
│  │             │  │              │  │  (Phase 3)      │ │
│  │  metrics ───┼──┼─► metrics ──┼──┼─► policy.Eval() │ │
│  │  (Phase 1)  │  │  (Phase 1)  │  │  │              │ │
│  └─────────────┘  │              │  │  ▼              │ │
│                    │  Resize() ◄─┼──┼── apply()       │ │
│                    │  ScaleUp()  │  │                  │ │
│                    │  ScaleDown()│  └─────────────────┘ │
│                    │  (Phase 3)  │                       │
│                    └──────────────┘                       │
│                                                           │
│  ┌──────────────────┐  ┌───────────────────────────────┐ │
│  │ System Collector  │  │ REST API + MCP                │ │
│  │ CPU, Mem, GC      │  │ /api/v1/metrics (Phase 2)    │ │
│  │ (Phase 1)         │  │ /api/v1/scaling/* (Phase 4)  │ │
│  └──────────────────┘  │ MCP scale_* tools (Phase 4)   │ │
│                         └───────────────────────────────┘ │
│                                                           │
│  ┌──────────────────────────────────────────────────────┐ │
│  │ SQLite Store                                         │ │
│  │ scaling_events, metric_snapshots (Phase 4)           │ │
│  │ task_profiles (Phase 4)                              │ │
│  └──────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```

## Scaling Decision Logic (Phase 3)

```
Every 30 seconds:
  ├── Read system metrics (CPU%, Memory%)
  ├── Read pool metrics (active/idle workers, queue depth)
  ├── Check cooldown (skip if recently scaled)
  │
  ├── SCALE UP when:
  │   ├── Queue depth ≥ 2 AND all workers busy
  │   ├── AND CPU < 60% (room to grow)
  │   ├── AND Memory < 80% (room to grow)
  │   └── Delta: +1 worker (conservative, re-evaluate next cycle)
  │
  ├── SCALE DOWN when:
  │   ├── ≥ 2 workers idle for ≥ 120 seconds
  │   ├── AND CPU < 30%
  │   └── Delta: -1 worker (never below minimum)
  │
  └── HOLD when:
      └── Conditions ambiguous or cooldown active
```

## Critical Path

```
W01 ──► W05 ──► W06 ──► W13 ──► W14 ──► W15 ──► W20
   └──► W02 ──► W09 ──► W10 ──► W11 ──┘
```

Minimum 7 serial steps. With 3 workers and typical task durations,
estimate ~3-4 sessions to complete all 20 issues.

## Relationship to Gitea Migration

These two batches are **independent** — neither blocks the other.
The scaling work applies to whichever forge the dispatcher is pointed at.
If both batches run simultaneously, the Gitea migration benefits from
the observability (you can watch resource usage during the first
Gitea dispatch tests).

Recommended: start Gitea migration first (it's the immediate need),
begin Phase 1 of scaling in parallel since W01-W03 have zero
dependencies on anything.
