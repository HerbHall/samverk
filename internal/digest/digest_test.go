package digest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// mockTracker implements forge.IssueTracker for testing.
type mockTracker struct {
	issues []*forge.Issue
}

func (m *mockTracker) CreateIssue(_ context.Context, _ *forge.CreateIssueRequest) (*forge.Issue, error) {
	return nil, nil
}

func (m *mockTracker) GetIssue(_ context.Context, number int) (*forge.Issue, error) {
	for _, iss := range m.issues {
		if iss.Number == number {
			return iss, nil
		}
	}
	return nil, nil
}

func (m *mockTracker) UpdateIssue(_ context.Context, _ int, _ *forge.UpdateIssueRequest) (*forge.Issue, error) {
	return nil, nil
}

func (m *mockTracker) ListIssues(_ context.Context, opts *forge.ListOptions) ([]*forge.Issue, error) {
	result := make([]*forge.Issue, 0, len(m.issues))
	for _, iss := range m.issues {
		if opts != nil && opts.State != "" && string(iss.State) != string(opts.State) {
			continue
		}
		if opts != nil && len(opts.Labels) > 0 {
			if !hasAllLabels(iss.Labels, opts.Labels) {
				continue
			}
		}
		result = append(result, iss)
	}
	return result, nil
}

func hasAllLabels(issueLabels, required []string) bool {
	set := make(map[string]bool, len(issueLabels))
	for _, l := range issueLabels {
		set[l] = true
	}
	for _, r := range required {
		if !set[r] {
			return false
		}
	}
	return true
}

func (m *mockTracker) AddComment(_ context.Context, _ int, _ string) (*forge.Comment, error) {
	return nil, nil
}

func (m *mockTracker) ListComments(_ context.Context, _ int) ([]*forge.Comment, error) {
	return nil, nil
}

func (m *mockTracker) SetLabels(_ context.Context, _ int, _ []string) error { return nil }
func (m *mockTracker) AddLabel(_ context.Context, _ int, _ string) error    { return nil }
func (m *mockTracker) RemoveLabel(_ context.Context, _ int, _ string) error { return nil }
func (m *mockTracker) Assign(_ context.Context, _ int, _ string) error      { return nil }
func (m *mockTracker) Unassign(_ context.Context, _ int, _ string) error    { return nil }
func (m *mockTracker) Watch(_ context.Context, _ func(forge.Event)) error   { return nil }

