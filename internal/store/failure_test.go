package store

import (
	"context"
	"testing"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

// makeFailureEvent is a test helper that constructs a FailureEvent with
// sensible defaults. Fields can be overridden by the caller after construction.
func makeFailureEvent(issueNumber int, fc models.FailureClass, ts time.Time) *models.FailureEvent {
	return &models.FailureEvent{
		IssueNumber:   issueNumber,
		SessionID:     "sess-001",
		FailureClass:  fc,
		ErrorMessage:  "test error",
		AgentType:     "code-gen",
		Provider:      "claude",
		AttemptNumber: 1,
		Duration:      5 * time.Second,
		Timestamp:     ts,
	}
}

// TestSaveFailureEvent_RoundTrip saves one event then lists it back and
// verifies every persisted field survives the round-trip.
func TestSaveFailureEvent_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ts := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	e := makeFailureEvent(42, models.FailureClassTimeout, ts)
	e.SessionID = "sess-abc"
	e.ErrorMessage = "context deadline exceeded"
	e.AgentType = "qc"
	e.Provider = "ollama"
	e.AttemptNumber = 2
	e.Duration = 30 * time.Second

	if err := s.SaveFailureEvent(ctx, e); err != nil {
		t.Fatalf("SaveFailureEvent: %v", err)
	}
	if e.ID == "" {
		t.Fatal("expected ID to be generated")
	}

	epoch := time.Unix(0, 0).UTC()
	events, err := s.ListFailureEvents(ctx, epoch, 0)
	if err != nil {
		t.Fatalf("ListFailureEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	got := events[0]
	if got.ID != e.ID {
		t.Errorf("ID = %q, want %q", got.ID, e.ID)
	}
	if got.IssueNumber != 42 {
		t.Errorf("IssueNumber = %d, want 42", got.IssueNumber)
	}
	if got.SessionID != "sess-abc" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "sess-abc")
	}
	if got.FailureClass != models.FailureClassTimeout {
		t.Errorf("FailureClass = %q, want %q", got.FailureClass, models.FailureClassTimeout)
	}
	if got.ErrorMessage != "context deadline exceeded" {
		t.Errorf("ErrorMessage = %q, want %q", got.ErrorMessage, "context deadline exceeded")
	}
	if got.AgentType != "qc" {
		t.Errorf("AgentType = %q, want %q", got.AgentType, "qc")
	}
	if got.Provider != "ollama" {
		t.Errorf("Provider = %q, want %q", got.Provider, "ollama")
	}
	if got.AttemptNumber != 2 {
		t.Errorf("AttemptNumber = %d, want 2", got.AttemptNumber)
	}
	if got.Duration != 30*time.Second {
		t.Errorf("Duration = %v, want 30s", got.Duration)
	}
	if !got.Timestamp.Equal(ts) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, ts)
	}
}

// TestSaveFailureEvent_IDGeneratedOnEmpty confirms that an empty ID is
// populated by SaveFailureEvent.
func TestSaveFailureEvent_IDGeneratedOnEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	e := makeFailureEvent(1, models.FailureClassUnknown, time.Now().UTC())
	e.ID = "" // force generation

	if err := s.SaveFailureEvent(ctx, e); err != nil {
		t.Fatalf("SaveFailureEvent: %v", err)
	}
	if e.ID == "" {
		t.Error("expected ID to be populated after save")
	}
}

// TestListFailureEvents_SinceFilter verifies that events before the since
// boundary are excluded and events at or after are included.
func TestListFailureEvents_SinceFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	old := base.Add(-2 * time.Hour)
	recent := base.Add(-30 * time.Minute)
	cutoff := base.Add(-1 * time.Hour)

	if err := s.SaveFailureEvent(ctx, makeFailureEvent(1, models.FailureClassAuth, old)); err != nil {
		t.Fatalf("save old event: %v", err)
	}
	if err := s.SaveFailureEvent(ctx, makeFailureEvent(2, models.FailureClassBudget, recent)); err != nil {
		t.Fatalf("save recent event: %v", err)
	}

	events, err := s.ListFailureEvents(ctx, cutoff, 0)
	if err != nil {
		t.Fatalf("ListFailureEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event after cutoff, got %d", len(events))
	}
	if events[0].IssueNumber != 2 {
		t.Errorf("expected issue 2, got %d", events[0].IssueNumber)
	}
}

