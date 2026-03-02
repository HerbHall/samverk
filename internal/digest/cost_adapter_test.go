package digest

import (
	"context"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
)

// newTestStoreWithSession creates an in-memory SQLite store and inserts a
// minimal session so that cost records can satisfy the foreign key constraint.
func newTestStoreWithSession(t *testing.T) (s *store.SQLiteStore, sessionID string) {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	sess := &models.Session{
		IssueNumber: 1,
		AgentType:   "code-gen",
		Provider:    "claude",
		Model:       "opus-4",
		Status:      models.SessionStatusActive,
		StartedAt:   time.Now().UTC(),
	}
	if err := s.CreateSession(context.Background(), sess); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return s, sess.ID
}

func TestStoreCostSource_ComputeCostSince(t *testing.T) {
	s, sessionID := newTestStoreWithSession(t)
	ctx := context.Background()

	baseTime := time.Now().UTC().Truncate(time.Second)

	// Insert two cost records.
	records := []*models.CostRecord{
		{
			SessionID:    sessionID,
			Provider:     "claude",
			Model:        "opus-4",
			InputTokens:  2000,
			OutputTokens: 800,
			CostUSD:      0.05,
			CreatedAt:    baseTime.Add(-30 * time.Minute),
		},
		{
			SessionID:    sessionID,
			Provider:     "ollama",
			Model:        "qwen2.5-coder:7b",
			InputTokens:  1000,
			OutputTokens: 200,
			CostUSD:      0,
			CreatedAt:    baseTime.Add(-10 * time.Minute),
		},
	}
	for _, r := range records {
		if err := s.RecordCost(ctx, r); err != nil {
			t.Fatalf("record cost: %v", err)
		}
	}

	src := NewStoreCostSource(s, 0)
	since := baseTime.Add(-1 * time.Hour)
	result := src.ComputeCostSince(ctx, since)

	wantTokens := (2000 + 800) + (1000 + 200) // 4000 total
	if result.TokensUsed != wantTokens {
		t.Errorf("TokensUsed = %d, want %d", result.TokensUsed, wantTokens)
	}
	if result.EstimatedCostUSD != 0.05 {
		t.Errorf("EstimatedCostUSD = %f, want 0.05", result.EstimatedCostUSD)
	}
	if result.BudgetRemainingUSD != 0 {
		t.Errorf("BudgetRemainingUSD = %f, want 0 (unlimited budget)", result.BudgetRemainingUSD)
	}
}

func TestStoreCostSource_WithBudget(t *testing.T) {
	s, sessionID := newTestStoreWithSession(t)
	ctx := context.Background()

	r := &models.CostRecord{
		SessionID:    sessionID,
		Provider:     "claude",
		Model:        "sonnet-4",
		InputTokens:  5000,
		OutputTokens: 2000,
		CostUSD:      0.30,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.RecordCost(ctx, r); err != nil {
		t.Fatalf("record cost: %v", err)
	}

	src := NewStoreCostSource(s, 1.00) // $1.00 daily budget
	result := src.ComputeCostSince(ctx, time.Now().UTC().Add(-1*time.Hour))

	if result.TokensUsed != 7000 {
		t.Errorf("TokensUsed = %d, want 7000", result.TokensUsed)
	}
	if result.EstimatedCostUSD != 0.30 {
		t.Errorf("EstimatedCostUSD = %f, want 0.30", result.EstimatedCostUSD)
	}
	wantRemaining := 0.70
	if result.BudgetRemainingUSD != wantRemaining {
		t.Errorf("BudgetRemainingUSD = %f, want %f", result.BudgetRemainingUSD, wantRemaining)
	}
}

func TestStoreCostSource_BudgetExceeded(t *testing.T) {
	s, sessionID := newTestStoreWithSession(t)
	ctx := context.Background()

	r := &models.CostRecord{
		SessionID:    sessionID,
		Provider:     "claude",
		Model:        "opus-4",
		InputTokens:  50000,
		OutputTokens: 20000,
		CostUSD:      2.50,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.RecordCost(ctx, r); err != nil {
		t.Fatalf("record cost: %v", err)
	}

	src := NewStoreCostSource(s, 1.00) // $1.00 budget, $2.50 spent
	result := src.ComputeCostSince(ctx, time.Now().UTC().Add(-1*time.Hour))

	if result.BudgetRemainingUSD != 0 {
		t.Errorf("BudgetRemainingUSD = %f, want 0 (floored at zero)", result.BudgetRemainingUSD)
	}
}

func TestStoreCostSource_EmptyStore(t *testing.T) {
	s, _ := newTestStoreWithSession(t)
	ctx := context.Background()

	src := NewStoreCostSource(s, 5.00)
	result := src.ComputeCostSince(ctx, time.Now().UTC().Add(-24*time.Hour))

	if result.TokensUsed != 0 {
		t.Errorf("TokensUsed = %d, want 0", result.TokensUsed)
	}
	if result.EstimatedCostUSD != 0 {
		t.Errorf("EstimatedCostUSD = %f, want 0", result.EstimatedCostUSD)
	}
	// With a budget but no spend, remaining should be 0 (unlimited behavior is dailyBudget=0)
	// Wait -- with dailyBudget=5.00 and no spend, remaining = 5.00 - 0 = 5.00
	if result.BudgetRemainingUSD != 5.00 {
		t.Errorf("BudgetRemainingUSD = %f, want 5.00", result.BudgetRemainingUSD)
	}
}

func TestStoreCostSource_EmptyStoreUnlimited(t *testing.T) {
	s, _ := newTestStoreWithSession(t)
	ctx := context.Background()

	src := NewStoreCostSource(s, 0) // unlimited
	result := src.ComputeCostSince(ctx, time.Now().UTC().Add(-24*time.Hour))

	if result.TokensUsed != 0 {
		t.Errorf("TokensUsed = %d, want 0", result.TokensUsed)
	}
	if result.EstimatedCostUSD != 0 {
		t.Errorf("EstimatedCostUSD = %f, want 0", result.EstimatedCostUSD)
	}
	if result.BudgetRemainingUSD != 0 {
		t.Errorf("BudgetRemainingUSD = %f, want 0 (unlimited)", result.BudgetRemainingUSD)
	}
}
