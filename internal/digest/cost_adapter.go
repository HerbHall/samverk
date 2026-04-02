package digest

import (
	"context"
	"go.uber.org/zap"
	"time"

	"samverk.dev/samverk/internal/store"
)

// StoreCostSource adapts store.Store to the CostSource interface.
type StoreCostSource struct {
	store       store.Store
	dailyBudget float64 // daily budget in USD, 0 means unlimited
}

// NewStoreCostSource creates a CostSource backed by the SQLite store.
func NewStoreCostSource(s store.Store, dailyBudget float64) *StoreCostSource {
	return &StoreCostSource{
		store:       s,
		dailyBudget: dailyBudget,
	}
}

// ComputeCostSince implements CostSource by querying the store and mapping
// models.CostSummary fields to digest.CostSummary fields.
func (s *StoreCostSource) ComputeCostSince(ctx context.Context, since time.Time) CostSummary {
	summary, err := s.store.ComputeCostSince(ctx, since)
	if err != nil {
		zap.L().Error("cost adapter: failed to compute cost", zap.Error(err))
		return CostSummary{}
	}

	result := CostSummary{
		TokensUsed:       summary.TotalInputTokens + summary.TotalOutputTokens,
		EstimatedCostUSD: summary.TotalCostUSD,
	}

	if s.dailyBudget > 0 {
		result.BudgetRemainingUSD = s.dailyBudget - summary.TotalCostUSD
		if result.BudgetRemainingUSD < 0 {
			result.BudgetRemainingUSD = 0
		}
	}

	return result
}
