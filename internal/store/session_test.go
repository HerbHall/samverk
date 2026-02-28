package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

func TestCreateAndGetSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	sess := &models.Session{
		IssueNumber: 42,
		AgentType:   "code-gen",
		Provider:    "ollama",
		Model:       "qwen2.5-coder:7b",
		Status:      models.SessionStatusPending,
		StartedAt:   now,
	}

	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected ID to be generated")
	}

	got, err := s.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	if got.IssueNumber != 42 {
		t.Errorf("issue_number = %d, want 42", got.IssueNumber)
	}
	if got.AgentType != "code-gen" {
		t.Errorf("agent_type = %q, want %q", got.AgentType, "code-gen")
	}
	if got.Provider != "ollama" {
		t.Errorf("provider = %q, want %q", got.Provider, "ollama")
	}
	if got.Model != "qwen2.5-coder:7b" {
		t.Errorf("model = %q, want %q", got.Model, "qwen2.5-coder:7b")
	}
	if got.Status != models.SessionStatusPending {
		t.Errorf("status = %q, want %q", got.Status, models.SessionStatusPending)
	}
	if got.FinishedAt != nil {
		t.Errorf("finished_at = %v, want nil", got.FinishedAt)
	}
}

func TestUpdateSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	sess := &models.Session{
		IssueNumber: 10,
		AgentType:   "qc",
		Provider:    "claude",
		Model:       "opus-4",
		Status:      models.SessionStatusActive,
		StartedAt:   now,
	}

	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("create: %v", err)
	}

	finished := now.Add(5 * time.Minute)
	sess.Status = models.SessionStatusCompleted
	sess.FinishedAt = &finished
	sess.Error = ""

	if err := s.UpdateSession(ctx, sess); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := s.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if got.Status != models.SessionStatusCompleted {
		t.Errorf("status = %q, want %q", got.Status, models.SessionStatusCompleted)
	}
	if got.FinishedAt == nil {
		t.Fatal("expected finished_at to be set")
	}
	if !got.FinishedAt.Equal(finished) {
		t.Errorf("finished_at = %v, want %v", got.FinishedAt, finished)
	}
}

func TestListSessionsByStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// Create sessions with different statuses.
	for i, status := range []models.SessionStatus{
		models.SessionStatusPending,
		models.SessionStatusActive,
		models.SessionStatusPending,
		models.SessionStatusCompleted,
	} {
		sess := &models.Session{
			IssueNumber: i + 1,
			AgentType:   "code-gen",
			Provider:    "ollama",
			Model:       "test-model",
			Status:      status,
			StartedAt:   now.Add(time.Duration(i) * time.Minute),
		}
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatalf("create session %d: %v", i, err)
		}
	}

	pending, err := s.ListSessions(ctx, models.SessionStatusPending)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("pending count = %d, want 2", len(pending))
	}

	active, err := s.ListSessions(ctx, models.SessionStatusActive)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("active count = %d, want 1", len(active))
	}
}

func TestGetSessionNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetSession(ctx, "nonexistent-id")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestListSessionsEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sessions, err := s.ListSessions(ctx, models.SessionStatusActive)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("sessions count = %d, want 0", len(sessions))
	}
}
