package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func makePipelineEvent(issueNumber int, from, to string, at time.Time) PipelineEvent {
	return PipelineEvent{
		IssueNumber: issueNumber,
		Project:     "samverk/samverk",
		FromStage:   from,
		ToStage:     to,
		TriggeredBy: "dispatcher",
		OccurredAt:  at,
	}
}

func TestPipelineEvent_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC().Truncate(time.Second)
	ev := PipelineEvent{
		IssueNumber: 42, Project: "owner/repo",
		FromStage: "status:queued", ToStage: "status:claimed",
		TriggeredBy: "dispatcher", OccurredAt: ts,
	}
	if err := s.RecordPipelineEvent(ctx, ev); err != nil {
		t.Fatalf("RecordPipelineEvent: %v", err)
	}
	epoch := time.Unix(0, 0).UTC()
	events, err := s.GetPipelineEvents(ctx, 42, epoch, 0)
	if err != nil {
		t.Fatalf("GetPipelineEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	got := events[0]
	if got.ID == 0 {
		t.Error("expected auto-assigned ID > 0")
	}
	if got.IssueNumber != 42 {
		t.Errorf("IssueNumber = %d, want 42", got.IssueNumber)
	}
	if got.FromStage != "status:queued" {
		t.Errorf("FromStage = %q, want %q", got.FromStage, "status:queued")
	}
	if got.ToStage != "status:claimed" {
		t.Errorf("ToStage = %q, want %q", got.ToStage, "status:claimed")
	}
	if !got.OccurredAt.Equal(ts) {
		t.Errorf("OccurredAt = %v, want %v", got.OccurredAt, ts)
	}
}

func TestPipelineEvent_SinceFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)
	old := base.Add(-2 * time.Hour)
	recent := base.Add(-30 * time.Minute)
	cutoff := base.Add(-1 * time.Hour)
	if err := s.RecordPipelineEvent(ctx, makePipelineEvent(1, "", "status:claimed", old)); err != nil {
		t.Fatalf("save old: %v", err)
	}
	if err := s.RecordPipelineEvent(ctx, makePipelineEvent(1, "status:claimed", "status:in-progress", recent)); err != nil {
		t.Fatalf("save recent: %v", err)
	}
	events, err := s.GetPipelineEvents(ctx, 1, cutoff, 0)
	if err != nil {
		t.Fatalf("GetPipelineEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event after cutoff, got %d", len(events))
	}
	if events[0].ToStage != "status:in-progress" {
		t.Errorf("expected status:in-progress, got %q", events[0].ToStage)
	}
}

func TestPipelineEvent_LimitEnforced(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)
	epoch := time.Unix(0, 0).UTC()
	for i, to := range []string{"status:claimed", "status:in-progress", "status:needs-qc"} {
		if err := s.RecordPipelineEvent(ctx, makePipelineEvent(5, "", to, base.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
	events, err := s.GetPipelineEvents(ctx, 5, epoch, 2)
	if err != nil {
		t.Fatalf("GetPipelineEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2, got %d", len(events))
	}
	if events[0].ToStage != "status:claimed" {
		t.Errorf("expected status:claimed first, got %q", events[0].ToStage)
	}
}

func TestPipelineEvent_IssueFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	epoch := time.Unix(0, 0).UTC()
	base := time.Now().UTC()
	if err := s.RecordPipelineEvent(ctx, makePipelineEvent(10, "", "status:claimed", base)); err != nil {
		t.Fatalf("save 10: %v", err)
	}
	if err := s.RecordPipelineEvent(ctx, makePipelineEvent(20, "", "status:claimed", base)); err != nil {
		t.Fatalf("save 20: %v", err)
	}
	events, err := s.GetPipelineEvents(ctx, 10, epoch, 0)
	if err != nil {
		t.Fatalf("GetPipelineEvents: %v", err)
	}
	if len(events) != 1 || events[0].IssueNumber != 10 {
		t.Errorf("expected 1 event for issue 10, got %d events", len(events))
	}
}

func TestPipelineEvent_AllIssues(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	epoch := time.Unix(0, 0).UTC()
	base := time.Now().UTC()
	for _, n := range []int{1, 2, 3} {
		if err := s.RecordPipelineEvent(ctx, makePipelineEvent(n, "", "status:claimed", base)); err != nil {
			t.Fatalf("save %d: %v", n, err)
		}
	}
	events, err := s.GetPipelineEvents(ctx, 0, epoch, 0)
	if err != nil {
		t.Fatalf("GetPipelineEvents(all): %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3, got %d", len(events))
	}
}

func TestPipelineEvent_Empty(t *testing.T) {
	s := newTestStore(t)
	epoch := time.Unix(0, 0).UTC()
	events, err := s.GetPipelineEvents(context.Background(), 1, epoch, 0)
	if err != nil {
		t.Fatalf("GetPipelineEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0, got %d", len(events))
	}
}

func TestPipelineEvent_Persistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pipeline.db")
	s1, err := New(dbPath)
	if err != nil {
		t.Fatalf("open store 1: %v", err)
	}
	ctx := context.Background()
	ev := makePipelineEvent(99, "status:queued", "status:claimed", time.Now().UTC().Truncate(time.Second))
	if err := s1.RecordPipelineEvent(ctx, ev); err != nil {
		t.Fatalf("RecordPipelineEvent: %v", err)
	}
	_ = s1.Close()

	s2, err := New(dbPath)
	if err != nil {
		t.Fatalf("open store 2: %v", err)
	}
	defer func() { _ = s2.Close() }()

	epoch := time.Unix(0, 0).UTC()
	events, err := s2.GetPipelineEvents(ctx, 99, epoch, 0)
	if err != nil {
		t.Fatalf("GetPipelineEvents after restart: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1, got %d", len(events))
	}
	if events[0].FromStage != "status:queued" {
		t.Errorf("FromStage = %q, want status:queued", events[0].FromStage)
	}
}
