package store

import (
	"context"
	"testing"
	"time"
)

func TestSaveAndListTriageDecisions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	d := &TriageDecision{
		IssueNumber:      42,
		Project:          "owner/repo",
		Verdict:          "auto_unblock",
		Reasoning:        "issue is safe to automate",
		CriteriaMet:      `[{"criterion":"recoverable","passed":true}]`,
		DocsReferenced:   `["docs/architecture.md"]`,
		SubIssuesCreated: "[]",
		CreatedAt:        time.Now().UTC(),
	}

	if err := s.SaveTriageDecision(ctx, d); err != nil {
		t.Fatalf("save: %v", err)
	}

	decisions, err := s.ListTriageDecisions(ctx, time.Now().Add(-time.Hour), 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].Verdict != "auto_unblock" {
		t.Errorf("verdict = %q, want %q", decisions[0].Verdict, "auto_unblock")
	}
	if decisions[0].IssueNumber != 42 {
		t.Errorf("issue_number = %d, want 42", decisions[0].IssueNumber)
	}
	if decisions[0].Reasoning != "issue is safe to automate" {
		t.Errorf("reasoning = %q, want %q", decisions[0].Reasoning, "issue is safe to automate")
	}
}

func TestGetTriageDecisionForIssue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// No decision yet.
	_, err := s.GetTriageDecisionForIssue(ctx, 99)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Save one.
	d := &TriageDecision{
		IssueNumber:      99,
		Project:          "owner/repo",
		Verdict:          "confirmed_human",
		Reasoning:        "security change",
		CriteriaMet:      "[]",
		DocsReferenced:   "[]",
		SubIssuesCreated: "[]",
		CreatedAt:        time.Now().UTC(),
	}
	if err = s.SaveTriageDecision(ctx, d); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetTriageDecisionForIssue(ctx, 99)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Verdict != "confirmed_human" {
		t.Errorf("verdict = %q, want %q", got.Verdict, "confirmed_human")
	}
}

func TestUpdateTriageReview(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	d := &TriageDecision{
		IssueNumber:      55,
		Project:          "owner/repo",
		Verdict:          "auto_unblock",
		Reasoning:        "safe to automate",
		CriteriaMet:      "[]",
		DocsReferenced:   "[]",
		SubIssuesCreated: "[]",
		CreatedAt:        time.Now().UTC(),
	}
	if err := s.SaveTriageDecision(ctx, d); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetTriageDecisionForIssue(ctx, 55)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if err = s.UpdateTriageReview(ctx, got.ID, "alice", "correct"); err != nil {
		t.Fatalf("update review: %v", err)
	}

	updated, err := s.GetTriageDecisionForIssue(ctx, 55)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if updated.ReviewedBy != "alice" {
		t.Errorf("reviewed_by = %q, want %q", updated.ReviewedBy, "alice")
	}
	if updated.ReviewOutcome != "correct" {
		t.Errorf("review_outcome = %q, want %q", updated.ReviewOutcome, "correct")
	}
}

func TestListTriageDecisionsWithLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i := range 5 {
		d := &TriageDecision{
			IssueNumber:      i + 1,
			Project:          "owner/repo",
			Verdict:          "confirmed_human",
			Reasoning:        "test",
			CriteriaMet:      "[]",
			DocsReferenced:   "[]",
			SubIssuesCreated: "[]",
			CreatedAt:        time.Now().UTC(),
		}
		if err := s.SaveTriageDecision(ctx, d); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	decisions, err := s.ListTriageDecisions(ctx, time.Now().Add(-time.Hour), 3)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(decisions) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(decisions))
	}
}

func TestUpdateTriageReviewNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.UpdateTriageReview(ctx, 999, "alice", "correct")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
