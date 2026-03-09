package models

import "time"

// TaskProfile holds rolling performance statistics for a specific
// (AgentType, Provider) combination, computed from completed sessions.
// Values are updated after each successful task completion.
type TaskProfile struct {
	AgentType   string        `json:"agent_type"`
	Provider    string        `json:"provider"`
	AvgDuration time.Duration `json:"avg_duration"`
	P50Duration time.Duration `json:"p50_duration"`
	P90Duration time.Duration `json:"p90_duration"`
	SampleCount int           `json:"sample_count"`
	AvgTokens   int           `json:"avg_tokens"`
	UpdatedAt   time.Time     `json:"updated_at"`
}
