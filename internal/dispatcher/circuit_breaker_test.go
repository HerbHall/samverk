package dispatcher

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"samverk.dev/samverk/pkg/models"
)

func TestCircuitBreaker_TimeoutTripsAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(zap.NewNop())

	providerKey := "ollama-vm300"

	// First two timeouts should not trip the circuit.
	cb.RecordFailure(models.FailureClassTimeout, providerKey)
	if !cb.AllowProvider(providerKey) {
		t.Error("provider should be allowed after 1 timeout")
	}

	cb.RecordFailure(models.FailureClassTimeout, providerKey)
	if !cb.AllowProvider(providerKey) {
		t.Error("provider should be allowed after 2 timeouts")
	}

	// Third timeout should trip the circuit (default threshold = 3).
	cb.RecordFailure(models.FailureClassTimeout, providerKey)
	if cb.AllowProvider(providerKey) {
		t.Error("provider should be blocked after 3 consecutive timeouts")
	}
}

func TestCircuitBreaker_SuccessResetsTimeoutCounter(t *testing.T) {
	cb := NewCircuitBreaker(zap.NewNop())

	providerKey := "ollama-vm300"

	// Record 2 timeouts, then a success.
	cb.RecordFailure(models.FailureClassTimeout, providerKey)
	cb.RecordFailure(models.FailureClassTimeout, providerKey)
	cb.RecordSuccess(providerKey)

	// Two more timeouts should not trip (counter was reset).
	cb.RecordFailure(models.FailureClassTimeout, providerKey)
	cb.RecordFailure(models.FailureClassTimeout, providerKey)
	if !cb.AllowProvider(providerKey) {
		t.Error("provider should be allowed: success reset the counter")
	}
}

func TestCircuitBreaker_TimeoutAutoRecovery(t *testing.T) {
	cb := NewCircuitBreaker(zap.NewNop())
	// Use a very short cooldown for testing.
	cb.providerCooldown = 50 * time.Millisecond

	providerKey := "ollama-vm300"

	// Trip the circuit.
	for range 3 {
		cb.RecordFailure(models.FailureClassTimeout, providerKey)
	}
	if cb.AllowProvider(providerKey) {
		t.Fatal("provider should be blocked after trip")
	}

	// Wait for cooldown.
	time.Sleep(60 * time.Millisecond)

	if !cb.AllowProvider(providerKey) {
		t.Error("provider should auto-recover after cooldown")
	}
}

func TestCircuitBreaker_StatusShowsTrippedTimeoutProvider(t *testing.T) {
	cb := NewCircuitBreaker(zap.NewNop())

	providerKey := "ollama-vm300"

	// Trip via timeouts.
	for range 3 {
		cb.RecordFailure(models.FailureClassTimeout, providerKey)
	}

	status := cb.Status()
	if _, exists := status.TrippedProviders[providerKey]; !exists {
		t.Errorf("expected %q in tripped providers, got %v", providerKey, status.TrippedProviders)
	}
}

func TestCircuitBreaker_NonRetryableDoesNotTrip(t *testing.T) {
	cb := NewCircuitBreaker(zap.NewNop())

	providerKey := "test-provider"

	// OOMKill, Shutdown, Panic should NOT trip the circuit.
	nonTripping := []models.FailureClass{
		models.FailureClassOOMKill,
		models.FailureClassShutdown,
		models.FailureClassPanic,
		models.FailureClassPostProcess,
		models.FailureClassUnknown,
	}
	for _, fc := range nonTripping {
		for range 5 {
			cb.RecordFailure(fc, providerKey)
		}
		if !cb.AllowProvider(providerKey) {
			t.Errorf("provider should still be allowed after %s failures", fc)
		}
	}
}