// TestListFailureEvents_LimitEnforced confirms that limit=N returns at most N
// events and that they are ordered newest-first.
func TestListFailureEvents_LimitEnforced(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	epoch := time.Unix(0, 0).UTC()

	for i := range 5 {
		ts := base.Add(time.Duration(i) * time.Minute)
		e := makeFailureEvent(i+1, models.FailureClassTimeout, ts)
		if err := s.SaveFailureEvent(ctx, e); err != nil {
			t.Fatalf("save event %d: %v", i, err)
		}
	}

	events, err := s.ListFailureEvents(ctx, epoch, 3)
	if err != nil {
		t.Fatalf("ListFailureEvents: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events with limit=3, got %d", len(events))
	}
	// Newest-first: issue 5 has the latest timestamp.
	if events[0].IssueNumber != 5 {
		t.Errorf("expected newest event first (issue 5), got issue %d", events[0].IssueNumber)
	}
}

// TestListFailureEvents_ZeroLimitReturnsAll confirms that limit=0 lifts the
// limit and returns all matching rows.
func TestListFailureEvents_ZeroLimitReturnsAll(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	epoch := time.Unix(0, 0).UTC()
	for i := range 6 {
		e := makeFailureEvent(i+1, models.FailureClassUnknown, time.Now().UTC())
		if err := s.SaveFailureEvent(ctx, e); err != nil {
			t.Fatalf("save event %d: %v", i, err)
		}
	}

	events, err := s.ListFailureEvents(ctx, epoch, 0)
	if err != nil {
		t.Fatalf("ListFailureEvents: %v", err)
	}
	if len(events) != 6 {
		t.Errorf("expected 6 events with limit=0, got %d", len(events))
	}
}

