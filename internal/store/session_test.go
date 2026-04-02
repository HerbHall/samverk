package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"samverk.dev/samverk/pkg/models"
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

func TestCheckpointHashRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	sess := &models.Session{
		IssueNumber: 77,
		AgentType:   "code-gen",
		Provider:    "claude-cli",
		Model:       "opus-4",
		Status:      models.SessionStatusActive,
		StartedAt:   now,
	}

	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Initially empty.
	got, err := s.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CheckpointHash != "" {
		t.Errorf("initial checkpoint_hash = %q, want empty", got.CheckpointHash)
	}

	// Update with a checkpoint hash.
	sess.CheckpointHash = "abc123def456"
	if err := s.UpdateSession(ctx, sess); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err = s.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.CheckpointHash != "abc123def456" {
		t.Errorf("checkpoint_hash = %q, want %q", got.CheckpointHash, "abc123def456")
	}

	// Verify it appears in list results too.
	sessions, err := s.ListSessions(ctx, models.SessionStatusActive)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions count = %d, want 1", len(sessions))
	}
	if sessions[0].CheckpointHash != "abc123def456" {
		t.Errorf("list checkpoint_hash = %q, want %q", sessions[0].CheckpointHash, "abc123def456")
	}
}

func TestPartialOutputRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	sess := &models.Session{
		IssueNumber: 99,
		AgentType:   "code-gen",
		Provider:    "claude-cli",
		Model:       "opus-4",
		Status:      models.SessionStatusActive,
		StartedAt:   now,
	}

	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Initially empty.
	got, err := s.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PartialOutput != "" {
		t.Errorf("initial partial_output = %q, want empty", got.PartialOutput)
	}

	// Update with partial output checkpoint.
	sess.PartialOutput = "streaming output chunk 1..."
	if err := s.UpdateSession(ctx, sess); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err = s.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.PartialOutput != "streaming output chunk 1..." {
		t.Errorf("partial_output = %q, want %q", got.PartialOutput, "streaming output chunk 1...")
	}

	// Verify it appears in list results too.
	sessions, err := s.ListSessions(ctx, models.SessionStatusActive)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions count = %d, want 1", len(sessions))
	}
	if sessions[0].PartialOutput != "streaming output chunk 1..." {
		t.Errorf("list partial_output = %q, want %q", sessions[0].PartialOutput, "streaming output chunk 1...")
	}
}

func TestMaxTurnsHitAndTurnsUsedRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	sess := &models.Session{
		IssueNumber: 88,
		AgentType:   "code-gen",
		Provider:    "claude",
		Model:       "opus-4",
		Status:      models.SessionStatusActive,
		StartedAt:   now,
	}

	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Initially should be false/0.
	got, err := s.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.MaxTurnsHit {
		t.Errorf("initial max_turns_hit = %v, want false", got.MaxTurnsHit)
	}
	if got.TurnsUsed != 0 {
		t.Errorf("initial turns_used = %d, want 0", got.TurnsUsed)
	}

	// Update with turns tracking.
	sess.MaxTurnsHit = true
	sess.TurnsUsed = 5
	if err := s.UpdateSession(ctx, sess); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err = s.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if !got.MaxTurnsHit {
		t.Errorf("max_turns_hit = %v, want true", got.MaxTurnsHit)
	}
	if got.TurnsUsed != 5 {
		t.Errorf("turns_used = %d, want 5", got.TurnsUsed)
	}

	// Verify it appears in list results too.
	sessions, err := s.ListSessions(ctx, models.SessionStatusActive)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions count = %d, want 1", len(sessions))
	}
	if !sessions[0].MaxTurnsHit {
		t.Errorf("list max_turns_hit = %v, want true", sessions[0].MaxTurnsHit)
	}
	if sessions[0].TurnsUsed != 5 {
		t.Errorf("list turns_used = %d, want 5", sessions[0].TurnsUsed)
	}
}
