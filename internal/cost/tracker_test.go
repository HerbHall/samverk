package cost

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
)

// mockStore implements store.Store for testing. Only RecordCost and
// ComputeCostSince are wired; all other methods panic via the embedded
// nil interface if called (which is fine -- tests only call mocked methods).
type mockStore struct {
	store.Store
	recordCostFn  func(ctx context.Context, r *models.CostRecord) error
	computeCostFn func(ctx context.Context, since time.Time) (*models.CostSummary, error)
}

func (m *mockStore) RecordCost(ctx context.Context, r *models.CostRecord) error {
	if m.recordCostFn != nil {
		return m.recordCostFn(ctx, r)
	}
	return nil
}

func (m *mockStore) ComputeCostSince(ctx context.Context, since time.Time) (*models.CostSummary, error) {
	if m.computeCostFn != nil {
		return m.computeCostFn(ctx, since)
	}
	return &models.CostSummary{}, nil
}

func TestCheckBudgetUnlimited(t *testing.T) {
	tr := NewTracker(&mockStore{}, 0, 24)

	if err := tr.CheckBudget(context.Background()); err != nil {
		t.Fatalf("unlimited budget should return nil, got %v", err)
	}
}

func TestCheckBudgetUnderBudget(t *testing.T) {
	ms := &mockStore{
		computeCostFn: func(_ context.Context, _ time.Time) (*models.CostSummary, error) {
			return &models.CostSummary{TotalCostUSD: 0.50}, nil
		},
	}
	tr := NewTracker(ms, 1.00, 24)

	if err := tr.CheckBudget(context.Background()); err != nil {
		t.Fatalf("under budget should return nil, got %v", err)
	}
}

func TestCheckBudgetExceeded(t *testing.T) {
	ms := &mockStore{
		computeCostFn: func(_ context.Context, _ time.Time) (*models.CostSummary, error) {
			return &models.CostSummary{TotalCostUSD: 1.00}, nil
		},
	}
	tr := NewTracker(ms, 1.00, 24)

	err := tr.CheckBudget(context.Background())
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("at-budget should return ErrBudgetExceeded, got %v", err)
	}
}

func TestCheckBudgetStoreError(t *testing.T) {
	storeErr := errors.New("database unavailable")
	ms := &mockStore{
		computeCostFn: func(_ context.Context, _ time.Time) (*models.CostSummary, error) {
			return nil, storeErr
		},
	}
	tr := NewTracker(ms, 5.00, 24)

	err := tr.CheckBudget(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, storeErr) {
		t.Fatalf("expected wrapped store error, got %v", err)
	}
}

func TestRecordUsage(t *testing.T) {
	var recorded *models.CostRecord
	ms := &mockStore{
		recordCostFn: func(_ context.Context, r *models.CostRecord) error {
			recorded = r
			return nil
		},
	}
	tr := NewTracker(ms, 0, 24)

	resp := &provider.ChatResponse{
		PromptTokens:     1000,
		CompletionTokens: 500,
	}

	err := tr.RecordUsage(context.Background(), "sess-1", "claude", "claude-opus-4-6", resp)
	if err != nil {
		t.Fatalf("RecordUsage returned error: %v", err)
	}
	if recorded == nil {
		t.Fatal("expected RecordCost to be called")
	}
	if recorded.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want %q", recorded.SessionID, "sess-1")
	}
	if recorded.Provider != "claude" {
		t.Errorf("Provider = %q, want %q", recorded.Provider, "claude")
	}
	if recorded.Model != "claude-opus-4-6" {
		t.Errorf("Model = %q, want %q", recorded.Model, "claude-opus-4-6")
	}
	if recorded.InputTokens != 1000 {
		t.Errorf("InputTokens = %d, want 1000", recorded.InputTokens)
	}
	if recorded.OutputTokens != 500 {
		t.Errorf("OutputTokens = %d, want 500", recorded.OutputTokens)
	}
	if recorded.CostUSD <= 0 {
		t.Errorf("CostUSD = %f, want > 0 for cloud model", recorded.CostUSD)
	}
}

func TestEstimateCostKnownModel(t *testing.T) {
	// claude-opus-4-6: $15/M input, $75/M output
	// 1000 input tokens = 1000/1M * 15 = 0.015
	// 500 output tokens = 500/1M * 75 = 0.0375
	// Total = 0.0525
	cost := estimateCost("claude", "claude-opus-4-6", 1000, 500)
	expected := 0.0525
	if math.Abs(cost-expected) > 1e-9 {
		t.Errorf("cost = %f, want %f", cost, expected)
	}
}

func TestEstimateCostOllama(t *testing.T) {
	cost := estimateCost("ollama", "qwen2.5-coder:7b", 10000, 5000)
	if cost != 0 {
		t.Errorf("ollama cost = %f, want 0", cost)
	}
}

func TestEstimateCostUnknownModel(t *testing.T) {
	// Unknown cloud model: $10/M input, $30/M output (conservative default)
	// 1000 input = 1000/1M * 10 = 0.01
	// 500 output = 500/1M * 30 = 0.015
	// Total = 0.025
	cost := estimateCost("some-provider", "unknown-model-v99", 1000, 500)
	expected := 0.025
	if math.Abs(cost-expected) > 1e-9 {
		t.Errorf("cost = %f, want %f", cost, expected)
	}
}

func TestNewTrackerDefaults(t *testing.T) {
	tr := NewTracker(&mockStore{}, 5.00, 0)
	if tr.windowHours != 24 {
		t.Errorf("windowHours = %d, want 24 (default)", tr.windowHours)
	}

	tr2 := NewTracker(&mockStore{}, 5.00, -1)
	if tr2.windowHours != 24 {
		t.Errorf("windowHours = %d, want 24 (default for negative)", tr2.windowHours)
	}

	tr3 := NewTracker(&mockStore{}, 5.00, 12)
	if tr3.windowHours != 12 {
		t.Errorf("windowHours = %d, want 12 (explicit)", tr3.windowHours)
	}
}
