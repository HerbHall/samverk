package store

import (
	"context"
	"fmt"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

// RecordCost inserts a cost record. If r.ID is empty, a random ID is generated.
func (s *SQLiteStore) RecordCost(ctx context.Context, r *models.CostRecord) error {
	if r.ID == "" {
		r.ID = generateID()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cost_records (id, session_id, provider, model, input_tokens, output_tokens, cost_usd, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID,
		r.SessionID,
		r.Provider,
		r.Model,
		r.InputTokens,
		r.OutputTokens,
		r.CostUSD,
		r.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert cost record: %w", err)
	}
	return nil
}

// ComputeCostSince aggregates all cost records created at or after the given time.
func (s *SQLiteStore) ComputeCostSince(ctx context.Context, since time.Time) (summary *models.CostSummary, err error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(input_tokens), 0),
		        COALESCE(SUM(output_tokens), 0),
		        COALESCE(SUM(cost_usd), 0),
		        COUNT(*)
		 FROM cost_records
		 WHERE created_at >= ?`,
		since.Format(time.RFC3339),
	)

	summary = &models.CostSummary{}
	err = row.Scan(&summary.TotalInputTokens, &summary.TotalOutputTokens, &summary.TotalCostUSD, &summary.RecordCount)
	if err != nil {
		return nil, fmt.Errorf("compute cost since: %w", err)
	}
	return summary, nil
}

// ComputeCostForIssue aggregates all cost records across every session for the
// given issue number. This enables per-issue budget tracking and outlier detection.
func (s *SQLiteStore) ComputeCostForIssue(ctx context.Context, issueNumber int) (summary *models.CostSummary, err error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cr.input_tokens), 0),
		        COALESCE(SUM(cr.output_tokens), 0),
		        COALESCE(SUM(cr.cost_usd), 0),
		        COUNT(*)
		 FROM cost_records cr
		 JOIN sessions s ON cr.session_id = s.id
		 WHERE s.issue_number = ?`,
		issueNumber,
	)

	summary = &models.CostSummary{}
	err = row.Scan(&summary.TotalInputTokens, &summary.TotalOutputTokens, &summary.TotalCostUSD, &summary.RecordCount)
	if err != nil {
		return nil, fmt.Errorf("compute cost for issue #%d: %w", issueNumber, err)
	}
	return summary, nil
}

// GetBudgetStatus computes today's spending and remaining budget.
// spent is the total cost_usd since midnight UTC today.
// remaining is dailyBudgetUSD - spent (floored at 0).
func (s *SQLiteStore) GetBudgetStatus(ctx context.Context, dailyBudgetUSD float64) (spent, remaining float64, err error) {
	midnight := time.Now().UTC().Truncate(24 * time.Hour)

	summary, err := s.ComputeCostSince(ctx, midnight)
	if err != nil {
		return 0, 0, err
	}

	spent = summary.TotalCostUSD
	remaining = dailyBudgetUSD - spent
	if remaining < 0 {
		remaining = 0
	}
	return spent, remaining, nil
}