// TestListFailureEvents_Empty confirms an empty store returns a nil/empty slice
// without error.
func TestListFailureEvents_Empty(t *testing.T) {
	s := newTestStore(t)
	epoch := time.Unix(0, 0).UTC()

	events, err := s.ListFailureEvents(context.Background(), epoch, 0)
	if err != nil {
		t.Fatalf("ListFailureEvents on empty store: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

// TestCountFailuresByIssue verifies counts for multiple events across issues.
func TestCountFailuresByIssue(t *testing.T) {
	tests := []struct {
		name        string
		events      []int // issue numbers to insert
		queryIssue  int
		wantCount   int
	}{
		{
			name:       "no events for issue",
			events:     []int{10, 10, 10},
			queryIssue: 99,
			wantCount:  0,
		},
		{
			name:       "single event",
			events:     []int{7},
			queryIssue: 7,
			wantCount:  1,
		},
		{
			name:       "multiple events same issue",
			events:     []int{3, 3, 3, 4, 4},
			queryIssue: 3,
			wantCount:  3,
		},
		{
			name:       "other issue events not counted",
			events:     []int{3, 3, 3, 4, 4},
			queryIssue: 4,
			wantCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newTestStore(t)
			ctx := context.Background()

			for _, issueNum := range tt.events {
				e := makeFailureEvent(issueNum, models.FailureClassUnknown, time.Now().UTC())
				if err := s.SaveFailureEvent(ctx, e); err != nil {
					t.Fatalf("save event for issue %d: %v", issueNum, err)
				}
			}

			count, err := s.CountFailuresByIssue(ctx, tt.queryIssue)
			if err != nil {
				t.Fatalf("CountFailuresByIssue: %v", err)
			}
			if count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}
		})
	}
}

// TestGetFailureSummary_ByClass confirms that failure counts are bucketed
// correctly by failure class.
func TestGetFailureSummary_ByClass(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	epoch := time.Unix(0, 0).UTC()
	base := time.Now().UTC().Truncate(time.Second)

	classEvents := map[models.FailureClass]int{
		models.FailureClassAuth:    3,
		models.FailureClassTimeout: 2,
		models.FailureClassUnknown: 1,
	}
	for fc, n := range classEvents {
		for i := range n {
			e := makeFailureEvent(i+1, fc, base)
			if err := s.SaveFailureEvent(ctx, e); err != nil {
				t.Fatalf("save %s event: %v", fc, err)
			}
		}
	}

	summary, err := s.GetFailureSummary(ctx, epoch)
	if err != nil {
		t.Fatalf("GetFailureSummary: %v", err)
	}

	if summary.TotalFailures != 6 {
		t.Errorf("TotalFailures = %d, want 6", summary.TotalFailures)
	}
	for fc, want := range classEvents {
		got := summary.ByClass[fc]
		if got != want {
			t.Errorf("ByClass[%q] = %d, want %d", fc, got, want)
		}
	}
}

// TestGetFailureSummary_TopIssuesOrdering confirms TopIssues is ordered by
// failure count descending.
func TestGetFailureSummary_TopIssuesOrdering(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	epoch := time.Unix(0, 0).UTC()
	base := time.Now().UTC().Truncate(time.Second)

	// Issue 10 gets 4 failures, issue 20 gets 1, issue 30 gets 7.
	issueCounts := map[int]int{10: 4, 20: 1, 30: 7}
	for issueNum, n := range issueCounts {
		for range n {
			e := makeFailureEvent(issueNum, models.FailureClassUnknown, base)
			if err := s.SaveFailureEvent(ctx, e); err != nil {
				t.Fatalf("save event for issue %d: %v", issueNum, err)
			}
		}
	}

	summary, err := s.GetFailureSummary(ctx, epoch)
	if err != nil {
		t.Fatalf("GetFailureSummary: %v", err)
	}

	if len(summary.TopIssues) != 3 {
		t.Fatalf("TopIssues len = %d, want 3", len(summary.TopIssues))
	}
	// First entry must have the highest count (issue 30 with 7).
	if summary.TopIssues[0].IssueNumber != 30 {
		t.Errorf("TopIssues[0].IssueNumber = %d, want 30", summary.TopIssues[0].IssueNumber)
	}
	if summary.TopIssues[0].Count != 7 {
		t.Errorf("TopIssues[0].Count = %d, want 7", summary.TopIssues[0].Count)
	}
}

// TestGetFailureSummary_LoopingIssues confirms that only issues with 5 or more
// failures appear in LoopingIssues.
func TestGetFailureSummary_LoopingIssues(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	epoch := time.Unix(0, 0).UTC()
	base := time.Now().UTC().Truncate(time.Second)

	// Issue 1: 4 failures (below threshold, must NOT appear).
	// Issue 2: 5 failures (at threshold, MUST appear).
	// Issue 3: 6 failures (above threshold, MUST appear).
	issueCounts := map[int]int{1: 4, 2: 5, 3: 6}
	for issueNum, n := range issueCounts {
		for range n {
			e := makeFailureEvent(issueNum, models.FailureClassTimeout, base)
			if err := s.SaveFailureEvent(ctx, e); err != nil {
				t.Fatalf("save event for issue %d: %v", issueNum, err)
			}
		}
	}

	summary, err := s.GetFailureSummary(ctx, epoch)
	if err != nil {
		t.Fatalf("GetFailureSummary: %v", err)
	}

	if len(summary.LoopingIssues) != 2 {
		t.Errorf("LoopingIssues len = %d, want 2", len(summary.LoopingIssues))
	}

	looping := make(map[int]int, len(summary.LoopingIssues))
	for i := range summary.LoopingIssues {
		looping[summary.LoopingIssues[i].IssueNumber] = summary.LoopingIssues[i].Count
	}
	if _, ok := looping[1]; ok {
		t.Error("issue 1 (4 failures) must not appear in LoopingIssues")
	}
	if looping[2] != 5 {
		t.Errorf("LoopingIssues: issue 2 count = %d, want 5", looping[2])
	}
	if looping[3] != 6 {
		t.Errorf("LoopingIssues: issue 3 count = %d, want 6", looping[3])
	}
}

// TestGetFailureSummary_ProviderHealth confirms that provider failure counts
// are aggregated and empty-provider events are excluded.
func TestGetFailureSummary_ProviderHealth(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	epoch := time.Unix(0, 0).UTC()
	base := time.Now().UTC().Truncate(time.Second)

	save := func(issueNum int, provider string) {
		t.Helper()
		e := makeFailureEvent(issueNum, models.FailureClassProviderDown, base)
		e.Provider = provider
		if err := s.SaveFailureEvent(ctx, e); err != nil {
			t.Fatalf("save event provider=%q: %v", provider, err)
		}
	}

	save(1, "claude")
	save(2, "claude")
	save(3, "ollama")
	save(4, "") // no provider — must be excluded from ProviderHealth

	summary, err := s.GetFailureSummary(ctx, epoch)
	if err != nil {
		t.Fatalf("GetFailureSummary: %v", err)
	}

	if summary.ProviderHealth["claude"] != 2 {
		t.Errorf("ProviderHealth[claude] = %d, want 2", summary.ProviderHealth["claude"])
	}
	if summary.ProviderHealth["ollama"] != 1 {
		t.Errorf("ProviderHealth[ollama] = %d, want 1", summary.ProviderHealth["ollama"])
	}
	if _, ok := summary.ProviderHealth[""]; ok {
		t.Error("empty-provider entry must not appear in ProviderHealth")
	}
}

// TestGetFailureSummary_SinceFilter confirms that events before the since
// boundary are excluded from the summary.
func TestGetFailureSummary_SinceFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	old := base.Add(-3 * time.Hour)
	recent := base.Add(-30 * time.Minute)
	cutoff := base.Add(-1 * time.Hour)

	if err := s.SaveFailureEvent(ctx, makeFailureEvent(1, models.FailureClassAuth, old)); err != nil {
		t.Fatalf("save old event: %v", err)
	}
	if err := s.SaveFailureEvent(ctx, makeFailureEvent(2, models.FailureClassAuth, recent)); err != nil {
		t.Fatalf("save recent event: %v", err)
	}

	summary, err := s.GetFailureSummary(ctx, cutoff)
	if err != nil {
		t.Fatalf("GetFailureSummary: %v", err)
	}
	if summary.TotalFailures != 1 {
		t.Errorf("TotalFailures = %d, want 1", summary.TotalFailures)
	}
	if summary.ByClass[models.FailureClassAuth] != 1 {
		t.Errorf("ByClass[auth] = %d, want 1", summary.ByClass[models.FailureClassAuth])
	}
}

