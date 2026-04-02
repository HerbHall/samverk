package scaling

import (
	"strings"
	"testing"
	"time"

	"samverk.dev/samverk/internal/metrics"
)

// makePool builds a PoolSnapshot for testing.
func makePool(total, active, idle, queue int) metrics.PoolSnapshot {
	return metrics.PoolSnapshot{
		CollectedAt:   time.Now(),
		TotalWorkers:  total,
		ActiveWorkers: active,
		IdleWorkers:   idle,
		QueueDepth:    queue,
	}
}

// makePoolAt builds a PoolSnapshot with an explicit collection time.
func makePoolAt(t time.Time, total, active, idle, queue int) metrics.PoolSnapshot {
	return metrics.PoolSnapshot{
		CollectedAt:   t,
		TotalWorkers:  total,
		ActiveWorkers: active,
		IdleWorkers:   idle,
		QueueDepth:    queue,
	}
}

// makeSys builds a SystemSnapshot with a given heap/sys memory ratio.
// heapPct is the desired heap-to-sys percentage (0–100).
func makeSys(heapPct float64) metrics.SystemSnapshot {
	const sysBytesTotal = 1_000_000_000 // 1 GB
	heap := uint64(heapPct / 100 * sysBytesTotal)
	return metrics.SystemSnapshot{
		CollectedAt:    time.Now(),
		HeapAllocBytes: heap,
		SysBytesTotal:  sysBytesTotal,
	}
}

// defaultPolicy creates a ThresholdPolicy with DefaultPolicyConfig values
// tweaked for fast tests (no real-time waiting).
func defaultPolicy() *ThresholdPolicy {
	cfg := DefaultPolicyConfig()
	cfg.CooldownAfterScale = 5 * time.Minute // keep cooldown out of the way
	cfg.ScaleDown.IdleDuration = 2 * time.Minute
	return NewThresholdPolicy(cfg)
}

// --- Disabled policy ---

func TestThresholdPolicy_Disabled(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.Enabled = false
	p := NewThresholdPolicy(cfg)

	d := p.Evaluate(makePool(2, 2, 0, 5), makeSys(20))
	if d.Action != Hold {
		t.Errorf("disabled: got action %s, want Hold", d.Action)
	}
	if !strings.Contains(d.Reason, "disabled") {
		t.Errorf("disabled reason %q should mention 'disabled'", d.Reason)
	}
	if d.Confidence != 1.0 {
		t.Errorf("disabled: confidence = %.2f, want 1.0", d.Confidence)
	}
}

// --- Scale-up cases ---

func TestThresholdPolicy_ScaleUp_QueueAndAllBusy(t *testing.T) {
	p := defaultPolicy()
	// 2 workers, all busy, queue depth 3 (>= 2), memory 20% (< 80%).
	d := p.Evaluate(makePool(2, 2, 0, 3), makeSys(20))
	if d.Action != ScaleUp {
		t.Errorf("got action %s, want ScaleUp; reason: %s", d.Action, d.Reason)
	}
	if d.Delta != 1 {
		t.Errorf("delta = %d, want 1", d.Delta)
	}
	if d.Confidence <= 0 || d.Confidence > 1 {
		t.Errorf("confidence = %.2f, want (0,1]", d.Confidence)
	}
}

func TestThresholdPolicy_NoScaleUp_QueueBelowTrigger(t *testing.T) {
	p := defaultPolicy()
	// Queue depth 1 (< trigger 2) — should not scale up.
	d := p.Evaluate(makePool(2, 2, 0, 1), makeSys(20))
	if d.Action == ScaleUp {
		t.Errorf("queue 1 < trigger 2: got ScaleUp, want Hold; reason: %s", d.Reason)
	}
}

func TestThresholdPolicy_NoScaleUp_MemoryPressure(t *testing.T) {
	p := defaultPolicy()
	// Memory 85% (> threshold 80%), queue full — memory pressure should block scale-up.
	d := p.Evaluate(makePool(2, 2, 0, 5), makeSys(85))
	if d.Action == ScaleUp {
		t.Errorf("high memory: got ScaleUp, want Hold; reason: %s", d.Reason)
	}
}

func TestThresholdPolicy_NoScaleUp_AlreadyAtMax(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.MaxWorkers = 3
	p := NewThresholdPolicy(cfg)
	// At max workers — should not scale up.
	d := p.Evaluate(makePool(3, 3, 0, 10), makeSys(10))
	if d.Action == ScaleUp {
		t.Errorf("at max workers: got ScaleUp, want Hold; reason: %s", d.Reason)
	}
}

