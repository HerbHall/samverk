package cost

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
)

// ErrBudgetExceeded is returned when the rolling window spend meets or exceeds the budget.
var ErrBudgetExceeded = errors.New("budget exceeded")

// Tracker wraps cost recording with pre-execution budget checking.
type Tracker struct {
	store       store.Store
	budgetUSD   float64 // rolling window budget, 0 = unlimited
	windowHours int     // rolling window in hours for budget check
}

// NewTracker creates a cost tracker.
// budgetUSD of 0 means unlimited. windowHours defaults to 24 if <= 0.
func NewTracker(st store.Store, budgetUSD float64, windowHours int) *Tracker {
	if windowHours <= 0 {
		windowHours = 24
	}
	return &Tracker{
		store:       st,
		budgetUSD:   budgetUSD,
		windowHours: windowHours,
	}
}

// CheckBudget returns ErrBudgetExceeded if the rolling window spend meets or
// exceeds the budget. Returns nil if budget is unlimited (0) or spend is under budget.
func (t *Tracker) CheckBudget(ctx context.Context) error {
	if t.budgetUSD <= 0 {
		return nil
	}
	since := time.Now().Add(-time.Duration(t.windowHours) * time.Hour)
	summary, err := t.store.ComputeCostSince(ctx, since)
	if err != nil {
		return fmt.Errorf("check budget: %w", err)
	}
	if summary.TotalCostUSD >= t.budgetUSD {
		return ErrBudgetExceeded
	}
	return nil
}

// RecordUsage records token usage and estimated cost for a chat response.
func (t *Tracker) RecordUsage(ctx context.Context, sessionID, providerName, model string, resp *provider.ChatResponse) error {
	costUSD := estimateCost(providerName, model, resp.PromptTokens, resp.CompletionTokens)
	record := &models.CostRecord{
		SessionID:    sessionID,
		Provider:     providerName,
		Model:        model,
		InputTokens:  resp.PromptTokens,
		OutputTokens: resp.CompletionTokens,
		CostUSD:      costUSD,
		CreatedAt:    time.Now(),
	}
	return t.store.RecordCost(ctx, record)
}

// GetIssueCostSummary returns the aggregated cost for all sessions of the given issue.
func (t *Tracker) GetIssueCostSummary(ctx context.Context, issueNumber int) (*models.CostSummary, error) {
	return t.store.ComputeCostForIssue(ctx, issueNumber)
}

// CheckOutlier compares actual token usage for an issue against its estimate.
// Returns a warning message if actual exceeds 2x the estimate, empty string otherwise.
func (t *Tracker) CheckOutlier(ctx context.Context, issueNumber, estimatedTokens int) string {
	if estimatedTokens <= 0 {
		return ""
	}
	summary, err := t.store.ComputeCostForIssue(ctx, issueNumber)
	if err != nil {
		return ""
	}
	actual := summary.TotalInputTokens + summary.TotalOutputTokens
	if actual > 2*estimatedTokens {
		return fmt.Sprintf("Token outlier: issue #%d used %d tokens (estimated %d, %.1fx over)",
			issueNumber, actual, estimatedTokens, float64(actual)/float64(estimatedTokens))
	}
	return ""
}

// pricing holds per-million-token costs for input and output.
type pricing struct {
	inputPer1M  float64
	outputPer1M float64
}

// knownPricing maps model identifiers to their per-million-token pricing (approximate, early 2026).
var knownPricing = map[string]pricing{
	// Claude models
	"claude-sonnet-4-20250514":  {3.0, 15.0},
	"claude-haiku-4-5-20251001": {0.80, 4.0},
	"claude-opus-4-6":           {15.0, 75.0},
	// OpenAI models
	"gpt-4o":      {2.50, 10.0},
	"gpt-4o-mini": {0.15, 0.60},
	"gpt-4-turbo": {10.0, 30.0},
}

// estimateCost calculates the estimated cost in USD based on provider and model.
// Uses hardcoded pricing for known models, falls back to conservative defaults.
func estimateCost(providerName, model string, inputTokens, outputTokens int) float64 {
	// Ollama (local) is free.
	if providerName == "ollama" {
		return 0
	}

	p, ok := knownPricing[model]
	if !ok {
		// Conservative default for unknown cloud models.
		p = pricing{inputPer1M: 10.0, outputPer1M: 30.0}
	}

	return (float64(inputTokens)/1_000_000)*p.inputPer1M +
		(float64(outputTokens)/1_000_000)*p.outputPer1M
}
