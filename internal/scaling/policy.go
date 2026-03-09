// Package scaling provides the policy engine for adaptive worker scaling.
// The policy evaluates system and pool metrics and produces scaling decisions;
// it does not execute them. Separation keeps the logic testable in isolation.
package scaling

import (
	"fmt"
	"sync"
	"time"

	"github.com/herbhall/samverk/internal/metrics"
)

// Action represents a scaling decision type.
type Action int

const (
	// Hold means no change to worker count.
	Hold Action = iota
	// ScaleUp means add workers.
	ScaleUp
	// ScaleDown means remove workers.
	ScaleDown
)

// String returns a human-readable name for the action.
func (a Action) String() string {
	switch a {
	case Hold:
		return "hold"
	case ScaleUp:
		return "scale-up"
	case ScaleDown:
		return "scale-down"
	default:
		return "unknown"
	}
}

// Decision is the output of a policy evaluation.
type Decision struct {
	// Action is the recommended scaling action.
	Action Action
	// Delta is the number of workers to add (ScaleUp) or remove (ScaleDown).
	// Zero for Hold decisions.
	Delta int
	// Reason is a human-readable explanation suitable for logs and the dashboard.
	Reason string
	// Confidence is a 0–1 score reflecting how many signals agree.
	// Higher means more signals agree; lower means ambiguous.
	Confidence float64
}

// Policy evaluates metrics and returns a scaling Decision.
type Policy interface {
	Evaluate(pool metrics.PoolSnapshot, sys metrics.SystemSnapshot) Decision
}

// ScaleUpConfig holds scale-up thresholds.
type ScaleUpConfig struct {
	// MemoryThreshold is the maximum memory-usage percentage (heap/sys*100) below
	// which scale-up is permitted. Protects against scaling under memory pressure.
	MemoryThreshold int `yaml:"memory_threshold"`
	// QueueDepthTrigger is the minimum queue depth that triggers a scale-up check.
	QueueDepthTrigger int `yaml:"queue_depth_trigger"`
	// AllWorkersBusy, when true, requires all workers to be active before scaling up.
	AllWorkersBusy bool `yaml:"all_workers_busy"`
}

// ScaleDownConfig holds scale-down thresholds.
type ScaleDownConfig struct {
	// IdleWorkerThreshold is the minimum number of idle workers required to consider
	// scaling down.
	IdleWorkerThreshold int `yaml:"idle_worker_threshold"`
	// IdleDuration is how long idle workers must exceed IdleWorkerThreshold before
	// scale-down is triggered. Prevents premature scale-down during brief lulls.
	IdleDuration time.Duration `yaml:"idle_duration"`
}

// PolicyConfig holds all configurable thresholds for ThresholdPolicy.
type PolicyConfig struct {
	// Enabled controls whether the policy produces non-Hold decisions.
	// When false, Evaluate always returns Hold.
	Enabled bool `yaml:"enabled"`
	// MinWorkers is the minimum worker count the policy will recommend.
	MinWorkers int `yaml:"min_workers"`
	// MaxWorkers is the maximum worker count the policy will recommend.
	MaxWorkers int `yaml:"max_workers"`
	// EvaluationInterval is a hint for the autoscaler loop; not enforced here.
	EvaluationInterval time.Duration `yaml:"evaluation_interval"`
	// CooldownAfterScale is the minimum time between consecutive scale events.
	// Prevents oscillation.
	CooldownAfterScale time.Duration `yaml:"cooldown_after_scale"`
	// ScaleUp holds the scale-up thresholds.
	ScaleUp ScaleUpConfig `yaml:"scale_up"`
	// ScaleDown holds the scale-down thresholds.
	ScaleDown ScaleDownConfig `yaml:"scale_down"`
}

// DefaultPolicyConfig returns a conservative default configuration.
func DefaultPolicyConfig() PolicyConfig {
	return PolicyConfig{
		Enabled:            true,
		MinWorkers:         1,
		MaxWorkers:         8,
		EvaluationInterval: 30 * time.Second,
		CooldownAfterScale: 2 * time.Minute,
		ScaleUp: ScaleUpConfig{
			MemoryThreshold:   80,
			QueueDepthTrigger: 2,
			AllWorkersBusy:    true,
		},
		ScaleDown: ScaleDownConfig{
			IdleWorkerThreshold: 2,
			IdleDuration:        2 * time.Minute,
		},
	}
}

// ThresholdPolicy is a Policy implementation driven by configurable thresholds.
// It is safe for concurrent use.
type ThresholdPolicy struct {
	config       PolicyConfig
	mu           sync.Mutex
	lastScale    time.Time // time of the most recent scale event
	idleStart    time.Time // when idle-worker count first exceeded the threshold
	hasIdleStart bool      // true if idleStart is valid
}

