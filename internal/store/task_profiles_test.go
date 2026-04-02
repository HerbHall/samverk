package store

import (
	"context"
	"testing"
	"time"

	"samverk.dev/samverk/pkg/models"
)

// seedCompletedSession inserts a completed session with specified duration.
func seedCompletedSession(t *testing.T, s *SQLiteStore, agentType, provider string, duration time.Duration) {
	t.Helper()
	ctx := context.Background()
	start := time.Now().UTC().Add(-duration).Truncate(time.Second)
	fin := start.Add(duration)
	sess := &models.Session{
		ID:          generateID(),
		IssueNumber: 1,
		AgentType:   agentType,
		Provider:    provider,
		Model:       "test-model",
		Status:      models.SessionStatusCompleted,
		StartedAt:   start,
		FinishedAt:  &fin,
		CreatedAt:   start,
		UpdatedAt:   fin,
	}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func TestUpdateTaskProfile_NoSessions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// No sessions → UpdateTaskProfile is a no-op.
	if err := s.UpdateTaskProfile(ctx, "code-gen", "claude"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prof, err := s.GetTaskProfile(ctx, "code-gen", "claude")
	if err != nil {
		t.Fatalf("GetTaskProfile: %v", err)
	}
	if prof != nil {
		t.Errorf("expected nil profile with no sessions, got %+v", prof)
	}
}

func TestUpdateTaskProfile_SingleSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedCompletedSession(t, s, "triage", "ollama", 30*time.Second)

	if err := s.UpdateTaskProfile(ctx, "triage", "ollama"); err != nil {
		t.Fatalf("UpdateTaskProfile: %v", err)
	}

	prof, err := s.GetTaskProfile(ctx, "triage", "ollama")
	if err != nil {
		t.Fatalf("GetTaskProfile: %v", err)
	}
	if prof == nil {
		t.Fatal("expected non-nil profile")
	}
	if prof.SampleCount != 1 {
		t.Errorf("SampleCount = %d, want 1", prof.SampleCount)
	}
	if prof.AvgDuration != 30*time.Second {
		t.Errorf("AvgDuration = %v, want 30s", prof.AvgDuration)
	}
}

func TestUpdateTaskProfile_MultipleSessionsPercentiles(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Seed 10 sessions with durations: 10s, 20s, …, 100s.
	durations := []time.Duration{
		10 * time.Second, 20 * time.Second, 30 * time.Second,
		40 * time.Second, 50 * time.Second, 60 * time.Second,
		70 * time.Second, 80 * time.Second, 90 * time.Second, 100 * time.Second,
	}
	for _, d := range durations {
		seedCompletedSession(t, s, "code-gen", "claude", d)
	}

	if err := s.UpdateTaskProfile(ctx, "code-gen", "claude"); err != nil {
		t.Fatalf("UpdateTaskProfile: %v", err)
	}

	prof, err := s.GetTaskProfile(ctx, "code-gen", "claude")
	if err != nil {
		t.Fatalf("GetTaskProfile: %v", err)
	}
	if prof.SampleCount != 10 {
		t.Errorf("SampleCount = %d, want 10", prof.SampleCount)
	}
	// Avg of 10..100s = 55s.
	if prof.AvgDuration != 55*time.Second {
		t.Errorf("AvgDuration = %v, want 55s", prof.AvgDuration)
	}
	// P50 of sorted [10..100]: idx = 50*10/100 = 5 → 60s.
	if prof.P50Duration != 60*time.Second {
		t.Errorf("P50 = %v, want 60s", prof.P50Duration)
	}
	// P90: idx = 90*10/100 = 9 → 100s.
	if prof.P90Duration != 100*time.Second {
		t.Errorf("P90 = %v, want 100s", prof.P90Duration)
	}
}

func TestListTaskProfiles_OrderedByAgentProvider(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	pairs := [][2]string{
		{"triage", "ollama"},
		{"code-gen", "claude"},
		{"code-gen", "openai"},
	}
	for _, p := range pairs {
		seedCompletedSession(t, s, p[0], p[1], time.Minute)
		if err := s.UpdateTaskProfile(ctx, p[0], p[1]); err != nil {
			t.Fatalf("UpdateTaskProfile(%s,%s): %v", p[0], p[1], err)
		}
	}

	profiles, err := s.ListTaskProfiles(ctx)
	if err != nil {
		t.Fatalf("ListTaskProfiles: %v", err)
	}
	if len(profiles) != 3 {
		t.Fatalf("got %d profiles, want 3", len(profiles))
	}
	// Ordered by agent_type, provider: code-gen/claude, code-gen/openai, triage/ollama.
	want := [][2]string{
		{"code-gen", "claude"},
		{"code-gen", "openai"},
		{"triage", "ollama"},
	}
	for i, p := range profiles {
		if p.AgentType != want[i][0] || p.Provider != want[i][1] {
			t.Errorf("profiles[%d] = (%s,%s), want (%s,%s)", i, p.AgentType, p.Provider, want[i][0], want[i][1])
		}
	}
}

func TestUpdateTaskProfile_FailedSessionsExcluded(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Seed one completed, one failed.
	seedCompletedSession(t, s, "qc", "claude", time.Minute)

	fin := time.Now().UTC()
	start := fin.Add(-5 * time.Minute)
	failed := &models.Session{
		ID:          generateID(),
		IssueNumber: 2,
		AgentType:   "qc",
		Provider:    "claude",
		Model:       "test",
		Status:      models.SessionStatusFailed,
		StartedAt:   start,
		FinishedAt:  &fin,
		CreatedAt:   start,
		UpdatedAt:   fin,
	}
	if err := s.CreateSession(ctx, failed); err != nil {
		t.Fatalf("seed failed session: %v", err)
	}

	if err := s.UpdateTaskProfile(ctx, "qc", "claude"); err != nil {
		t.Fatalf("UpdateTaskProfile: %v", err)
	}

	prof, err := s.GetTaskProfile(ctx, "qc", "claude")
	if err != nil {
		t.Fatalf("GetTaskProfile: %v", err)
	}
	// Only the 1 completed session counts.
	if prof.SampleCount != 1 {
		t.Errorf("SampleCount = %d, want 1 (failed sessions excluded)", prof.SampleCount)
	}
	if prof.AvgDuration != time.Minute {
		t.Errorf("AvgDuration = %v, want 1m", prof.AvgDuration)
	}
}

func TestPercentileDuration(t *testing.T) {
	tests := []struct {
		name   string
		sorted []time.Duration
		p      int
		want   time.Duration
	}{
		{"empty", nil, 50, 0},
		{"single", []time.Duration{5 * time.Second}, 50, 5 * time.Second},
		// idx = (50 * 2) / 100 = 1 → sorted[1] = 20s
		{"p50 of 2", []time.Duration{10 * time.Second, 20 * time.Second}, 50, 20 * time.Second},
		// idx = (90 * 10) / 100 = 9 → sorted[9] = 100s
		{"p90 of 10", func() []time.Duration {
			d := make([]time.Duration, 10)
			for i := range d {
				d[i] = time.Duration(i+1) * 10 * time.Second
			}
			return d
		}(), 90, 100 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := percentileDuration(tc.sorted, tc.p)
			if got != tc.want {
				t.Errorf("percentileDuration(p=%d) = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
}
