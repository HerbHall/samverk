package status

import (
	"context"
	"strings"
	"testing"

	"github.com/herbhall/samverk/internal/forge"
)

// mockTracker implements forge.IssueTracker for testing.
type mockTracker struct {
	issues []*forge.Issue
}

func (m *mockTracker) CreateIssue(_ context.Context, _ *forge.CreateIssueRequest) (*forge.Issue, error) {
	panic("not implemented")
}

func (m *mockTracker) GetIssue(_ context.Context, _ int) (*forge.Issue, error) {
	panic("not implemented")
}

func (m *mockTracker) UpdateIssue(_ context.Context, _ int, _ *forge.UpdateIssueRequest) (*forge.Issue, error) {
	panic("not implemented")
}

func (m *mockTracker) ListIssues(_ context.Context, opts *forge.ListOptions) ([]*forge.Issue, error) {
	result := make([]*forge.Issue, 0, len(m.issues))
	for _, iss := range m.issues {
		if opts != nil && opts.State != "" && iss.State != opts.State {
			continue
		}
		result = append(result, iss)
	}
	return result, nil
}

func (m *mockTracker) AddComment(_ context.Context, _ int, _ string) (*forge.Comment, error) {
	panic("not implemented")
}

func (m *mockTracker) ListComments(_ context.Context, _ int) ([]*forge.Comment, error) {
	panic("not implemented")
}

func (m *mockTracker) SetLabels(_ context.Context, _ int, _ []string) error { panic("not implemented") }
func (m *mockTracker) AddLabel(_ context.Context, _ int, _ string) error    { panic("not implemented") }
func (m *mockTracker) RemoveLabel(_ context.Context, _ int, _ string) error {
	panic("not implemented")
}
func (m *mockTracker) Assign(_ context.Context, _ int, _ string) error   { panic("not implemented") }
func (m *mockTracker) Unassign(_ context.Context, _ int, _ string) error { panic("not implemented") }
func (m *mockTracker) Watch(_ context.Context, _ func(forge.Event)) error {
	panic("not implemented")
}
func (m *mockTracker) SearchIssues(_ context.Context, _ *forge.SearchOptions) ([]*forge.Issue, error) {
	panic("not implemented")
}

func TestGenerate(t *testing.T) {
	tests := []struct {
		name       string
		issues     []*forge.Issue
		healthURL  string
		wantChecks []string // substrings that must appear in output
		noChecks   []string // substrings that must NOT appear in output
	}{
		{
			name:   "no open issues",
			issues: nil,
			wantChecks: []string{
				"Total open issues: 0",
				"No issues in progress.",
				"No queued issues.",
				"No blocked issues.",
				"No issues awaiting QC.",
				"No issues requiring human action.",
				"updated_by: samverk-cli",
			},
		},
		{
			name: "mixed status labels",
			issues: []*forge.Issue{
				{Number: 1, Title: "Active task", State: forge.StateOpen, Labels: []string{"status:in-progress"}},
				{Number: 2, Title: "Claimed work", State: forge.StateOpen, Labels: []string{"status:claimed"}},
				{Number: 3, Title: "Stuck issue", State: forge.StateOpen, Labels: []string{"status:blocked"}},
				{Number: 4, Title: "Human needed", State: forge.StateOpen, Labels: []string{"status:needs-human"}},
				{Number: 5, Title: "Pending human", State: forge.StateOpen, Labels: []string{"status:human-pending"}},
				{Number: 6, Title: "QC check", State: forge.StateOpen, Labels: []string{"status:needs-qc"}},
				{Number: 7, Title: "Backlog item", State: forge.StateOpen, Labels: []string{"priority:medium"}},
			},
			wantChecks: []string{
				"Total open issues: 7",
				"## In Progress (2)",
				"#1: Active task",
				"#2: Claimed work",
				"## Blocked (1)",
				"#3: Stuck issue",
				"## Needs Human (2)",
				"#4: Human needed",
				"#5: Pending human",
				"## Needs QC (1)",
				"#6: QC check",
				"## Queued (1)",
				"#7: Backlog item",
			},
		},
		{
			name:      "health url empty shows unknown",
			issues:    nil,
			healthURL: "",
			wantChecks: []string{
				"unknown",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tracker := &mockTracker{issues: tc.issues}
			w := New(tracker, tc.healthURL)

			got, err := w.Generate(context.Background())
			if err != nil {
				t.Fatalf("Generate() error: %v", err)
			}

			for _, want := range tc.wantChecks {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\n\nFull output:\n%s", want, got)
				}
			}
			for _, noWant := range tc.noChecks {
				if strings.Contains(got, noWant) {
					t.Errorf("output should not contain %q\n\nFull output:\n%s", noWant, got)
				}
			}
		})
	}
}