// NewThresholdPolicy creates a ThresholdPolicy with the given configuration.
// Invalid values are silently clamped to safe defaults (min/max bounds).
func NewThresholdPolicy(cfg PolicyConfig) *ThresholdPolicy {
	if cfg.MinWorkers <= 0 {
		cfg.MinWorkers = 1
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 8
	}
	if cfg.MinWorkers > cfg.MaxWorkers {
		cfg.MaxWorkers = cfg.MinWorkers
	}
	return &ThresholdPolicy{config: cfg}
}

// NotifyScaled records the time of a scale event for cooldown tracking.
// The autoscaler calls this after it applies a decision.
func (p *ThresholdPolicy) NotifyScaled() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastScale = time.Now()
}

// Evaluate returns the recommended scaling decision given current metrics.
// It considers queue depth, worker utilisation, memory pressure, idle duration,
// and the cooldown period.
func (p *ThresholdPolicy) Evaluate(pool metrics.PoolSnapshot, sys metrics.SystemSnapshot) Decision {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.config.Enabled {
		return Decision{Action: Hold, Reason: "scaling disabled", Confidence: 1.0}
	}

	now := pool.CollectedAt
	if now.IsZero() {
		now = time.Now()
	}

	// Cooldown: do nothing if we scaled recently.
	if !p.lastScale.IsZero() && now.Sub(p.lastScale) < p.config.CooldownAfterScale {
		remaining := p.config.CooldownAfterScale - now.Sub(p.lastScale)
		return Decision{
			Action:     Hold,
			Reason:     fmt.Sprintf("cooldown active, %s remaining", remaining.Truncate(time.Second)),
			Confidence: 1.0,
		}
	}

	memPct := memoryPct(sys)
	currentWorkers := pool.TotalWorkers

	// --- Scale-up evaluation ---
	if currentWorkers < p.config.MaxWorkers {
		upSignals := 0
		totalUpSignals := 3

		queueTriggered := pool.QueueDepth >= p.config.ScaleUp.QueueDepthTrigger
		if queueTriggered {
			upSignals++
		}
		allBusy := pool.IdleWorkers == 0
		if !p.config.ScaleUp.AllWorkersBusy || allBusy {
			upSignals++
		}
		memOK := memPct < float64(p.config.ScaleUp.MemoryThreshold)
		if memOK {
			upSignals++
		}

		if queueTriggered && memOK && (!p.config.ScaleUp.AllWorkersBusy || allBusy) {
			// Reset idle tracking since we're scaling up.
			p.hasIdleStart = false
			confidence := float64(upSignals) / float64(totalUpSignals)
			return Decision{
				Action:     ScaleUp,
				Delta:      1,
				Reason:     fmt.Sprintf("queue depth %d >= %d, all-busy=%v, memory %.0f%% < %d%%", pool.QueueDepth, p.config.ScaleUp.QueueDepthTrigger, allBusy, memPct, p.config.ScaleUp.MemoryThreshold),
				Confidence: confidence,
			}
		}
	}

	// --- Scale-down evaluation ---
	if currentWorkers > p.config.MinWorkers {
		idleAboveThreshold := pool.IdleWorkers >= p.config.ScaleDown.IdleWorkerThreshold
		if idleAboveThreshold {
			if !p.hasIdleStart {
				p.idleStart = now
				p.hasIdleStart = true
			}
			idleFor := now.Sub(p.idleStart)
			if idleFor >= p.config.ScaleDown.IdleDuration {
				p.hasIdleStart = false // reset for next cycle
				signals := 2          // idle count + duration met
				if pool.QueueDepth == 0 {
					signals++
				}
				confidence := float64(signals) / 3.0
				return Decision{
					Action:     ScaleDown,
					Delta:      1,
					Reason:     fmt.Sprintf("%d idle workers (>= %d) for %s (>= %s), queue=%d", pool.IdleWorkers, p.config.ScaleDown.IdleWorkerThreshold, idleFor.Truncate(time.Second), p.config.ScaleDown.IdleDuration, pool.QueueDepth),
					Confidence: confidence,
				}
			}
			// Idle threshold met but not for long enough yet.
			remaining := p.config.ScaleDown.IdleDuration - idleFor
			return Decision{
				Action:     Hold,
				Reason:     fmt.Sprintf("%d idle workers, waiting %s more before scale-down", pool.IdleWorkers, remaining.Truncate(time.Second)),
				Confidence: 0.5,
			}
		}
	}

	// Reset idle tracking since we're below the threshold.
	p.hasIdleStart = false

	return Decision{
		Action:     Hold,
		Reason:     "conditions stable",
		Confidence: 0.8,
	}
}

// memoryPct computes heap allocation as a percentage of total system memory
// obtained by the Go runtime. This is a conservative proxy for memory pressure:
// a high ratio indicates GC is working hard and adding more workers may worsen it.
func memoryPct(sys metrics.SystemSnapshot) float64 {
	if sys.SysBytesTotal == 0 {
		return 0
	}
	return float64(sys.HeapAllocBytes) / float64(sys.SysBytesTotal) * 100
}
