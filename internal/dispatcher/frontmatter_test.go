package dispatcher

import (
	"context"
	"strings"
	"testing"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

func TestDerivePriorityFromLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   models.Priority
	}{
		{
			name:   "no labels",
			labels: nil,
			want:   models.PriorityNormal,
		},
		{
			name:   "empty labels",
			labels: []string{},
			want:   models.PriorityNormal,
		},
		{
			name:   "critical",
			labels: []string{models.LabelPriorityCritical},
			want:   models.PriorityCritical,
		},
		{
			name:   "high",
			labels: []string{models.LabelPriorityHigh},
			want:   models.PriorityHigh,
		},
		{
			name:   "low",
			labels: []string{models.LabelPriorityLow},
			want:   models.PriorityLow,
		},
		{
			name:   "unrelated labels return normal",
			labels: []string{"bug", "enhancement", models.LabelStatusQueued},
			want:   models.PriorityNormal,
		},
		{
			name:   "first priority label wins",
			labels: []string{models.LabelPriorityCritical, models.LabelPriorityLow},
			want:   models.PriorityCritical,
		},
		{
			name:   "priority among other labels",
			labels: []string{"bug", models.LabelPriorityHigh, models.LabelStatusQueued},
			want:   models.PriorityHigh,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := derivePriorityFromLabels(tt.labels)
			if got != tt.want {
				t.Errorf("derivePriorityFromLabels(%v) = %q, want %q", tt.labels, got, tt.want)
			}
		})
	}
}

func TestAutoInjectFrontmatter_PersistsBody(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	issue := &forge.Issue{
		Number: 42,
		Title:  "fix: broken thing",
		Body:   "Plain issue body without frontmatter.",
		State:  forge.StateOpen,
		Labels: []string{"bug", models.LabelPriorityHigh},
	}
	tracker.issues[42] = issue

	fm := d.autoInjectFrontmatter(context.Background(), "testowner", "testrepo", issue, models.AgentTypeCodeGen)

	if fm == nil {
		t.Fatal("expected non-nil frontmatter")
	}
	if fm.SchemaVersion != models.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", fm.SchemaVersion, models.SchemaVersion)
	}
	if fm.Type != models.IssueTypeTask {
		t.Errorf("Type = %q, want %q", fm.Type, models.IssueTypeTask)
	}
	if fm.AgentType != models.AgentTypeCodeGen {
		t.Errorf("AgentType = %q, want %q", fm.AgentType, models.AgentTypeCodeGen)
	}
	if fm.Priority != models.PriorityHigh {
		t.Errorf("Priority = %q, want %q", fm.Priority, models.PriorityHigh)
	}

	// Verify the issue body was updated on the tracker.
	updated := tracker.issues[42]
	if !strings.HasPrefix(updated.Body, "---\n") {
		t.Errorf("expected body to start with frontmatter delimiter, got: %s", updated.Body[:min(50, len(updated.Body))])
	}
	if !strings.Contains(updated.Body, "agent_type: code-gen") {
		t.Error("expected body to contain agent_type: code-gen")
	}
	if !strings.Contains(updated.Body, "Plain issue body without frontmatter.") {
		t.Error("expected original body to be preserved after frontmatter")
	}
}

func TestAutoInjectFrontmatter_RoundTrips(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	issue := &forge.Issue{
		Number: 55,
		Title:  "feat: new feature",
		Body:   "## Summary\n\nSome feature description.",
		State:  forge.StateOpen,
		Labels: []string{models.LabelPriorityCritical},
	}
	tracker.issues[55] = issue

	d.autoInjectFrontmatter(context.Background(), "testowner", "testrepo", issue, models.AgentTypeCodeGen)

	// Re-parse the persisted body to verify it round-trips.
	result, err := models.ParseFrontmatter(tracker.issues[55].Body)
	if err != nil {
		t.Fatalf("ParseFrontmatter failed on injected body: %v", err)
	}
	if result.Frontmatter == nil {
		t.Fatal("expected frontmatter after round-trip parse")
	}
	if result.Frontmatter.AgentType != models.AgentTypeCodeGen {
		t.Errorf("round-trip AgentType = %q, want %q", result.Frontmatter.AgentType, models.AgentTypeCodeGen)
	}
	if result.Frontmatter.Priority != models.PriorityCritical {
		t.Errorf("round-trip Priority = %q, want %q", result.Frontmatter.Priority, models.PriorityCritical)
	}
	if !strings.Contains(result.Body, "## Summary") {
		t.Error("expected markdown body preserved after round-trip")
	}
}

func TestHandleOpened_HeuristicInjectsFrontmatter(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	issue := &forge.Issue{
		Number: 100,
		Title:  "fix: something broken",
		Body:   "No frontmatter, just a description of the bug.",
		State:  forge.StateOpen,
		Labels: []string{"bug"},
	}
	tracker.issues[100] = issue

	err := d.handleOpened(context.Background(), forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 100,
		Issue:       issue,
		Owner:       "testowner",
		Repo:        "testrepo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be routed (not escalated).
	if !hasLabel(issue.Labels, models.LabelStatusClaimed) {
		t.Error("expected status:claimed after heuristic routing")
	}

	// Body should have frontmatter injected.
	updated := tracker.issues[100]
	if !strings.HasPrefix(updated.Body, "---\n") {
		t.Error("expected frontmatter to be injected into issue body")
	}

	// Verify the injected frontmatter is parseable.
	result, parseErr := models.ParseFrontmatter(updated.Body)
	if parseErr != nil {
		t.Fatalf("ParseFrontmatter on injected body: %v", parseErr)
	}
	if result.Frontmatter == nil {
		t.Fatal("expected non-nil frontmatter after injection")
	}
	if result.Frontmatter.AgentType != models.AgentTypeCodeGen {
		t.Errorf("injected AgentType = %q, want %q", result.Frontmatter.AgentType, models.AgentTypeCodeGen)
	}
}