func TestThresholdPolicy_NoScaleUp_NotAllWorkersBusy(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.ScaleUp.AllWorkersBusy = true
	p := NewThresholdPolicy(cfg)
	// Queue has work but 1 worker is idle — all_workers_busy blocks scale-up.
	d := p.Evaluate(makePool(3, 2, 1, 5), makeSys(10))
	if d.Action == ScaleUp {
		t.Errorf("idle worker present: got ScaleUp, want Hold; reason: %s", d.Reason)
	}
}

func TestThresholdPolicy_ScaleUp_AllWorkersBusy_Disabled(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.ScaleUp.AllWorkersBusy = false
	p := NewThresholdPolicy(cfg)
	// Queue has work, 1 worker idle, but AllWorkersBusy=false so still scale up.
	d := p.Evaluate(makePool(3, 2, 1, 5), makeSys(10))
	if d.Action != ScaleUp {
		t.Errorf("AllWorkersBusy=false: got %s, want ScaleUp; reason: %s", d.Action, d.Reason)
	}
}

// --- Scale-down cases ---

func TestThresholdPolicy_ScaleDown_SustainedIdle(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.ScaleDown.IdleWorkerThreshold = 2
	cfg.ScaleDown.IdleDuration = 10 * time.Second
	cfg.CooldownAfterScale = 0
	p := NewThresholdPolicy(cfg)

	base := time.Now()

	// First eval: idle count meets threshold, start the clock.
	p.Evaluate(makePoolAt(base, 4, 1, 3, 0), makeSys(10))

	// Second eval: 15s later — idle duration exceeded.
	d := p.Evaluate(makePoolAt(base.Add(15*time.Second), 4, 1, 3, 0), makeSys(10))
	if d.Action != ScaleDown {
		t.Errorf("sustained idle: got %s, want ScaleDown; reason: %s", d.Action, d.Reason)
	}
	if d.Delta != 1 {
		t.Errorf("delta = %d, want 1", d.Delta)
	}
}

func TestThresholdPolicy_NoScaleDown_IdleDurationNotMet(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.ScaleDown.IdleWorkerThreshold = 2
	cfg.ScaleDown.IdleDuration = 60 * time.Second
	cfg.CooldownAfterScale = 0
	p := NewThresholdPolicy(cfg)

	base := time.Now()
	p.Evaluate(makePoolAt(base, 4, 1, 3, 0), makeSys(10))
	// Only 5s elapsed — not enough.
	d := p.Evaluate(makePoolAt(base.Add(5*time.Second), 4, 1, 3, 0), makeSys(10))
	if d.Action == ScaleDown {
		t.Errorf("idle 5s < 60s: got ScaleDown, want Hold; reason: %s", d.Reason)
	}
}

func TestThresholdPolicy_NoScaleDown_BelowIdleThreshold(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.ScaleDown.IdleWorkerThreshold = 3
	cfg.CooldownAfterScale = 0
	p := NewThresholdPolicy(cfg)
	// Only 2 idle workers (< threshold 3).
	d := p.Evaluate(makePool(4, 2, 2, 0), makeSys(10))
	if d.Action == ScaleDown {
		t.Errorf("2 idle < threshold 3: got ScaleDown, want Hold; reason: %s", d.Reason)
	}
}

func TestThresholdPolicy_NoScaleDown_AtMinWorkers(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.MinWorkers = 2
	cfg.ScaleDown.IdleWorkerThreshold = 1
	cfg.ScaleDown.IdleDuration = 0
	cfg.CooldownAfterScale = 0
	p := NewThresholdPolicy(cfg)
	// At min workers — cannot scale down.
	d := p.Evaluate(makePool(2, 0, 2, 0), makeSys(10))
	if d.Action == ScaleDown {
		t.Errorf("at min workers: got ScaleDown, want Hold; reason: %s", d.Reason)
	}
}

func TestThresholdPolicy_ScaleDown_IdleResetOnBusyPeriod(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.ScaleDown.IdleWorkerThreshold = 2
	cfg.ScaleDown.IdleDuration = 30 * time.Second
	cfg.CooldownAfterScale = 0
	p := NewThresholdPolicy(cfg)

	base := time.Now()
	// Start idle tracking.
	p.Evaluate(makePoolAt(base, 4, 1, 3, 0), makeSys(10))
	// Work arrives — workers become busy, idle drops below threshold.
	p.Evaluate(makePoolAt(base.Add(10*time.Second), 4, 4, 0, 5), makeSys(10))
	// Idle again — but clock should have reset.
	p.Evaluate(makePoolAt(base.Add(20*time.Second), 4, 1, 3, 0), makeSys(10))
	// 15s from the reset — not enough yet.
	d := p.Evaluate(makePoolAt(base.Add(35*time.Second), 4, 1, 3, 0), makeSys(10))
	if d.Action == ScaleDown {
		t.Errorf("idle clock should have reset: got ScaleDown; reason: %s", d.Reason)
	}
}

