package store

import (
	"context"
	"testing"
	"time"

	"samverk.dev/samverk/pkg/models"
)

// createTestSession is a helper that inserts a minimal session and returns its ID.
func createTestSession(t *testing.T, s *SQLiteStore) string {
	t.Helper()
	ctx := context.Background()
	sess := &models.Session{
		IssueNumber: 1,
		AgentType:   "code-gen",
		Provider:    "ollama",
		Model:       "test-model",
		Status:      models.SessionStatusActive,
		StartedAt:   time.Now().UTC(),
	}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("create test session: %v", err)
	}
	return sess.ID
}

func TestRecordCost(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sessionID := createTestSession(t, s)

	r := &models.CostRecord{
		SessionID:    sessionID,
		Provider:     "claude",
		Model:        "opus-4",
		InputTokens:  1500,
		OutputTokens: 500,
		CostUSD:      0.045,
	}

	if err := s.RecordCost(ctx, r); err != nil {
		t.Fatalf("record cost: %v", err)
	}
	if r.ID == "" {
		t.Fatal("expected ID to be generated")
	}

	// Verify via ComputeCostSince with a time before the record.
	summary, err := s.ComputeCostSince(ctx, time.Now().UTC().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("compute cost: %v", err)
	}
	if summary.RecordCount != 1 {
		t.Errorf("record_count = %d, want 1", summary.RecordCount)
	}
	if summary.TotalInputTokens != 1500 {
		t.Errorf("input_tokens = %d, want 1500", summary.TotalInputTokens)
	}
	if summary.TotalOutputTokens != 500 {
		t.Errorf("output_tokens = %d, want 500", summary.TotalOutputTokens)
	}
}

func TestComputeCostSince(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sessionID := createTestSession(t, s)

	baseTime := time.Now().UTC().Truncate(time.Second)

	// Insert two records at different times.
	r1 := &models.CostRecord{
		SessionID:    sessionID,
		Provider:     "ollama",
		Model:        "qwen2.5-coder:7b",
		InputTokens:  1000,
		OutputTokens: 200,
		CostUSD:      0, // local model
		CreatedAt:    baseTime.Add(-30 * time.Minute),
	}
	r2 := &models.CostRecord{
		SessionID:    sessionID,
		Provider:     "claude",
		Model:        "sonnet-4",
		InputTokens:  2000,
		OutputTokens: 800,
		CostUSD:      0.03,
		CreatedAt:    baseTime.Add(-5 * time.Minute),
	}

	if err := s.RecordCost(ctx, r1); err != nil {
		t.Fatalf("record r1: %v", err)
	}
	if err := s.RecordCost(ctx, r2); err != nil {
		t.Fatalf("record r2: %v", err)
	}

	// Query since 15 minutes ago -- should only include r2.
	since := baseTime.Add(-15 * time.Minute)
	summary, err := s.ComputeCostSince(ctx, since)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}

	if summary.RecordCount != 1 {
		t.Errorf("record_count = %d, want 1", summary.RecordCount)
	}
	if summary.TotalInputTokens != 2000 {
		t.Errorf("input_tokens = %d, want 2000", summary.TotalInputTokens)
	}
	if summary.TotalOutputTokens != 800 {
		t.Errorf("output_tokens = %d, want 800", summary.TotalOutputTokens)
	}
	if summary.TotalCostUSD != 0.03 {
		t.Errorf("cost_usd = %f, want 0.03", summary.TotalCostUSD)
	}
}

func TestComputeCostSinceEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	summary, err := s.ComputeCostSince(ctx, time.Now().UTC().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("compute: %v", err)
	}

	if summary.RecordCount != 0 {
		t.Errorf("record_count = %d, want 0", summary.RecordCount)
	}
	if summary.TotalInputTokens != 0 {
		t.Errorf("input_tokens = %d, want 0", summary.TotalInputTokens)
	}
	if summary.TotalCostUSD != 0 {
		t.Errorf("cost_usd = %f, want 0", summary.TotalCostUSD)
	}
}

