package provider_test

import (
	"errors"
	"testing"

	"github.com/herbhall/samverk/internal/provider"
)

func TestIsRetryable_ErrProviderUnavailable(t *testing.T) {
	cause := errors.New("connection refused")
	err := &provider.ErrProviderUnavailable{
		Provider: "ollama",
		Cause:    cause,
	}

	if !provider.IsRetryable(err) {
		t.Error("IsRetryable = false, want true for ErrProviderUnavailable")
	}
}

func TestErrProviderUnavailable_Error(t *testing.T) {
	cause := errors.New("connection refused")
	err := &provider.ErrProviderUnavailable{
		Provider: "ollama",
		Cause:    cause,
	}

	got := err.Error()
	want := "provider ollama unavailable: connection refused"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrProviderUnavailable_Unwrap(t *testing.T) {
	cause := errors.New("connection refused")
	err := &provider.ErrProviderUnavailable{
		Provider: "ollama",
		Cause:    cause,
	}

	if !errors.Is(err, cause) {
		t.Error("errors.Is(err, cause) = false, want true — Unwrap not working")
	}
}

func TestIsRetryable_ErrProviderTimeout(t *testing.T) {
	// ErrProviderTimeout should also remain retryable.
	err := &provider.ErrProviderTimeout{
		Provider:    "ollama",
		Model:       "llama3",
		TimeoutType: provider.TimeoutStale,
	}

	if !provider.IsRetryable(err) {
		t.Error("IsRetryable = false, want true for ErrProviderTimeout")
	}
}

func TestIsRetryable_OrdinaryError(t *testing.T) {
	err := errors.New("some random error")

	if provider.IsRetryable(err) {
		t.Error("IsRetryable = true, want false for ordinary error")
	}
}
