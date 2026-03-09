package scaling

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
	"time"

	"github.com/herbhall/samverk/internal/metrics"
)

// --- mock implementations ---

type mockPool struct {
	snapshot       metrics.PoolSnapshot
	addWorkersCalls atomic.Int32
	remWorkersCalls atomic.Int32
	addErr         error
	remReturn      int
}

func (m *mockPool) Snapshot() metrics.PoolSnapshot { return m.snapshot }
func (m *mockPool) AddWorkers(_ int) error {
	m.addWorkersCalls.Add(1)
	if m.addErr != nil {
		return m.addErr
	}
	m.snapshot.TotalWorkers++
	return nil
}
func (m *mockPool) RemoveWorkers(_ int) int {
	m.remWorkersCalls.Add(1)
	if m.remReturn == 0 {
		return 0
	}
	m.snapshot.TotalWorkers--
	return m.remReturn
}
func (m *mockPool) Workers() int { return m.snapshot.TotalWorkers }

type mockCollector struct{}

func (m *mockCollector) Collect() metrics.SystemSnapshot {
	return metrics.SystemSnapshot{
		CollectedAt:    time.Now(),
		HeapAllocBytes: 100_000_000,  // 100 MB
		SysBytesTotal:  1_000_000_000, // 1 GB → 10%
	}
}

// fastPolicy returns a ThresholdPolicy with near-instant triggers for testing.
func fastPolicy() *ThresholdPolicy {
	cfg := DefaultPolicyConfig()
	cfg.CooldownAfterScale = 0
	cfg.ScaleDown.IdleDuration = 0
	cfg.ScaleDown.IdleWorkerThreshold = 1
	cfg.ScaleUp.QueueDepthTrigger = 1
	cfg.ScaleUp.AllWorkersBusy = false
	cfg.EvaluationInterval = 5 * time.Millisecond
	return NewThresholdPolicy(cfg)
}

// --- autoscaler tests ---

func TestAutoscaler_Run_CancelStops(t *testing.T) {
	pool := &mockPool{snapshot: makePool(2, 1, 1, 0)}
	p := fastPolicy()
	a := NewAutoscaler(p, pool, &mockCollector{}, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("autoscaler did not stop after context cancel")
	}
}

func TestAutoscaler_Run_ScalesUp(t *testing.T) {
	// Queue depth 3, all busy — should scale up.
	pool := &mockPool{
		snapshot: metrics.PoolSnapshot{
			CollectedAt:   time.Now(),
			TotalWorkers:  2,
			ActiveWorkers: 2,
			IdleWorkers:   0,
			QueueDepth:    3,
		},
		remReturn: 1,
	}
	pool.snapshot.TotalWorkers = 2

	p := fastPolicy()
	a := NewAutoscaler(p, pool, &mockCollector{}, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = a.Run(ctx)

	if got := pool.addWorkersCalls.Load(); got == 0 {
		t.Error("expected AddWorkers to be called at least once")
	}
}

func TestAutoscaler_Run_ScalesDown(t *testing.T) {
	// 3 workers, 2 idle, empty queue — should scale down.
	pool := &mockPool{
		snapshot: metrics.PoolSnapshot{
			CollectedAt:   time.Now(),
			TotalWorkers:  3,
			ActiveWorkers: 1,
			IdleWorkers:   2,
			QueueDepth:    0,
		},
		remReturn: 1,
	}

	p := fastPolicy()
	a := NewAutoscaler(p, pool, &mockCollector{}, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = a.Run(ctx)

	if got := pool.remWorkersCalls.Load(); got == 0 {
		t.Error("expected RemoveWorkers to be called at least once")
	}
}

func TestAutoscaler_ScaleUp_PoolError(t *testing.T) {
	// Pool returns an error from AddWorkers.
	pool := &mockPool{
		snapshot: metrics.PoolSnapshot{TotalWorkers: 2, ActiveWorkers: 2, QueueDepth: 5, CollectedAt: time.Now()},
		addErr:   errors.New("pool error"),
	}
	p := fastPolicy()
	a := NewAutoscaler(p, pool, &mockCollector{}, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = a.Run(ctx)

	// AddWorkers was called but returned error — should not panic.
	if got := pool.addWorkersCalls.Load(); got == 0 {
		t.Error("expected AddWorkers to be called")
	}
}

func TestAutoscaler_ScaleDown_PoolReturnsZero(t *testing.T) {
	// Pool returns 0 (at minimum) from RemoveWorkers — should not panic.
	pool := &mockPool{
		snapshot:  metrics.PoolSnapshot{TotalWorkers: 3, IdleWorkers: 2, CollectedAt: time.Now()},
		remReturn: 0,
	}
	p := fastPolicy()
	a := NewAutoscaler(p, pool, &mockCollector{}, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = a.Run(ctx)

	if got := pool.remWorkersCalls.Load(); got == 0 {
		t.Error("expected RemoveWorkers to be called")
	}
}

func TestAutoscaler_Disabled_NoScaling(t *testing.T) {
	pool := &mockPool{
		snapshot: metrics.PoolSnapshot{TotalWorkers: 2, ActiveWorkers: 2, QueueDepth: 10, CollectedAt: time.Now()},
	}
	cfg := DefaultPolicyConfig()
	cfg.Enabled = false
	cfg.EvaluationInterval = 5 * time.Millisecond
	p := NewThresholdPolicy(cfg)
	a := NewAutoscaler(p, pool, &mockCollector{}, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = a.Run(ctx)

	if got := pool.addWorkersCalls.Load(); got != 0 {
		t.Errorf("disabled: AddWorkers called %d times, want 0", got)
	}
}

func TestNewAutoscaler_DefaultInterval(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.EvaluationInterval = 0 // zero should default to 30s
	p := NewThresholdPolicy(cfg)
	pool := &mockPool{snapshot: makePool(2, 1, 1, 0)}
	a := NewAutoscaler(p, pool, &mockCollector{}, zap.NewNop())
	if a.interval != 30*time.Second {
		t.Errorf("interval = %s, want 30s", a.interval)
	}
}
