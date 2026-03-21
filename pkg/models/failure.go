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

// IsProviderFailure returns true if the failure is an infrastructure problem
// (provider unreachable, unhealthy, or hung) rather than an issue-specific problem.
// Provider failures should NOT count toward per-issue escalation thresholds.
func (fc FailureClass) IsProviderFailure() bool {
	switch fc {
	case FailureClassProviderDown, FailureClassOOMKill:
		return true
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

	// RCA fields — populated when a fix is recorded.
	RootCauseCategory string     `json:"root_cause_category,omitempty"`
	FixClassification string     `json:"fix_classification,omitempty"`
	PreventionMeasure string     `json:"prevention_measure,omitempty"`
	RecurrenceRisk    string     `json:"recurrence_risk,omitempty"`
	DetectionMethod   string     `json:"detection_method,omitempty"`
	TimeToDetectMin   *int       `json:"time_to_detect_min,omitempty"`
	LinkedIssues      string     `json:"linked_issues,omitempty"`
	Component         string     `json:"component,omitempty"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
	// Status mirrors the session status for KPI queries (e.g. "resolved").
	Status string `json:"status,omitempty"`
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

// RootCauseBreakdownEntry holds one row of the root-cause breakdown.
type RootCauseBreakdownEntry struct {
	Category string  `json:"category"`
	Count    int     `json:"count"`
	Pct      float64 `json:"pct"`
}

// KPIReport is the response payload for GET /api/v1/kpis.
// Rates are nil when sample_size < 5 (insufficient data).
type KPIReport struct {
	WindowDays           int                       `json:"window_days"`
	FirstTimeFixRate     *float64                  `json:"first_time_fix_rate"`
	FixSuccessRate       *float64                  `json:"fix_success_rate"`
	RecurrenceRate       *float64                  `json:"recurrence_rate"`
	WorkaroundRate       *float64                  `json:"workaround_rate"`
	MeanTimeToFixHours   *float64                  `json:"mean_time_to_fix_hours"`
	PlanningGapRate      *float64                  `json:"planning_gap_rate"`
	UnknownRootCauseRate *float64                  `json:"unknown_root_cause_rate"`
	PreventionCoverage   *float64                  `json:"prevention_coverage"`
	RootCauseBreakdown   []RootCauseBreakdownEntry `json:"root_cause_breakdown"`
	SampleSize           int                       `json:"sample_size"`
}

// QualityRecommendation is one advisory recommendation from the advisory engine.
type QualityRecommendation struct {
	ID            string    `json:"id"`
	Level         string    `json:"level"`        // "info" | "warn" | "critical"
	Category      string    `json:"category"`
	Title         string    `json:"title"`
	Detail        string    `json:"detail"`
	AffectedCount int       `json:"affected_count"`
	WindowDays    int       `json:"window_days"`
	GeneratedAt   time.Time `json:"generated_at"`
}

// CorrectionEvent records a correction decision made by the failure response engine.
type CorrectionEvent struct {
	ID           string       `json:"id"`
	IssueNumber  int          `json:"issue_number"`
	FailureClass FailureClass `json:"failure_class"`
	Action       string       `json:"action"`
	Scope        string       `json:"scope"`
	Reason       string       `json:"reason"`
	NewProvider  string       `json:"new_provider,omitempty"`
	Outcome      string       `json:"outcome"` // pending, applied, failed
	CreatedAt    time.Time    `json:"created_at"`
}