// --- Cooldown ---

func TestThresholdPolicy_Cooldown(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.CooldownAfterScale = 5 * time.Minute
	p := NewThresholdPolicy(cfg)
	p.NotifyScaled()

	// Scale-up conditions are met but we're in cooldown.
	d := p.Evaluate(makePool(2, 2, 0, 5), makeSys(10))
	if d.Action != Hold {
		t.Errorf("cooldown: got %s, want Hold; reason: %s", d.Action, d.Reason)
	}
	if !strings.Contains(d.Reason, "cooldown") {
		t.Errorf("cooldown reason %q should mention 'cooldown'", d.Reason)
	}
}

func TestThresholdPolicy_CooldownExpires(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.CooldownAfterScale = 10 * time.Millisecond
	p := NewThresholdPolicy(cfg)
	p.NotifyScaled()
	time.Sleep(20 * time.Millisecond)

	// Cooldown should have expired — scale-up conditions met.
	d := p.Evaluate(makePool(2, 2, 0, 5), makeSys(10))
	if d.Action != ScaleUp {
		t.Errorf("cooldown expired: got %s, want ScaleUp; reason: %s", d.Action, d.Reason)
	}
}

// --- Hold cases ---

func TestThresholdPolicy_Hold_QueueEmptyAllIdle(t *testing.T) {
	p := defaultPolicy()
	// No queue, no busy workers — nothing to do.
	d := p.Evaluate(makePool(3, 0, 3, 0), makeSys(10))
	if d.Action == ScaleUp {
		t.Errorf("empty queue + all idle: got ScaleUp; reason: %s", d.Reason)
	}
}

func TestThresholdPolicy_Hold_Stable(t *testing.T) {
	p := defaultPolicy()
	// Some active workers, small queue, reasonable memory — no action.
	d := p.Evaluate(makePool(3, 2, 1, 1), makeSys(30))
	if d.Action != Hold {
		t.Errorf("stable: got %s, want Hold; reason: %s", d.Action, d.Reason)
	}
	if d.Reason == "" {
		t.Error("reason should not be empty")
	}
}

// --- Config validation ---

func TestNewThresholdPolicy_ClampsInvalidConfig(t *testing.T) {
	cfg := PolicyConfig{MinWorkers: -1, MaxWorkers: 0}
	p := NewThresholdPolicy(cfg)
	if p.config.MinWorkers != 1 {
		t.Errorf("MinWorkers = %d, want 1", p.config.MinWorkers)
	}
	if p.config.MaxWorkers != 8 {
		t.Errorf("MaxWorkers = %d, want 8", p.config.MaxWorkers)
	}
}

func TestNewThresholdPolicy_ClampsMinGtMax(t *testing.T) {
	cfg := PolicyConfig{MinWorkers: 5, MaxWorkers: 2}
	p := NewThresholdPolicy(cfg)
	if p.config.MaxWorkers < p.config.MinWorkers {
		t.Errorf("MaxWorkers %d < MinWorkers %d after clamp", p.config.MaxWorkers, p.config.MinWorkers)
	}
}

// --- Memory pressure ---

func TestThresholdPolicy_MemoryPct_ZeroSys(t *testing.T) {
	sys := metrics.SystemSnapshot{HeapAllocBytes: 1000, SysBytesTotal: 0}
	if got := memoryPct(sys); got != 0 {
		t.Errorf("memoryPct with zero SysBytes = %.2f, want 0", got)
	}
}

func TestThresholdPolicy_NoScaleUp_ExactMemoryAtThreshold(t *testing.T) {
	cfg := DefaultPolicyConfig()
	cfg.ScaleUp.MemoryThreshold = 80
	p := NewThresholdPolicy(cfg)
	// Memory exactly at threshold (80%) — scale-up forbidden (< not <=).
	d := p.Evaluate(makePool(2, 2, 0, 5), makeSys(80))
	if d.Action == ScaleUp {
		t.Errorf("memory exactly at threshold: got ScaleUp, want Hold; reason: %s", d.Reason)
	}
}

// --- Action.String ---

func TestAction_String(t *testing.T) {
	tests := []struct{ a Action; want string }{
		{Hold, "hold"},
		{ScaleUp, "scale-up"},
		{ScaleDown, "scale-down"},
		{Action(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.a.String(); got != tt.want {
			t.Errorf("Action(%d).String() = %q, want %q", tt.a, got, tt.want)
		}
	}
}
