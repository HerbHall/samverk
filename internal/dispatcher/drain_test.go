package dispatcher

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

func TestDrainPreventsNewClaims(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Add a queued issue.
	tracker.mu.Lock()
	tracker.issues[1] = &forge.Issue{
		Number:    1,
		Title:     "Test issue",
		Body:      issueBody("code-gen", nil),
		State:     forge.StateOpen,
		Labels:    []string{models.LabelStatusQueued},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	tracker.mu.Unlock()

	// Enter drain mode.
	status := d.Drain()
	if !status.Draining {
		t.Fatal("expected draining to be true after Drain()")
	}

	// pollQueued should skip due to drain mode.
	ctx := context.Background()
	d.pollQueued(ctx)

	// Issue should NOT have been claimed.
	d.mu.RLock()
	claimedCount := len(d.claimed)
	d.mu.RUnlock()
	if claimedCount != 0 {
		t.Errorf("expected 0 claimed issues in drain mode, got %d", claimedCount)
	}
}

func TestDrainPreventsHandleOpened(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.mu.Lock()
	tracker.issues[1] = &forge.Issue{
		Number:    1,
		Title:     "Test issue",
		Body:      issueBody("code-gen", nil),
		State:     forge.StateOpen,
		Labels:    []string{models.LabelStatusQueued},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	tracker.mu.Unlock()

	d.Drain()

	ev := forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 1,
		Owner:       "test",
		Repo:        "repo",
		Issue: &forge.Issue{
			Number: 1,
			Title:  "Test issue",
			Body:   issueBody("code-gen", nil),
			State:  forge.StateOpen,
			Labels: []string{models.LabelStatusQueued},
		},
	}
	err := d.handleOpened(context.Background(), ev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Issue should NOT have been routed.
	d.mu.RLock()
	claimedCount := len(d.claimed)
	d.mu.RUnlock()
	if claimedCount != 0 {
		t.Errorf("expected 0 claimed issues in drain mode, got %d", claimedCount)
	}
}

func TestDrainPreventsHandleLabeled(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.mu.Lock()
	tracker.issues[1] = &forge.Issue{
		Number:    1,
		Title:     "Test issue",
		Body:      issueBody("code-gen", nil),
		State:     forge.StateOpen,
		Labels:    []string{models.LabelStatusQueued},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	tracker.mu.Unlock()

	d.Drain()

	ev := forge.Event{
		Type:        forge.EventIssueLabeled,
		IssueNumber: 1,
		Label:       models.LabelStatusQueued,
		Owner:       "test",
		Repo:        "repo",
	}
	err := d.handleLabeled(context.Background(), ev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	d.mu.RLock()
	claimedCount := len(d.claimed)
	d.mu.RUnlock()
	if claimedCount != 0 {
		t.Errorf("expected 0 claimed issues in drain mode, got %d", claimedCount)
	}
}

func TestDrainStatusReturnsLiveCount(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Simulate claimed issues by populating the claimed map directly.
	d.mu.Lock()
	d.claimed[issueKey("test", "repo", 42)] = &claimedIssue{
		AgentID:   "code-gen",
		Owner:     "test",
		Repo:      "repo",
		ClaimedAt: time.Now(),
	}
	d.claimed[issueKey("test", "repo", 99)] = &claimedIssue{
		AgentID:   "test",
		Owner:     "test",
		Repo:      "repo",
		ClaimedAt: time.Now(),
	}
	d.mu.Unlock()

	status := d.DrainState()
	if status.Draining {
		t.Error("expected draining=false before Drain()")
	}
	if len(status.ClaimedIssues) != 2 {
		t.Errorf("expected 2 claimed issues, got %d", len(status.ClaimedIssues))
	}

	// Verify the issue numbers are present.
	issueSet := make(map[int]bool)
	for _, num := range status.ClaimedIssues {
		issueSet[num] = true
	}
	if !issueSet[42] || !issueSet[99] {
		t.Errorf("expected issues 42 and 99, got %v", status.ClaimedIssues)
	}
}

func TestDrainClearedOnStartup(t *testing.T) {
	// Verify that a new dispatcher starts with draining=false.
	d := New([]TrackerEntry{{Owner: "test", Repo: "repo", Tracker: newMockTracker()}},
		&mockPolicy{costThreshold: 5.0}, nil, nil, nil, zap.NewNop())
	if d.IsDraining() {
		t.Error("expected new dispatcher to start with draining=false")
	}
}

func TestDrainAndCancelDrain(t *testing.T) {
	d := newTestDispatcher(newMockTracker())

	if d.IsDraining() {
		t.Error("expected draining=false initially")
	}

	d.Drain()
	if !d.IsDraining() {
		t.Error("expected draining=true after Drain()")
	}

	d.CancelDrain()
	if d.IsDraining() {
		t.Error("expected draining=false after CancelDrain()")
	}
}