func TestBuildDigest(t *testing.T) {
	now := time.Now()
	hourAgo := now.Add(-1 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)
	dayAgo := now.Add(-24 * time.Hour)
	closedAt := now.Add(-30 * time.Minute)

	tests := []struct {
		name            string
		issues          []*forge.Issue
		lastCheckIn     time.Time
		wantPending     int
		wantCompleted   int
		wantActive      int
		wantQueued      int
		wantBlocked     int
		wantFirstTitle  string
	}{
		{
			name:        "empty repo",
			issues:      nil,
			lastCheckIn: dayAgo,
		},
		{
			name: "mixed states",
			issues: []*forge.Issue{
				{
					Number: 1, Title: "Merge PR #52", State: forge.StateOpen,
					Labels: []string{"status:needs-human"}, CreatedAt: twoHoursAgo, UpdatedAt: hourAgo,
					Body: "---\ntype: block\npriority: critical\n---\n\n## Context\n\nQC passed.\n",
				},
				{
					Number: 2, Title: "Add dependency", State: forge.StateOpen,
					Labels: []string{"status:needs-human"}, CreatedAt: hourAgo, UpdatedAt: hourAgo,
					Body: "---\ntype: block\npriority: high\n---\n\n## Context\n\nRequired for Gitea.\n",
				},
				{
					Number: 3, Title: "Completed task", State: forge.StateClosed,
					Labels: []string{}, CreatedAt: twoHoursAgo, UpdatedAt: hourAgo, ClosedAt: &closedAt,
					Body: "---\ntype: task\nactual_tokens: 5000\nmodel_used: claude-sonnet-4\n---\n\n## Result\n\nRefactored router.\n",
				},
				{
					Number: 4, Title: "In progress work", State: forge.StateOpen,
					Labels: []string{"status:in-progress"}, CreatedAt: hourAgo, UpdatedAt: hourAgo,
					Body: "---\nagent_type: code-gen\n---\n",
				},
				{
					Number: 5, Title: "Queued task", State: forge.StateOpen,
					Labels: []string{"status:queued"}, CreatedAt: hourAgo, UpdatedAt: hourAgo,
				},
				{
					Number: 6, Title: "Blocked task", State: forge.StateOpen,
					Labels: []string{"status:blocked"}, CreatedAt: hourAgo, UpdatedAt: hourAgo,
				},
			},
			lastCheckIn:    dayAgo,
			wantPending:    2,
			wantCompleted:  1,
			wantActive:     1,
			wantQueued:     1,
			wantBlocked:    1,
			wantFirstTitle: "Merge PR #52", // critical sorts before high
		},
		{
			name: "priority sorting critical before high",
			issues: []*forge.Issue{
				{
					Number: 10, Title: "Low priority", State: forge.StateOpen,
					Labels: []string{"status:needs-human"}, CreatedAt: twoHoursAgo, UpdatedAt: hourAgo,
					Body: "---\ntype: block\npriority: high\n---\n",
				},
				{
					Number: 11, Title: "High priority", State: forge.StateOpen,
					Labels: []string{"status:needs-human"}, CreatedAt: hourAgo, UpdatedAt: hourAgo,
					Body: "---\ntype: block\npriority: critical\n---\n",
				},
			},
			lastCheckIn:    dayAgo,
			wantPending:    2,
			wantFirstTitle: "High priority",
		},
		{
			name: "closed before last check-in excluded",
			issues: []*forge.Issue{
				{
					Number: 20, Title: "Old closed", State: forge.StateClosed,
					CreatedAt: dayAgo, UpdatedAt: dayAgo, ClosedAt: timePtr(dayAgo.Add(-1 * time.Hour)),
				},
			},
			lastCheckIn:   dayAgo,
			wantCompleted: 0,
		},
		{
			name: "issues without frontmatter",
			issues: []*forge.Issue{
				{
					Number: 30, Title: "Plain issue", State: forge.StateOpen,
					Labels: []string{"status:needs-human"}, CreatedAt: hourAgo, UpdatedAt: hourAgo,
					Body: "Just a plain issue body with no frontmatter.",
				},
			},
			lastCheckIn:    dayAgo,
			wantPending:    1,
			wantFirstTitle: "Plain issue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := &mockTracker{issues: tt.issues}
			d, err := BuildDigest(context.Background(), tracker, nil, tt.lastCheckIn)
			if err != nil {
				t.Fatalf("BuildDigest() error: %v", err)
			}
			if got := len(d.PendingActions); got != tt.wantPending {
				t.Errorf("PendingActions count = %d, want %d", got, tt.wantPending)
			}
			if got := len(d.CompletedActions); got != tt.wantCompleted {
				t.Errorf("CompletedActions count = %d, want %d", got, tt.wantCompleted)
			}
			if got := len(d.ActiveWork); got != tt.wantActive {
				t.Errorf("ActiveWork count = %d, want %d", got, tt.wantActive)
			}
			if d.QueuedCount != tt.wantQueued {
				t.Errorf("QueuedCount = %d, want %d", d.QueuedCount, tt.wantQueued)
			}
			if d.BlockedCount != tt.wantBlocked {
				t.Errorf("BlockedCount = %d, want %d", d.BlockedCount, tt.wantBlocked)
			}
			if tt.wantFirstTitle != "" && len(d.PendingActions) > 0 {
				if d.PendingActions[0].Title != tt.wantFirstTitle {
					t.Errorf("first PendingAction title = %q, want %q", d.PendingActions[0].Title, tt.wantFirstTitle)
				}
			}
		})
	}
}

func TestBuildDigest_FrontmatterParsing(t *testing.T) {
	now := time.Now()
	closedAt := now.Add(-10 * time.Minute)
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{
				Number: 1, Title: "Task with tokens", State: forge.StateClosed,
				CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now, ClosedAt: &closedAt,
				Body: "---\ntype: task\nactual_tokens: 12000\nmodel_used: claude-sonnet-4\n---\n\n## Result\n\nDone.\n",
			},
		},
	}

	d, err := BuildDigest(context.Background(), tracker, nil, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("BuildDigest() error: %v", err)
	}
	if len(d.CompletedActions) != 1 {
		t.Fatalf("expected 1 completed action, got %d", len(d.CompletedActions))
	}
	ca := d.CompletedActions[0]
	if ca.TokensUsed != 12000 {
		t.Errorf("TokensUsed = %d, want 12000", ca.TokensUsed)
	}
	if ca.ModelUsed != "claude-sonnet-4" {
		t.Errorf("ModelUsed = %q, want %q", ca.ModelUsed, "claude-sonnet-4")
	}
	if ca.ResultSummary != "Done." {
		t.Errorf("ResultSummary = %q, want %q", ca.ResultSummary, "Done.")
	}
}