// TestGetFailureSummary_Empty confirms the summary is safe to read on an empty
// store — all maps are initialised and counts are zero.
func TestGetFailureSummary_Empty(t *testing.T) {
	s := newTestStore(t)
	epoch := time.Unix(0, 0).UTC()

	summary, err := s.GetFailureSummary(context.Background(), epoch)
	if err != nil {
		t.Fatalf("GetFailureSummary on empty store: %v", err)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.TotalFailures != 0 {
		t.Errorf("TotalFailures = %d, want 0", summary.TotalFailures)
	}
	if summary.ByClass == nil {
		t.Error("ByClass map must be initialised")
	}
	if summary.ProviderHealth == nil {
		t.Error("ProviderHealth map must be initialised")
	}
	if len(summary.TopIssues) != 0 {
		t.Errorf("TopIssues len = %d, want 0", len(summary.TopIssues))
	}
	if len(summary.LoopingIssues) != 0 {
		t.Errorf("LoopingIssues len = %d, want 0", len(summary.LoopingIssues))
	}
}

// TestGetIssueFailureCount_Nonexistent confirms that querying a non-existent
// issue returns 0 without error.
func TestGetIssueFailureCount_Nonexistent(t *testing.T) {
	s := newTestStore(t)

	count, err := s.GetIssueFailureCount(context.Background(), 999)
	if err != nil {
		t.Fatalf("GetIssueFailureCount: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

// TestIncrementIssueFailureCount_CreatesAndIncrements confirms that the first
// call creates the record with count=1 and subsequent calls increment by one.
func TestIncrementIssueFailureCount_CreatesAndIncrements(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tests := []struct {
		call      int
		wantCount int
	}{
		{call: 1, wantCount: 1},
		{call: 2, wantCount: 2},
		{call: 3, wantCount: 3},
	}

	for _, tt := range tests {
		count, err := s.IncrementIssueFailureCount(ctx, 42)
		if err != nil {
			t.Fatalf("call %d: IncrementIssueFailureCount: %v", tt.call, err)
		}
		if count != tt.wantCount {
			t.Errorf("call %d: count = %d, want %d", tt.call, count, tt.wantCount)
		}
	}
}

// TestIncrementIssueFailureCount_IndependentIssues confirms that increments for
// different issue numbers do not interfere with each other.
func TestIncrementIssueFailureCount_IndependentIssues(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.IncrementIssueFailureCount(ctx, 1); err != nil {
		t.Fatalf("increment issue 1: %v", err)
	}
	if _, err := s.IncrementIssueFailureCount(ctx, 1); err != nil {
		t.Fatalf("increment issue 1 again: %v", err)
	}
	if _, err := s.IncrementIssueFailureCount(ctx, 2); err != nil {
		t.Fatalf("increment issue 2: %v", err)
	}

	c1, err := s.GetIssueFailureCount(ctx, 1)
	if err != nil {
		t.Fatalf("GetIssueFailureCount(1): %v", err)
	}
	c2, err := s.GetIssueFailureCount(ctx, 2)
	if err != nil {
		t.Fatalf("GetIssueFailureCount(2): %v", err)
	}

	if c1 != 2 {
		t.Errorf("issue 1 count = %d, want 2", c1)
	}
	if c2 != 1 {
		t.Errorf("issue 2 count = %d, want 1", c2)
	}
}

// TestClearIssueFailureCount confirms the record is removed and a subsequent
// GetIssueFailureCount returns 0.
func TestClearIssueFailureCount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.IncrementIssueFailureCount(ctx, 7); err != nil {
		t.Fatalf("increment: %v", err)
	}
	if _, err := s.IncrementIssueFailureCount(ctx, 7); err != nil {
		t.Fatalf("increment: %v", err)
	}

	if err := s.ClearIssueFailureCount(ctx, 7); err != nil {
		t.Fatalf("ClearIssueFailureCount: %v", err)
	}

	count, err := s.GetIssueFailureCount(ctx, 7)
	if err != nil {
		t.Fatalf("GetIssueFailureCount after clear: %v", err)
	}
	if count != 0 {
		t.Errorf("count after clear = %d, want 0", count)
	}
}

// TestClearIssueFailureCount_Nonexistent confirms that clearing a non-existent
// record is a no-op and does not return an error.
func TestClearIssueFailureCount_Nonexistent(t *testing.T) {
	s := newTestStore(t)

	if err := s.ClearIssueFailureCount(context.Background(), 999); err != nil {
		t.Errorf("ClearIssueFailureCount on nonexistent record: %v", err)
	}
}

// TestIncrementAfterClear confirms that incrementing after a clear resets the
// counter to 1, as if the record never existed.
func TestIncrementAfterClear(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Increment three times.
	for range 3 {
		if _, err := s.IncrementIssueFailureCount(ctx, 5); err != nil {
			t.Fatalf("increment: %v", err)
		}
	}

	// Clear the record.
	if err := s.ClearIssueFailureCount(ctx, 5); err != nil {
		t.Fatalf("clear: %v", err)
	}

	// First increment after clear must return 1.
	count, err := s.IncrementIssueFailureCount(ctx, 5)
	if err != nil {
		t.Fatalf("increment after clear: %v", err)
	}
	if count != 1 {
		t.Errorf("count after clear+increment = %d, want 1", count)
	}
}

// TestGetFailureSummary_Since confirms the Since field on the returned summary
// matches the argument passed to GetFailureSummary.
func TestGetFailureSummary_SinceField(t *testing.T) {
	s := newTestStore(t)

	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	summary, err := s.GetFailureSummary(context.Background(), since)
	if err != nil {
		t.Fatalf("GetFailureSummary: %v", err)
	}
	if !summary.Since.Equal(since) {
		t.Errorf("Since = %v, want %v", summary.Since, since)
	}
}
