package dispatcher

import (
	"strings"

	"github.com/herbhall/samverk/pkg/models"
)

// classifyFailure categorises an error message into an actionable failure class.
// The classifier checks patterns from most specific to least specific.
func classifyFailure(errMsg string) models.FailureClass {
	lower := strings.ToLower(errMsg)

	// Shutdown: exit 143 (SIGTERM) is expected during service restarts.
	if strings.Contains(lower, "exit status 143") || strings.Contains(lower, "signal: terminated") {
		return models.FailureClassShutdown
	}

	// Panic: runtime panics and closed channel sends.
	if strings.Contains(lower, "send on closed channel") || strings.Contains(lower, "runtime error") || strings.Contains(lower, "panic:") {
		return models.FailureClassPanic
	}

	// Auth: OAuth/API key expiry or invalid credentials.
	if strings.Contains(lower, "401") || strings.Contains(lower, "authentication_error") ||
		strings.Contains(lower, "oauth token expired") || strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "invalid api key") || strings.Contains(lower, "invalid x-api-key") ||
		strings.Contains(lower, "not logged in") {
		return models.FailureClassAuth
	}

	// Budget: credit or budget exhaustion (permanent until refilled).
	if strings.Contains(lower, "credit balance is too low") || strings.Contains(lower, "budget exceeded") ||
		strings.Contains(lower, "insufficient_quota") {
		return models.FailureClassBudget
	}

	// Rate limit: transient throttling (retryable after backoff).
	// Use rate_limit (underscore) to avoid matching "rate limited" in
	// post-process context (e.g., "add comment: rate limited by GitHub").
	if strings.Contains(lower, "rate_limit") || strings.Contains(lower, "429") ||
		strings.Contains(lower, "too many requests") || strings.Contains(lower, "overloaded") {
		return models.FailureClassProviderDown
	}

	// Permanent: model not found or config errors that retrying cannot fix.
	if strings.Contains(lower, "model") && strings.Contains(lower, "not found") {
		return models.FailureClassPermanent
	}

	// OOM/Kill: process killed externally.
	if strings.Contains(lower, "signal: killed") {
		return models.FailureClassOOMKill
	}

	// Provider down: connection errors, network failures, and provider hangs.
	if strings.Contains(lower, "context deadline exceeded") || strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "no such host") || strings.Contains(lower, "i/o timeout") ||
		strings.Contains(lower, "no healthy provider") || strings.Contains(lower, "hung: no output") ||
		strings.Contains(lower, "connection reset") || strings.Contains(lower, "broken pipe") ||
		strings.Contains(lower, "eof") || strings.Contains(lower, "host unreachable") ||
		strings.Contains(lower, "network is unreachable") || strings.Contains(lower, "tls handshake") {
		return models.FailureClassProviderDown
	}

	// Timeout: heartbeat-based timeouts and context cancellation.
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "missed heartbeat") ||
		strings.Contains(lower, "context canceled") {
		return models.FailureClassTimeout
	}

	// Post-process: PR creation, comment posting, or validation failures.
	if strings.Contains(lower, "post-process error") || strings.Contains(lower, "create pr:") ||
		strings.Contains(lower, "create branch") || strings.Contains(lower, "add comment:") ||
		strings.Contains(lower, "validation failed") || strings.Contains(lower, "validation retry failed") ||
		strings.Contains(lower, "format_error") || strings.Contains(lower, "edit block") ||
		strings.Contains(lower, "output rejected") {
		return models.FailureClassPostProcess
	}

	// Classify: frontmatter or agent type validation.
	if strings.Contains(lower, "classify issue") || strings.Contains(lower, "invalid_frontmatter") ||
		strings.Contains(lower, "no frontmatter found") || strings.Contains(lower, "unknown agent_type") {
		return models.FailureClassClassify
	}

	// Cycle: dependency cycle.
	if strings.Contains(lower, "dependency_cycle") || strings.Contains(lower, "cycle detected") {
		return models.FailureClassCycle
	}

	// Decompose: issue decomposition.
	if strings.Contains(lower, "decompose") {
		return models.FailureClassDecompose
	}

	return models.FailureClassUnknown
}
