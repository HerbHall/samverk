package store

import (
	"context"
	"testing"
	"time"
)

func TestSyncIssues(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	issues := []CachedIssue{
		{Number: 1, Title: "First", State: "open", Labels: []string{"bug"}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now},
		{Number: 2, Title: "Second", State: "open", Labels: []string{"feat", "priority:high"}, Assignees: []string{"alice"}, CreatedAt: now, UpdatedAt: now},
		{Number: 3, Title: "Third", State: "closed", Labels: []string{"bug"}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now, ClosedAt: &now},
	}

	if err := s.SyncIssues(ctx, "testproj", issues); err != nil {
		t.Fatalf("sync issues: %v", err)
	}

	open, closed, err := s.GetIssueCounts(ctx, "testproj")
	if err != nil {
		t.Fatalf("get counts: %v", err)
	}
	if open != 2 {
		t.Errorf("open = %d, want 2", open)
	}
	if closed != 1 {
		t.Errorf("closed = %d, want 1", closed)
	}
}

func TestSyncIssuesUpsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	issues := []CachedIssue{
		{Number: 1, Title: "Original", State: "open", Labels: []string{"bug"}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now},
	}
	if err := s.SyncIssues(ctx, "proj", issues); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Update: close issue #1.
	closed := now.Add(time.Hour)
	issues[0].State = "closed"
	issues[0].Title = "Updated"
	issues[0].ClosedAt = &closed
	if err := s.SyncIssues(ctx, "proj", issues); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	open, closedCount, err := s.GetIssueCounts(ctx, "proj")
	if err != nil {
		t.Fatalf("get counts: %v", err)
	}
	if open != 0 {
		t.Errorf("open = %d, want 0", open)
	}
	if closedCount != 1 {
		t.Errorf("closed = %d, want 1", closedCount)
	}
}

func TestDeleteStaleIssues(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	issues := []CachedIssue{
		{Number: 1, Title: "Stays open", State: "open", Labels: []string{}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now},
		{Number: 2, Title: "Gets closed", State: "open", Labels: []string{}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now},
		{Number: 3, Title: "Already closed", State: "closed", Labels: []string{}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now, ClosedAt: &now},
	}
	if err := s.SyncIssues(ctx, "proj", issues); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Only #1 is still open on the forge.
	if err := s.DeleteStaleIssues(ctx, "proj", []int{1}); err != nil {
		t.Fatalf("delete stale: %v", err)
	}

	open, closed, err := s.GetIssueCounts(ctx, "proj")
	if err != nil {
		t.Fatalf("get counts: %v", err)
	}
	if open != 1 {
		t.Errorf("open = %d, want 1 (only #1)", open)
	}
	if closed != 1 {
		t.Errorf("closed = %d, want 1 (#3 preserved)", closed)
	}
}

func TestGetAllIssueCounts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	projA := []CachedIssue{
		{Number: 1, Title: "A1", State: "open", Labels: []string{}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now},
		{Number: 2, Title: "A2", State: "closed", Labels: []string{}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now, ClosedAt: &now},
	}
	projB := []CachedIssue{
		{Number: 10, Title: "B1", State: "open", Labels: []string{}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now},
		{Number: 11, Title: "B2", State: "open", Labels: []string{}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now},
		{Number: 12, Title: "B3", State: "closed", Labels: []string{}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now, ClosedAt: &now},
	}

	if err := s.SyncIssues(ctx, "projA", projA); err != nil {
		t.Fatalf("sync A: %v", err)
	}
	if err := s.SyncIssues(ctx, "projB", projB); err != nil {
		t.Fatalf("sync B: %v", err)
	}

	counts, err := s.GetAllIssueCounts(ctx)
	if err != nil {
		t.Fatalf("get all: %v", err)
	}

	if c := counts["projA"]; c.Open != 1 || c.Closed != 1 || c.Total != 2 {
		t.Errorf("projA = %+v, want {1 1 2}", c)
	}
	if c := counts["projB"]; c.Open != 2 || c.Closed != 1 || c.Total != 3 {
		t.Errorf("projB = %+v, want {2 1 3}", c)
	}
}

func TestGetIssueLabelCounts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	issues := []CachedIssue{
		{Number: 1, Title: "A", State: "open", Labels: []string{"status:queued", "priority:high"}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now},
		{Number: 2, Title: "B", State: "open", Labels: []string{"status:queued", "status:needs-qc"}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now},
		{Number: 3, Title: "C", State: "closed", Labels: []string{"status:queued"}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now, ClosedAt: &now},
	}
	if err := s.SyncIssues(ctx, "proj", issues); err != nil {
		t.Fatalf("sync: %v", err)
	}

	counts, err := s.GetIssueLabelCounts(ctx, "proj")
	if err != nil {
		t.Fatalf("label counts: %v", err)
	}

	if counts["status:queued"] != 2 {
		t.Errorf("status:queued = %d, want 2 (only open issues)", counts["status:queued"])
	}
	if counts["priority:high"] != 1 {
		t.Errorf("priority:high = %d, want 1", counts["priority:high"])
	}
	if counts["status:needs-qc"] != 1 {
		t.Errorf("status:needs-qc = %d, want 1", counts["status:needs-qc"])
	}
}

func TestGetCacheSyncTime(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// No data yet.
	_, err := s.GetCacheSyncTime(ctx, "proj")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	now := time.Now()
	issues := []CachedIssue{
		{Number: 1, Title: "A", State: "open", Labels: []string{}, Assignees: []string{}, CreatedAt: now, UpdatedAt: now},
	}
	if err := s.SyncIssues(ctx, "proj", issues); err != nil {
		t.Fatalf("sync: %v", err)
	}

	syncTime, err := s.GetCacheSyncTime(ctx, "proj")
	if err != nil {
		t.Fatalf("get sync time: %v", err)
	}
	if time.Since(syncTime) > 5*time.Second {
		t.Errorf("sync time too old: %v", syncTime)
	}
}

func TestSyncIssuesEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Empty batch is a no-op.
	if err := s.SyncIssues(ctx, "proj", nil); err != nil {
		t.Fatalf("sync empty: %v", err)
	}

	open, closed, err := s.GetIssueCounts(ctx, "proj")
	if err != nil {
		t.Fatalf("get counts: %v", err)
	}
	if open != 0 || closed != 0 {
		t.Errorf("counts = %d/%d, want 0/0", open, closed)
	}
}
