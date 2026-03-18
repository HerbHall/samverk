package dispatcher

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/pkg/models"
)

// circuitState represents whether a circuit breaker is open (tripped) or closed (healthy).
type circuitState int

const (
	circuitClosed circuitState = iota // healthy, requests pass through
	circuitOpen                       // tripped, requests blocked
)

// providerCircuit tracks per-provider failure state.
type providerCircuit struct {
	state       circuitState
	failures    int
	lastFailure time.Time
	openedAt    time.Time
}

// CircuitBreaker prevents dispatching to providers or issues that are in a
// permanent or repeating failure state. It implements two independent circuits:
//
//   - Provider circuit: after N consecutive failures from the same provider,
//     mark it unhealthy for a cooldown period. Don't route new tasks to it.
//   - Budget circuit: if any task returns a budget/credit error, pause all
//     dispatching until manually reset.
type CircuitBreaker struct {
	providers    map[string]*providerCircuit
	budgetOpen   bool
	budgetOpenAt time.Time
	mu           sync.Mutex
	logger       *zap.Logger

	// Configuration
	providerThreshold int           // consecutive failures before tripping (default: 3)
	providerCooldown  time.Duration // how long a tripped provider stays open (default: 15m)
}

// NewCircuitBreaker creates a circuit breaker with sensible defaults.
func NewCircuitBreaker(logger *zap.Logger) *CircuitBreaker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CircuitBreaker{
		providers:         make(map[string]*providerCircuit),
		logger:            logger,
		providerThreshold: 3,
		providerCooldown:  15 * time.Minute,
	}
}

// RecordFailure updates circuit breaker state based on a failure event.
// Auth and budget failures trip immediately (threshold = 1).
func (cb *CircuitBreaker) RecordFailure(fc models.FailureClass, providerKey string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch fc {
	case models.FailureClassBudget:
		cb.budgetOpen = true
		cb.budgetOpenAt = now
		cb.logger.Warn("budget circuit OPEN — all dispatching paused",
			zap.Time("opened_at", now),
		)

	case models.FailureClassAuth:
		// Auth failure trips the provider immediately.
		if providerKey != "" {
			cb.tripProvider(providerKey, now)
		}

	case models.FailureClassProviderDown, models.FailureClassPermanent,
		models.FailureClassTimeout:
		// Increment failure count; trip after threshold.
		// Timeouts indicate infrastructure issues (e.g., Ollama overloaded)
		// and consecutive occurrences should mark the provider unhealthy.
		if providerKey != "" {
			pc := cb.getOrCreate(providerKey)
			pc.failures++
			pc.lastFailure = now
			if pc.failures >= cb.providerThreshold {
				cb.tripProvider(providerKey, now)
			}
		}

	case models.FailureClassOOMKill,
		models.FailureClassShutdown, models.FailureClassPanic,
		models.FailureClassPostProcess, models.FailureClassClassify,
		models.FailureClassCycle, models.FailureClassDecompose,
		models.FailureClassUnknown:
		// These classes are tracked in failure events but don't trip circuits.
	}
}

// RecordSuccess resets the failure counter for a provider.
func (cb *CircuitBreaker) RecordSuccess(providerKey string) {
	if providerKey == "" {
		return
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	delete(cb.providers, providerKey)
}

// AllowDispatch returns true if dispatching is allowed globally.
// Returns false if the budget circuit is open.
func (cb *CircuitBreaker) AllowDispatch() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return !cb.budgetOpen
}

// AllowProvider returns true if the given provider can accept new tasks.
// A tripped provider auto-recovers after the cooldown period.
func (cb *CircuitBreaker) AllowProvider(providerKey string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	pc, exists := cb.providers[providerKey]
	if !exists {
		return true
	}
	if pc.state == circuitClosed {
		return true
	}

	// Check if cooldown has elapsed (auto-recovery).
	if time.Since(pc.openedAt) >= cb.providerCooldown {
		pc.state = circuitClosed
		pc.failures = 0
		cb.logger.Info("provider circuit auto-recovered",
			zap.String("provider", providerKey),
			zap.Duration("cooldown", cb.providerCooldown),
		)
		return true
	}

	return false
}

// ResetBudget manually resets the budget circuit breaker.
func (cb *CircuitBreaker) ResetBudget() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.budgetOpen = false
	cb.logger.Info("budget circuit RESET — dispatching resumed")
}

// Status returns a snapshot of the circuit breaker state for diagnostics.
func (cb *CircuitBreaker) Status() CircuitBreakerStatus {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	status := CircuitBreakerStatus{
		BudgetOpen:       cb.budgetOpen,
		BudgetOpenAt:     cb.budgetOpenAt,
		TrippedProviders: make(map[string]time.Time),
	}

	for key, pc := range cb.providers {
		if pc.state == circuitOpen {
			// Check auto-recovery.
			if time.Since(pc.openedAt) >= cb.providerCooldown {
				continue // already recovered
			}
			status.TrippedProviders[key] = pc.openedAt
		}
	}

	return status
}

// CircuitBreakerStatus is a read-only snapshot for API/CLI/digest output.
type CircuitBreakerStatus struct {
	BudgetOpen       bool              `json:"budget_open"`
	BudgetOpenAt     time.Time         `json:"budget_open_at,omitempty"`
	TrippedProviders map[string]time.Time `json:"tripped_providers"`
}

// tripProvider forces a provider circuit open immediately.
func (cb *CircuitBreaker) tripProvider(providerKey string, now time.Time) {
	pc := cb.getOrCreate(providerKey)
	pc.state = circuitOpen
	pc.openedAt = now
	cb.logger.Warn("provider circuit OPEN",
		zap.String("provider", providerKey),
		zap.Int("failures", pc.failures),
		zap.Duration("cooldown", cb.providerCooldown),
	)
}

// getOrCreate returns the circuit for a provider, creating it if needed.
func (cb *CircuitBreaker) getOrCreate(providerKey string) *providerCircuit {
	pc, exists := cb.providers[providerKey]
	if !exists {
		pc = &providerCircuit{}
		cb.providers[providerKey] = pc
	}
	return pc
}
