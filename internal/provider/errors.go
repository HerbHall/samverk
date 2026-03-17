package provider

import (
	"errors"
	"fmt"
	"time"
)

// TimeoutType distinguishes between different timeout failure modes.
type TimeoutType string

const (
	// TimeoutStartup means the process produced no output at all within the
	// startup window. This typically indicates the process failed to launch
	// or is stuck initializing.
	TimeoutStartup TimeoutType = "startup"

	// TimeoutStale means the process was producing output but stopped for
	// longer than the stale output threshold. This typically indicates the
	// process hung mid-execution.
	TimeoutStale TimeoutType = "stale"
)

// ErrProviderTimeout is returned when a provider times out during execution.
// It is retryable, allowing the pool to fall back to the next provider in
// the routing chain.
type ErrProviderTimeout struct {
	Provider    string
	Model       string
	TimeoutType TimeoutType
	Duration    time.Duration
}

func (e *ErrProviderTimeout) Error() string {
	return fmt.Sprintf("provider %s timeout (%s): no output for %v", e.Provider, e.TimeoutType, e.Duration)
}

// Retryable returns true, indicating the pool should try the next provider.
func (e *ErrProviderTimeout) Retryable() bool {
	return true
}

// IsRetryable checks whether an error is retryable. Returns true if the error
// implements the Retryable() bool interface and returns true.
func IsRetryable(err error) bool {
	type retryable interface {
		Retryable() bool
	}
	var r retryable
	if ok := errors.As(err, &r); ok {
		return r.Retryable()
	}
	return false
}
