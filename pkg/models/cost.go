package models

import "time"

// CostRecord tracks token usage for a single API call.
type CostRecord struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CostUSD      float64   `json:"cost_usd"` // 0 for local models
	CreatedAt    time.Time `json:"created_at"`
}

// CostSummary aggregates cost data over a time period.
type CostSummary struct {
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	RecordCount       int     `json:"record_count"`
}
