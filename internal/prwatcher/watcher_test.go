package prwatcher

import (
	"context"
	"testing"

	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/forge"
)

func TestIsEligible(t *testing.T) {
	baseCfg := autonomy.MergeConfig{
		AutoMergeOnCIPass: true,
		TrustedAuthors:    []string{"bot", "agent"},
		ExcludeLabels:     []string{"do-not-merge", "wip"},
	}

	tests := []struct {
		name string
		pr   *forge.PullRequest
		want bool
	}{
		{
			name: "eligible PR",
			pr: &forge.PullRequest{
				Number:    1,
				Author:    "bot",
				Draft:     false,
				Mergeable: true,
				Labels:    []string{"auto"},
			},
			want: true,
		},
		{
			name: "draft PR",
			pr: &forge.PullRequest{
				Number:    2,
				Author:    "bot",
				Draft:     true,
				Mergeable: true,
			},
			want: false,
		},
		{
			name: "not mergeable",
			pr: &forge.PullRequest{
				Number:    3,
				Author:    "bot",
				Draft:     false,
				Mergeable: false,
			},
			want: false,
		},
		{
			name: "untrusted author",
			pr: &forge.PullRequest{
				Number:    4,
				Author:    "stranger",
				Draft:     false,
				Mergeable: true,
			},
			want: false,
		},
		{
			name: "excluded label",
			pr: &forge.PullRequest{
				Number:    5,
				Author:    "agent",
				Draft:     false,
				Mergeable: true,
				Labels:    []string{"approved", "do-not-merge"},
			},
			want: false,
		},
	}

	w := &Watcher{mergeCfg: baseCfg}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := w.isEligible(tt.pr)
			if got != tt.want {
				t.Errorf("isEligible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTrustedAuthor_EmptyList(t *testing.T) {
	w := &Watcher{mergeCfg: autonomy.MergeConfig{TrustedAuthors: nil}}
	if !w.isTrustedAuthor("anyone") {
		t.Error("empty trusted list should trust all authors")
	}
}

func TestAllChecksPassed(t *testing.T) {
	w := &Watcher{}

	tests := []struct {
		name   string
		checks []forge.Check
		want   bool
	}{
		{
			name:   "empty checks - not passed",
			checks: nil,
			want:   false,
		},
		{
			name: "all success",
			checks: []forge.Check{
				{Name: "ci/build", Status: forge.CheckStatusSuccess},
				{Name: "ci/test", Status: forge.CheckStatusSuccess},
			},
			want: true,
		},
		{
			name: "one pending",
			checks: []forge.Check{
				{Name: "ci/build", Status: forge.CheckStatusSuccess},
				{Name: "ci/test", Status: forge.CheckStatusPending},
			},
			want: false,
		},
		{
			name: "one failure",
			checks: []forge.Check{
				{Name: "ci/build", Status: forge.CheckStatusFailure},
				{Name: "ci/test", Status: forge.CheckStatusSuccess},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := w.allChecksPassed(tt.checks)
			if got != tt.want {
				t.Errorf("allChecksPassed() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Mock types for remediation tests ---

type mockPRManager struct {
	reviewComments []forge.ReviewComment
	reviewErr      error
}

func (m *mockPRManager) CreatePullRequest(context.Context, *forge.CreatePRRequest) (*forge.PullRequest, error) {
	panic("not called")
}
func (m *mockPRManager) GetPullRequest(context.Context, int) (*forge.PullRequest, error) {
	panic("not called")
}
func (m *mockPRManager) ListPullRequests(context.Context, *forge.ListPROptions) ([]*forge.PullRequest, error) {
	panic("not called")
}
func (m *mockPRManager) MergePullRequest(context.Context, int, forge.MergeMethod, string) error {
	panic("not called")
}
func (m *mockPRManager) GetPRChecks(context.Context, int) ([]forge.Check, error) {
	panic("not called")
}
func (m *mockPRManager) ListReviewComments(_ context.Context, _ int) ([]forge.ReviewComment, error) {
	return m.reviewComments, m.reviewErr
}

type mockIssueTracker struct {
	existingIssues []*forge.Issue
	listErr        error
	createdIssue   *forge.CreateIssueRequest
	createErr      error
}

func (m *mockIssueTracker) CreateIssue(_ context.Context, req *forge.CreateIssueRequest) (*forge.Issue, error) {
	m.createdIssue = req
	if m.createErr != nil {
		return nil, m.createErr
	}
	return &forge.Issue{Number: 100, Title: req.Title}, nil
}
func (m *mockIssueTracker) GetIssue(context.Context, int) (*forge.Issue, error) {
	panic("not called")
}
func (m *mockIssueTracker) UpdateIssue(context.Context, int, *forge.UpdateIssueRequest) (*forge.Issue, error) {
	panic("not called")
}
func (m *mockIssueTracker) ListIssues(_ context.Context, _ *forge.ListOptions) ([]*forge.Issue, error) {
	return m.existingIssues, m.listErr
}
func (m *mockIssueTracker) AddComment(context.Context, int, string) (*forge.Comment, error) {
	panic("not called")
}
func (m *mockIssueTracker) ListComments(context.Context, int) ([]*forge.Comment, error) {
	panic("not called")
}
func (m *mockIssueTracker) SetLabels(context.Context, int, []string) error  { panic("not called") }
func (m *mockIssueTracker) AddLabel(context.Context, int, string) error     { panic("not called") }
func (m *mockIssueTracker) RemoveLabel(context.Context, int, string) error  { panic("not called") }
func (m *mockIssueTracker) Assign(context.Context, int, string) error       { panic("not called") }
func (m *mockIssueTracker) Unassign(context.Context, int, string) error     { panic("not called") }
func (m *mockIssueTracker) Watch(context.Context, func(forge.Event)) error  { panic("not called") }

func TestCheckReviewComments_CreatesRemediationIssue(t *testing.T) {
	pm := &mockPRManager{
		reviewComments: []forge.ReviewComment{
			{Author: "copilot", Body: "Use constants here", Path: "main.go", StartLine: 10, EndLine: 10},
			{Author: "copilot", Body: "Missing error check", Path: "handler.go", StartLine: 5, EndLine: 8},
		},
	}
	it := &mockIssueTracker{}

	w := &Watcher{
		prManager:    pm,
		issueTracker: it,
		mergeCfg: autonomy.MergeConfig{
			TrustedReviewers: []string{"copilot"},
		},
	}

	pr := &forge.PullRequest{
		Number: 42,
		Title:  "Add feature X",
		Head:   "feature/x",
		Labels: []string{"auto"},
	}

	hasBlocking, err := w.checkReviewComments(context.Background(), pr)
	if err != nil {
		t.Fatalf("checkReviewComments: %v", err)
	}
	if !hasBlocking {
		t.Error("expected hasBlocking=true")
	}
	if it.createdIssue == nil {
		t.Fatal("expected issue to be created")
	}
	wantTitle := `fix(#42): address review comments on "Add feature X"`
	if it.createdIssue.Title != wantTitle {
		t.Errorf("title = %q, want %q", it.createdIssue.Title, wantTitle)
	}
	wantLabels := map[string]bool{"status:queued": true, "agent:code-gen": true, "pr:42": true}
	for _, l := range it.createdIssue.Labels {
		if !wantLabels[l] {
			t.Errorf("unexpected label %q", l)
		}
		delete(wantLabels, l)
	}
	if len(wantLabels) > 0 {
		t.Errorf("missing labels: %v", wantLabels)
	}
}

func TestCheckReviewComments_SkipsNeedsHumanLabel(t *testing.T) {
	pm := &mockPRManager{
		reviewComments: []forge.ReviewComment{
			{Author: "copilot", Body: "Fix this"},
		},
	}
	it := &mockIssueTracker{}

	w := &Watcher{
		prManager:    pm,
		issueTracker: it,
		mergeCfg:     autonomy.MergeConfig{TrustedReviewers: []string{"copilot"}},
	}

	pr := &forge.PullRequest{
		Number: 10,
		Title:  "Some PR",
		Labels: []string{"status:needs-human"},
	}

	hasBlocking, err := w.checkReviewComments(context.Background(), pr)
	if err != nil {
		t.Fatalf("checkReviewComments: %v", err)
	}
	if hasBlocking {
		t.Error("expected hasBlocking=false for status:needs-human PR")
	}
	if it.createdIssue != nil {
		t.Error("should not create issue for needs-human PR")
	}
}

func TestCheckReviewComments_SkipsDuplicateIssue(t *testing.T) {
	pm := &mockPRManager{
		reviewComments: []forge.ReviewComment{
			{Author: "copilot", Body: "Fix this", Path: "main.go", EndLine: 1},
		},
	}
	it := &mockIssueTracker{
		existingIssues: []*forge.Issue{
			{Number: 99, Title: "fix(#42): existing remediation"},
		},
	}

	w := &Watcher{
		prManager:    pm,
		issueTracker: it,
		mergeCfg:     autonomy.MergeConfig{TrustedReviewers: []string{"copilot"}},
	}

	pr := &forge.PullRequest{Number: 42, Title: "Some PR", Labels: []string{"auto"}}

	hasBlocking, err := w.checkReviewComments(context.Background(), pr)
	if err != nil {
		t.Fatalf("checkReviewComments: %v", err)
	}
	if !hasBlocking {
		t.Error("expected hasBlocking=true (existing issue)")
	}
	if it.createdIssue != nil {
		t.Error("should not create duplicate issue")
	}
}

func TestCheckReviewComments_NoTrustedReviewerComments(t *testing.T) {
	pm := &mockPRManager{
		reviewComments: []forge.ReviewComment{
			{Author: "random-user", Body: "Nice code!", Path: "main.go"},
		},
	}
	it := &mockIssueTracker{}

	w := &Watcher{
		prManager:    pm,
		issueTracker: it,
		mergeCfg:     autonomy.MergeConfig{TrustedReviewers: []string{"copilot"}},
	}

	pr := &forge.PullRequest{Number: 42, Labels: []string{"auto"}}

	hasBlocking, err := w.checkReviewComments(context.Background(), pr)
	if err != nil {
		t.Fatalf("checkReviewComments: %v", err)
	}
	if hasBlocking {
		t.Error("expected hasBlocking=false")
	}
	if it.createdIssue != nil {
		t.Error("should not create issue when no trusted reviewer comments")
	}
}

func TestCheckReviewComments_SkipsResolvedComments(t *testing.T) {
	pm := &mockPRManager{
		reviewComments: []forge.ReviewComment{
			{Author: "copilot", Body: "Old feedback", Path: "main.go", Resolved: true},
		},
	}
	it := &mockIssueTracker{}

	w := &Watcher{
		prManager:    pm,
		issueTracker: it,
		mergeCfg:     autonomy.MergeConfig{TrustedReviewers: []string{"copilot"}},
	}

	pr := &forge.PullRequest{Number: 42, Labels: []string{"auto"}}

	hasBlocking, err := w.checkReviewComments(context.Background(), pr)
	if err != nil {
		t.Fatalf("checkReviewComments: %v", err)
	}
	if hasBlocking {
		t.Error("expected hasBlocking=false (all comments resolved)")
	}
}

func TestIsTrustedReviewer_EmptyList(t *testing.T) {
	w := &Watcher{mergeCfg: autonomy.MergeConfig{TrustedReviewers: nil}}
	if w.isTrustedReviewer("anyone") {
		t.Error("empty trusted reviewers list should not trust anyone")
	}
}