func TestBuildDigest_CountDependents(t *testing.T) {
	now := time.Now()
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{
				Number: 1, Title: "Blocking issue", State: forge.StateOpen,
				Labels: []string{"status:needs-human"}, CreatedAt: now, UpdatedAt: now,
			},
			{
				Number: 2, Title: "Depends on 1", State: forge.StateOpen,
				Labels: []string{"status:blocked"}, CreatedAt: now, UpdatedAt: now,
				Body: "---\ndepends_on:\n  - 1\n---\n",
			},
			{
				Number: 3, Title: "Also depends on 1", State: forge.StateOpen,
				Labels: []string{"status:blocked"}, CreatedAt: now, UpdatedAt: now,
				Body: "---\ndepends_on:\n  - 1\n---\n",
			},
		},
	}

	d, err := BuildDigest(context.Background(), tracker, nil, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("BuildDigest() error: %v", err)
	}
	if len(d.PendingActions) != 1 {
		t.Fatalf("expected 1 pending action, got %d", len(d.PendingActions))
	}
	if d.PendingActions[0].BlockedCount != 2 {
		t.Errorf("BlockedCount = %d, want 2", d.PendingActions[0].BlockedCount)
	}
}

func TestFormatDigest_EmptyRepo(t *testing.T) {
	d := DigestData{LastCheckIn: time.Now().Add(-24 * time.Hour)}
	output := FormatDigest(d)

	if !strings.Contains(output, "No agents have run yet") {
		t.Error("expected first-check-in message for empty digest")
	}
}

func TestFormatDigest_NoPendingActions(t *testing.T) {
	d := DigestData{
		LastCheckIn: time.Now().Add(-8 * time.Hour),
		CompletedActions: []CompletedAction{
			{IssueNumber: 1, Title: "Did something", CompletedAt: time.Now().Add(-1 * time.Hour)},
		},
		QueuedCount: 3,
	}
	output := FormatDigest(d)

	if !strings.Contains(output, "No decisions needed") {
		t.Error("expected no-decisions message")
	}
	if !strings.Contains(output, "COMPLETED AUTONOMOUSLY") {
		t.Error("expected completed section")
	}
	if !strings.Contains(output, "Queued: 3 issues waiting") {
		t.Error("expected queued count")
	}
}

func TestFormatDigest_FullDigest(t *testing.T) {
	now := time.Now()
	d := DigestData{
		LastCheckIn: now.Add(-14 * time.Hour),
		PendingActions: []PendingAction{
			{
				IssueNumber: 52, Title: "Merge PR #52",
				ActionType: "merge_main", Priority: models.PriorityCritical,
				Context:     "QC passed, all tests green.",
				BlockedCount: 2, RequestedAt: now.Add(-6 * time.Hour),
			},
		},
		CompletedActions: []CompletedAction{
			{IssueNumber: 47, Title: "Research webhooks", ActionType: "task",
				CompletedAt: now.Add(-2 * time.Hour)},
		},
		ActiveWork: []ActiveWork{
			{IssueNumber: 50, Title: "Gitea adapter", AgentType: "code-gen",
				ClaimedAt: now.Add(-3 * time.Hour)},
		},
		QueuedCount:  7,
		BlockedCount: 2,
		Cost: CostSummary{
			TokensUsed: 28000, EstimatedCostUSD: 1.42, BudgetRemainingUSD: 48.58,
		},
	}
	output := FormatDigest(d)

	checks := []string{
		"NEEDS YOUR DECISION (1 item, blocking work)",
		"MERGE_MAIN: Merge PR #52",
		"QC passed",
		"Blocks: 2 dependent issues",
		"1 approve | 1r reject | 1? more context",
		"COMPLETED AUTONOMOUSLY",
		"#47",
		"Active: 1 issue in progress",
		"Queued: 7 issues waiting",
		"Blocked: 2 issues",
		"~$1.42",
		"$48.58 remaining",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q", check)
		}
	}
}

func TestExtractSection(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		section string
		want    string
	}{
		{
			name:    "h2 section",
			body:    "# Title\n\n## Context\n\nThis is context.\n\n## Result\n\nDone.",
			section: "Context",
			want:    "This is context.",
		},
		{
			name:    "h3 section",
			body:    "### Context\n\nDeeper heading.\n\n### Other\n\nOther stuff.",
			section: "Context",
			want:    "Deeper heading.",
		},
		{
			name:    "missing section",
			body:    "## Title\n\nNo context here.",
			section: "Context",
			want:    "",
		},
		{
			name:    "case insensitive",
			body:    "## RESULT\n\nAll done.",
			section: "result",
			want:    "All done.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSection(tt.body, tt.section)
			if got != tt.want {
				t.Errorf("extractSection() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPriorityRank(t *testing.T) {
	if priorityRank(models.PriorityCritical) >= priorityRank(models.PriorityHigh) {
		t.Error("critical should rank before high")
	}
	if priorityRank(models.PriorityHigh) >= priorityRank(models.PriorityNormal) {
		t.Error("high should rank before normal")
	}
	if priorityRank(models.PriorityNormal) >= priorityRank(models.PriorityLow) {
		t.Error("normal should rank before low")
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