func TestComputeCostForIssue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create two sessions for different issues.
	sess1 := &models.Session{
		IssueNumber: 42,
		AgentType:   "code-gen",
		Provider:    "claude",
		Model:       "sonnet-4",
		Status:      models.SessionStatusCompleted,
		StartedAt:   time.Now().UTC(),
	}
	sess2 := &models.Session{
		IssueNumber: 42,
		AgentType:   "test",
		Provider:    "claude",
		Model:       "haiku-4",
		Status:      models.SessionStatusCompleted,
		StartedAt:   time.Now().UTC(),
	}
	sess3 := &models.Session{
		IssueNumber: 99, // different issue
		AgentType:   "code-gen",
		Provider:    "claude",
		Model:       "sonnet-4",
		Status:      models.SessionStatusCompleted,
		StartedAt:   time.Now().UTC(),
	}
	for _, sess := range []*models.Session{sess1, sess2, sess3} {
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatalf("create session: %v", err)
		}
	}

	// Record costs for each session.
	costs := []*models.CostRecord{
		{SessionID: sess1.ID, Provider: "claude", Model: "sonnet-4", InputTokens: 1000, OutputTokens: 500, CostUSD: 0.01},
		{SessionID: sess2.ID, Provider: "claude", Model: "haiku-4", InputTokens: 800, OutputTokens: 200, CostUSD: 0.005},
		{SessionID: sess3.ID, Provider: "claude", Model: "sonnet-4", InputTokens: 5000, OutputTokens: 2000, CostUSD: 0.05},
	}
	for _, c := range costs {
		if err := s.RecordCost(ctx, c); err != nil {
			t.Fatalf("record cost: %v", err)
		}
	}

	// Query issue 42: should aggregate sess1 + sess2 only.
	summary, err := s.ComputeCostForIssue(ctx, 42)
	if err != nil {
		t.Fatalf("compute cost for issue: %v", err)
	}
	if summary.TotalInputTokens != 1800 {
		t.Errorf("input_tokens = %d, want 1800", summary.TotalInputTokens)
	}
	if summary.TotalOutputTokens != 700 {
		t.Errorf("output_tokens = %d, want 700", summary.TotalOutputTokens)
	}
	if summary.RecordCount != 2 {
		t.Errorf("record_count = %d, want 2", summary.RecordCount)
	}
}

func TestComputeCostForIssueEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	summary, err := s.ComputeCostForIssue(ctx, 999)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if summary.RecordCount != 0 {
		t.Errorf("record_count = %d, want 0", summary.RecordCount)
	}
	if summary.TotalInputTokens != 0 {
		t.Errorf("input_tokens = %d, want 0", summary.TotalInputTokens)
	}
}

func TestGetBudgetStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sessionID := createTestSession(t, s)

	// Insert a cost record with today's timestamp.
	r := &models.CostRecord{
		SessionID:    sessionID,
		Provider:     "claude",
		Model:        "opus-4",
		InputTokens:  5000,
		OutputTokens: 2000,
		CostUSD:      0.10,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.RecordCost(ctx, r); err != nil {
		t.Fatalf("record: %v", err)
	}

	spent, remaining, err := s.GetBudgetStatus(ctx, 1.00)
	if err != nil {
		t.Fatalf("budget status: %v", err)
	}

	if spent != 0.10 {
		t.Errorf("spent = %f, want 0.10", spent)
	}
	if remaining != 0.90 {
		t.Errorf("remaining = %f, want 0.90", remaining)
	}
}

func TestGetBudgetStatusNoCosts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	spent, remaining, err := s.GetBudgetStatus(ctx, 5.00)
	if err != nil {
		t.Fatalf("budget status: %v", err)
	}

	if spent != 0 {
		t.Errorf("spent = %f, want 0", spent)
	}
	if remaining != 5.00 {
		t.Errorf("remaining = %f, want 5.00", remaining)
	}
}
