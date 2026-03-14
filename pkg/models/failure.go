package models

import "time"

// FailureClass categorises a failure into an actionable class for circuit
// breakers and automated analysis.
type FailureClass string

const (
	FailureClassAuth         FailureClass = "auth"          // 401, OAuth expired, authentication_error
	FailureClassBudget       FailureClass = "budget"        // credit/budget exhausted
	FailureClassPermanent    FailureClass = "permanent"     // model not found, permanent config error
	FailureClassTimeout      FailureClass = "timeout"       // heartbeat timeout / context deadline
	FailureClassProviderDown FailureClass = "provider_down" // connection refused, provider unreachable
	FailureClassOOMKill      FailureClass = "oom_kill"      // signal: killed (OOM or external)
	FailureClassShutdown     FailureClass = "shutdown"      // exit 143 (SIGTERM during graceful shutdown)
	FailureClassPanic        FailureClass = "panic"         // send on closed channel, runtime panic
	FailureClassPostProcess  FailureClass = "post_process"  // PR creation, comment posting failed
	FailureClassClassify     FailureClass = "classify"      // frontmatter parse or agent type validation
	FailureClassCycle        FailureClass = "cycle"         // dependency cycle detected
	FailureClassDecompose    FailureClass = "decompose"     // issue decomposition failed
	FailureClassUnknown      FailureClass = "unknown"       // unrecognised error pattern
)

// IsRetryable returns true if this failure class warrants a retry attempt.
func (fc FailureClass) IsRetryable() bool {
	switch fc {
	case FailureClassTimeout, FailureClassOOMKill, FailureClassPostProcess:
		return true
	case FailureClassShutdown:
		// Shutdown kills are expected during restarts — don't count as real failures.
		return false
	default:
		return false
	}
}

// IsPermanent returns true if retrying will never succeed.
func (fc FailureClass) IsPermanent() bool {
	switch fc {
	case FailureClassAuth, FailureClassBudget, FailureClassPermanent, FailureClassClassify, FailureClassCycle:
		return true
	default:
		return false
	}
}

// FailureEvent represents a single failure occurrence in the dispatcher or
// agent runtime. Every failure path emits one of these to the store for
// aggregation and analysis.
type FailureEvent struct {
	ID            string        `json:"id"`
	IssueNumber   int           `json:"issue_number"`
	SessionID     string        `json:"session_id,omitempty"` // empty for dispatcher-level failures
	FailureClass  FailureClass  `json:"failure_class"`
	ErrorMessage  string        `json:"error_message"`
	AgentType     string        `json:"agent_type,omitempty"`
	Provider      string        `json:"provider,omitempty"`
	AttemptNumber int           `json:"attempt_number"` // which attempt (1st, 2nd, 3rd, etc.)
	Duration      time.Duration `json:"duration_ms,omitempty"`
	Timestamp     time.Time     `json:"timestamp"`
}

// FailureSummary aggregates failure data for dashboards and digests.
type FailureSummary struct {
	TotalFailures  int                    `json:"total_failures"`
	ByClass        map[FailureClass]int   `json:"by_class"`
	TopIssues      []IssueFailureCount    `json:"top_issues"`       // issues with most failures
	LoopingIssues  []IssueFailureCount    `json:"looping_issues"`   // issues with 5+ failures
	ProviderHealth map[string]int         `json:"provider_health"`  // provider -> failure count
	Since          time.Time              `json:"since"`
}

// IssueFailureCount pairs an issue number with its failure count.
type IssueFailureCount struct {
	IssueNumber int `json:"issue_number"`
	Count       int `json:"count"`
}
