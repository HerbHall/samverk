package dispatcher

import (
	"strings"

	"github.com/herbhall/samverk/pkg/models"
)

// ClassifyFailureDetailed returns a 3-level failure classification.
// Level 1 (Category) is the FailureClass for backwards compatibility.
// Level 2 (Subcategory) provides actionable grouping.
// Level 3 (Specific) is the original error message (not returned here; stored in ErrorMessage).
func ClassifyFailureDetailed(errMsg string) models.FailureClassification {
	fc := classifyFailure(errMsg)
	sub := classifySubcategory(fc, errMsg)
	return models.FailureClassification{Category: fc, Subcategory: sub}
}

// classifySubcategory maps a failure class + error message to a Level 2 subcategory.
func classifySubcategory(fc models.FailureClass, errMsg string) string {
	lower := strings.ToLower(errMsg)

	switch fc {
	case models.FailureClassProviderDown:
		if strings.Contains(lower, "no healthy provider") {
			return "all_providers_down"
		}
		if strings.Contains(lower, "connection refused") || strings.Contains(lower, "host unreachable") {
			return "connection_refused"
		}
		if strings.Contains(lower, "hung: no output") {
			return "provider_hung"
		}
		return "network_error"

	case models.FailureClassRateLimit:
		return "rate_limit"

	case models.FailureClassTimeout:
		if strings.Contains(lower, "missed heartbeat") {
			return "timeout_agent"
		}
		return "timeout_provider"

	case models.FailureClassPostProcess:
		if strings.Contains(lower, "validation failed") || strings.Contains(lower, "validation retry") {
			if strings.Contains(lower, "lint") || strings.Contains(lower, "golangci") {
				return "lint_fail"
			}
			if strings.Contains(lower, "test") {
				return "test_fail"
			}
			return "validation_fail"
		}
		if strings.Contains(lower, "edit block") || strings.Contains(lower, "format_error") {
			return "format_error"
		}
		if strings.Contains(lower, "create pr") || strings.Contains(lower, "create branch") {
			return "pr_creation_fail"
		}
		return "post_process_other"

	case models.FailureClassAuth:
		if strings.Contains(lower, "oauth") {
			return "oauth_expired"
		}
		return "auth_invalid"

	case models.FailureClassOOMKill:
		return "oom_kill"

	case models.FailureClassShutdown:
		return "graceful_shutdown"

	case models.FailureClassPanic:
		return "runtime_panic"

	case models.FailureClassBudget:
		return "budget_exhausted"

	case models.FailureClassPermanent:
		return "model_not_found"

	case models.FailureClassClassify:
		if strings.Contains(lower, "no frontmatter") {
			return "missing_frontmatter"
		}
		return "invalid_frontmatter"

	case models.FailureClassCycle:
		return "dependency_cycle"

	case models.FailureClassDecompose:
		return "decomposition_failed"

	case models.FailureClassUnknown:
		// Attempt to extract signal from unknown errors.
		if strings.Contains(lower, "import") || strings.Contains(lower, "module") {
			return "wrong_imports"
		}
		if strings.Contains(lower, "axios") || strings.Contains(lower, "hallucin") {
			return "hallucinated_api"
		}
		return "unclassified"
	}

	return "unclassified"
}

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
	if strings.Contains(lower, "rate_limit") || strings.Contains(lower, "429") ||
		strings.Contains(lower, "too many requests") || strings.Contains(lower, "overloaded") {
		return models.FailureClassRateLimit
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
