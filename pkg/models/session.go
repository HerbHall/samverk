package models

import "time"

// SessionStatus represents the lifecycle state of a work session.
type SessionStatus string

const (
	SessionStatusPending   SessionStatus = "pending"
	SessionStatusActive    SessionStatus = "active"
	SessionStatusCompleted SessionStatus = "completed"
	SessionStatusFailed    SessionStatus = "failed"
	SessionStatusCancelled SessionStatus = "cancelled"
)

// Session represents a unit of agent work on an issue.
type Session struct {
	ID               string        `json:"id"`
	IssueNumber      int           `json:"issue_number"`
	AgentType        string        `json:"agent_type"` // e.g., "code-gen", "qc", "research"
	Provider         string        `json:"provider"`   // e.g., "ollama", "claude", "openai"
	Model            string        `json:"model"`      // e.g., "qwen2.5-coder:7b"
	Status           SessionStatus `json:"status"`
	StartedAt        time.Time     `json:"started_at"`
	FinishedAt       *time.Time    `json:"finished_at,omitempty"`
	Error            string        `json:"error,omitempty"`
	EstimatedTimeout time.Duration `json:"estimated_timeout_ms,omitempty"` // timeout predicted at dispatch time
	PartialOutput    string        `json:"partial_output,omitempty"`       // last checkpoint of streaming output
	CheckpointHash   string        `json:"checkpoint_hash,omitempty"`      // SHA-256 of posted checkpoint for dedup
	MaxTurnsHit      bool          `json:"max_turns_hit,omitempty"`        // true when session hit max turns limit
	TurnsUsed        int           `json:"turns_used,omitempty"`           // number of assistant turns completed
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}
