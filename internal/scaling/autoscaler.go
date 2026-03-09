package scaling

import (
	"context"
	"log/slog"
	"time"

	"github.com/herbhall/samverk/internal/metrics"
	"github.com/herbhall/samverk/pkg/models"
)

// PoolScaler is the interface the autoscaler uses to inspect and resize the pool.
// agent.Pool satisfies this interface.
type PoolScaler interface {
	Snapshot() metrics.PoolSnapshot
	AddWorkers(n int) error
	RemoveWorkers(n int) int
	Workers() int
}

// ScalingControlReader reads the current manual scaling override.
// store.Store satisfies this interface.
type ScalingControlReader interface {
	GetScalingControl(ctx context.Context) (*models.ScalingControl, error)
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
	policy        *ThresholdPolicy
	pool          PoolScaler
	collector     SystemCollector
	interval      time.Duration
	logger        *slog.Logger
	events        *EventBuffer
	persister     EventPersister      // optional; nil means no durable storage
	controlReader ScalingControlReader // optional; nil means no override support
}

// NewAutoscaler creates an Autoscaler wired to the given policy, pool, and
// system collector. Scaling events are stored in an internal rolling buffer
// (capacity 100) accessible via Events(). Call SetPersister to additionally
// write events to durable storage.
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
		events:    NewEventBuffer(100),
	}
}

// SetPersister attaches an optional durable store for scaling events.
// When set, each scale event is also saved via p.SaveScalingEvent so events
// survive process restarts and are visible to other processes (e.g. serve).
func (a *Autoscaler) SetPersister(p EventPersister) {
	a.persister = p
}

// SetControlReader attaches an optional store for reading manual scaling overrides.
// When set, the autoscaler checks for pause/set commands before each evaluation.
func (a *Autoscaler) SetControlReader(r ScalingControlReader) {
	a.controlReader = r
}

// Events returns recent scaling events from the in-memory buffer, newest first.
func (a *Autoscaler) Events() []models.ScalingEvent {
	return a.events.Events()
}

// PolicyConfig returns the policy configuration used by this autoscaler.
func (a *Autoscaler) PolicyConfig() PolicyConfig {
	return a.policy.config
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
	// Check for manual override before running the policy.
	if a.controlReader != nil {
		ctrl, ctrlErr := a.controlReader.GetScalingControl(context.Background())
		if ctrlErr != nil {
			a.logger.Warn("autoscaler: failed to read scaling control", slog.String("error", ctrlErr.Error()))
		} else if ctrl != nil {
			if ctrl.Paused {
				a.logger.Debug("autoscaler paused via manual override")
				return
			}
			if ctrl.ManualWorkers > 0 {
				a.applyManualOverride(ctrl.ManualWorkers, ctrl.Note)
				return
			}
		}
	}

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

// applyManualOverride adjusts the pool to the requested worker count.
func (a *Autoscaler) applyManualOverride(targetWorkers int, note string) {
	current := a.pool.Workers()
	if current == targetWorkers {
		return
	}
	delta := targetWorkers - current
	reason := "manual override"
	if note != "" {
		reason = "manual override: " + note
	}
	if delta > 0 {
		if err := a.pool.AddWorkers(delta); err != nil {
			a.logger.Warn("autoscaler manual override scale-up failed", slog.String("error", err.Error()))
			return
		}
	} else {
		a.pool.RemoveWorkers(-delta)
	}
	newCount := a.pool.Workers()
	a.logger.Info("autoscaler manual override applied",
		slog.Int("old_workers", current),
		slog.Int("new_workers", newCount),
		slog.String("reason", reason),
	)
	e := models.ScalingEvent{
		Timestamp:   time.Now(),
		Action:      "manual-override",
		FromWorkers: current,
		ToWorkers:   newCount,
		Reason:      reason,
		Confidence:  1.0,
	}
	a.events.Add(e)
	a.persist(e)
}

// persist saves e to the optional durable store. Errors are logged but not fatal.
func (a *Autoscaler) persist(e models.ScalingEvent) {
	if a.persister == nil {
		return
	}
	if err := a.persister.SaveScalingEvent(context.Background(), e); err != nil {
		a.logger.Warn("autoscaler: failed to persist scaling event", slog.String("error", err.Error()))
	}
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
		e := models.ScalingEvent{
			Timestamp:   time.Now(),
			Action:      ScaleUp.String(),
			FromWorkers: oldCount,
			ToWorkers:   newCount,
			Reason:      d.Reason,
			Confidence:  d.Confidence,
		}
		a.events.Add(e)
		a.persist(e)
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
		e := models.ScalingEvent{
			Timestamp:   time.Now(),
			Action:      ScaleDown.String(),
			FromWorkers: oldCount,
			ToWorkers:   newCount,
			Reason:      d.Reason,
			Confidence:  d.Confidence,
		}
		a.events.Add(e)
		a.persist(e)
		a.policy.NotifyScaled()
	}
}
