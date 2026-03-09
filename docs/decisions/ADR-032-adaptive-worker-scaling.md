# ADR-032: Adaptive Worker Scaling

## Status

Accepted

## Context

Samverk runs background agents against a pool of goroutine workers.
The workload is uneven: quiet periods with a few open issues alternate
with active sprints where multiple agents run concurrently. The runtime
environment is a resource-constrained LXC container (CT 202, 2 vCPUs,
2 GB RAM) running on a home Proxmox host.

Static pool sizing forces a bad trade-off: a large pool wastes memory
during quiet periods; a small pool serialises work during sprints. The
developer is often away when workload spikes occur, so manual resizing
is impractical.

## Decision

Samverk implements a closed-loop adaptive worker scaling subsystem with
four phases: collect → evaluate → act → record.

### Why threshold-based policy, not ML-based

Threshold policy (`ThresholdPolicy`) was chosen over ML or predictive
approaches because:

- **Solo dev context** — no operational team to tune model hyperparameters
  or respond to prediction drift.
- **Debuggability** — a human (or the Samverk operator) can read the
  threshold config file and immediately understand why the pool grew or
  shrank. ML models are opaque.
- **Simplicity** — the workload signal is already available (queue depth,
  idle count). Fitting a model to the same signal adds complexity without
  measurable accuracy gain at this scale.
- **Correctness ceiling is low** — wrong scaling decisions cost milliseconds
  of latency, not money or data. Over-provisioning by one or two workers
  is harmless.

If workload patterns become predictable and tuning takes excessive manual
effort, ML-based scaling can replace `ThresholdPolicy` behind the same
`Policy` interface without touching the autoscaler or pool.

### Why conservative scaling (eager up, slow down)

The defaults are intentionally asymmetric:

- **Scale up immediately** when queue depth exceeds the trigger threshold
  and all workers are busy.
- **Scale down slowly** by waiting for an idle duration before removing a
  worker.

Rationale: under-provisioning blocks issue processing; over-provisioning
by a few workers wastes a modest amount of RAM but unblocks work immediately.
The LXC container has headroom for 4-6 workers above the minimum. Rapid
scale-down wastes the warm-up cost of spawning a worker only to remove it
seconds later.

### Architecture

```text
SystemCollector → ThresholdPolicy → Autoscaler → Pool
                                         ↓
                               ScalingEventPersister → SQLite
                                         ↑
                               ScalingControlReader ← REST API / MCP / CLI
```

Key design properties:

- **Interface-segregated**: `Policy`, `PoolScaler`, `SystemCollector`,
  `ScalingControlReader`, and `ScalingEventPersister` are all independent
  interfaces. Each can be tested or replaced in isolation.
- **Cross-process signaling via SQLite**: the `samverk serve` process
  writes pause/resume/manual-override signals to a single-row
  `scaling_control` table. The `samverk dispatch` autoscaler reads it
  before each evaluation cycle. No IPC, no shared memory.
- **Control plane**: REST API (`/api/v1/scaling/*`), MCP tools
  (`scale_pause`, `scale_resume`, `scale_set`), and CLI (`samverk scale`)
  all write to the same table and take effect on the next evaluation tick.
- **Audit log**: every scale-up, scale-down, and manual-override action
  writes a `ScalingEvent` to SQLite with timestamp, reason, worker delta,
  and confidence. Events are surfaced via `samverk scale history` and the
  dashboard.

### Operational guidelines

**Recommended starting configuration** (LXC CT 202, 2 vCPUs, 2 GB RAM):

```yaml
scaling:
  enabled: true
  min_workers: 1
  max_workers: 4
  evaluation_interval: 30s
  cooldown_after_scale: 60s
  scale_up:
    queue_depth_trigger: 2
    all_workers_busy: true
  scale_down:
    idle_worker_threshold: 2
    idle_duration: 5m
```

**Tuning guidance**:

- If the pool oscillates (up then down quickly), increase `cooldown_after_scale`
  and `idle_duration`. The defaults (60 s / 5 min) prevent most oscillation.
- If work queues for more than one evaluation cycle before scaling, lower
  `queue_depth_trigger` to 1 or disable the `all_workers_busy` guard.
- If the container runs out of RAM, lower `max_workers`. Each goroutine
  worker consumes approximately 4-8 MB at rest plus model API call memory.
- If issues are timing-sensitive, lower `evaluation_interval` to 10-15 s.

**Manual override**:

Use `samverk scale set <N>` or the `scale_set` MCP tool to pin the pool
to a specific size. This pauses autonomous scaling. Use `samverk scale resume`
or `scale_resume` to re-enable autonomous scaling after the manual session.
Manual override is useful during active pairing sessions where the developer
wants deterministic behaviour.

### Future directions

- **Task-type profiling**: track average duration and token cost per
  issue label. Scale up earlier for complex issues and later for
  documentation tasks.
- **Predictive scaling**: if cron-pattern workload emerges (e.g., issue
  spikes at end of work day), pre-warm the pool before the spike.
- **Multi-node pool**: when work grows beyond a single container, the
  `PoolScaler` interface can be implemented for a distributed pool
  without changing the autoscaler.

## Consequences

- The pool is always right-sized for the current workload without
  developer intervention.
- Scaling decisions are transparent: events are logged and queryable.
- Manual override is always available with a single CLI command or
  MCP tool call.
- Adding a new scaling policy requires implementing the `Policy`
  interface and updating the factory in `scaling/config.go`. No other
  changes are needed.
- Threshold config is validated at startup; invalid configurations fail
  fast with descriptive errors.

## References

- [W01-W18 implementation issues](https://github.com/HerbHall/samverk/issues?q=label%3Aw-track)
- [ADR-014: Dispatcher Agent](ADR-014-dispatcher-agent.md)
- [ADR-019: Self-Hosted-First Development](ADR-019-self-hosted-first.md)
- [ADR-027: Failure Recovery Strategy](ADR-027-failure-recovery.md)
- [internal/scaling package](../../internal/scaling/)
