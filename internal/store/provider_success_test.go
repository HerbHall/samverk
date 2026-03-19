package store

import (
	"context"
	"testing"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

func TestLatestSuccessByProvider_NoSessions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	got, err := s.LatestSuccessByProvider(ctx)
	if err != nil {
		t.Fatalf("LatestSuccessByProvider: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestLatestSuccessByProvider_MultipleSessions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)

	// Provider A: one completed (later), one failed.
	completedA := base.Add(2 * time.Hour)
	finA := completedA
	sessA1 := &models.Session{
		IssueNumber: 1,
		AgentType:   "code-gen",
		Provider:    "provider-a",
		Model:       "model-a",
		Status:      models.SessionStatusCompleted,
		StartedAt:   base,
		FinishedAt:  &finA,
	}
	if err := s.CreateSession(ctx, sessA1); err != nil {
		t.Fatalf("create sessA1: %v", err)
	}

	// Provider A: failed session — must not appear as last success.
	sessA2 := &models.Session{
		IssueNumber: 2,
		AgentType:   "code-gen",
		Provider:    "provider-a",
		Model:       "model-a",
		Status:      models.SessionStatusFailed,
		StartedAt:   base.Add(3 * time.Hour),
	}
	if err := s.CreateSession(ctx, sessA2); err != nil {
		t.Fatalf("create sessA2: %v", err)
	}

	// Provider B: one completed.
	completedB := base.Add(1 * time.Hour)
	finB := completedB
	sessB := &models.Session{
		IssueNumber: 3,
		AgentType:   "triage",
		Provider:    "provider-b",
		Model:       "model-b",
		Status:      models.SessionStatusCompleted,
		StartedAt:   base,
		FinishedAt:  &finB,
	}
	if err := s.CreateSession(ctx, sessB); err != nil {
		t.Fatalf("create sessB: %v", err)
	}

	got, err := s.LatestSuccessByProvider(ctx)
	if err != nil {
		t.Fatalf("LatestSuccessByProvider: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 providers, got %d: %v", len(got), got)
	}

	if !got["provider-a"].Equal(completedA) {
		t.Errorf("provider-a: got %v, want %v", got["provider-a"], completedA)
	}
	if !got["provider-b"].Equal(completedB) {
		t.Errorf("provider-b: got %v, want %v", got["provider-b"], completedB)
	}
}
