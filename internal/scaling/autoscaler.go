package scaling

import (
	"context"
	"log/slog"
	"time"

	"github.com/herbhall/samverk/internal/metrics"
)

// PoolScaler is the interface the autoscaler uses to inspect and resize the pool.
// agent.Pool satisfies this interface.
type PoolScaler interface {
	Snapshot() metrics.PoolSnapshot
	AddWorkers(n int) error
	RemoveWorkers(n int) int
}

// SystemCollector provides runtime system snapshots.
// metrics.SystemCollector satisfies this interface.
type SystemCollector interface {
	Collect() metrics.SystemSnapshot
}

// Autoscaler is a long-running goroutine that periodically evaluates the scaling
// policy and applies the resulting decision to the pool.
// Start it with Run(ctx) and cancel the context to shut it down.
type Autoscaler struct {
	policy    *ThresholdPolicy
	pool      PoolScaler
	collector SystemCollector
	interval  time.Duration
	logger    *slog.Logger
}

// NewAutoscaler creates an Autoscaler wired to the given policy, pool, and
// system collector.
func NewAutoscaler(policy *ThresholdPolicy, pool PoolScaler, collector SystemCollector) *Autoscaler {
	interval := policy.config.EvaluationInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Autoscaler{
		policy:    policy,
		pool:      pool,
		collector: collector,
		interval:  interval,
		logger:    slog.Default(),
	}
}

// Run starts the evaluation loop. It blocks until ctx is cancelled, then returns
// ctx.Err(). The loop evaluates the policy at the configured interval and applies
// any non-Hold decision to the pool.
func (a *Autoscaler) Run(ctx context.Context) error {
	a.logger.Info("autoscaler started", slog.Duration("interval", a.interval))
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.logger.Info("autoscaler stopped")
			return ctx.Err()
		case <-ticker.C:
			a.evaluate()
		}
	}
}

// evaluate runs one policy cycle and applies the decision.
func (a *Autoscaler) evaluate() {
	poolSnap := a.pool.Snapshot()
	sysSnap := a.collector.Collect()
	decision := a.policy.Evaluate(poolSnap, sysSnap)

	if decision.Action == Hold {
		a.logger.Debug("autoscaler hold",
			slog.String("reason", decision.Reason),
			slog.Float64("confidence", decision.Confidence),
		)
		return
	}

	oldCount := poolSnap.TotalWorkers
	a.apply(decision, oldCount)
}

// apply executes a non-Hold scaling decision.
func (a *Autoscaler) apply(d Decision, oldCount int) {
	switch d.Action {
	case Hold:
		return
	case ScaleUp:
		if err := a.pool.AddWorkers(d.Delta); err != nil {
			a.logger.Warn("autoscaler scale-up failed",
				slog.String("reason", d.Reason),
				slog.String("error", err.Error()),
			)
			return
		}
		newCount := a.pool.Snapshot().TotalWorkers
		a.logger.Info("autoscaler scaled up",
			slog.Int("old_workers", oldCount),
			slog.Int("new_workers", newCount),
			slog.Int("delta", d.Delta),
			slog.String("reason", d.Reason),
			slog.Float64("confidence", d.Confidence),
		)
		a.policy.NotifyScaled()

	case ScaleDown:
		sent := a.pool.RemoveWorkers(d.Delta)
		if sent == 0 {
			a.logger.Warn("autoscaler scale-down had no effect (at minimum workers)",
				slog.String("reason", d.Reason),
			)
			return
		}
		newCount := a.pool.Snapshot().TotalWorkers
		a.logger.Info("autoscaler scaled down",
			slog.Int("old_workers", oldCount),
			slog.Int("new_workers", newCount),
			slog.Int("delta", sent),
			slog.String("reason", d.Reason),
			slog.Float64("confidence", d.Confidence),
		)
		a.policy.NotifyScaled()
	}
}
