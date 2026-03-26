package prwatcher

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
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

type updateCall struct {
	number int
	req    *forge.UpdateIssueRequest
}

type setLabelsCall struct {
	number int
	labels []string
}

type mockIssueTracker struct {
	existingIssues []*forge.Issue
	listErr        error
	createdIssue   *forge.CreateIssueRequest
	createErr      error
	prComments     map[int][]string // PR number -> comments added
	updateCalls    []updateCall
	updateErr      error
	setLabelsCalls []setLabelsCall
	setLabelsErr   error
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
func (m *mockIssueTracker) UpdateIssue(_ context.Context, number int, req *forge.UpdateIssueRequest) (*forge.Issue, error) {
	m.updateCalls = append(m.updateCalls, updateCall{number: number, req: req})
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	return &forge.Issue{Number: number, State: forge.StateClosed}, nil
}
func (m *mockIssueTracker) ListIssues(_ context.Context, _ *forge.ListOptions) ([]*forge.Issue, error) {
	return m.existingIssues, m.listErr
}
func (m *mockIssueTracker) AddComment(_ context.Context, number int, body string) (*forge.Comment, error) {
	if m.prComments == nil {
		m.prComments = make(map[int][]string)
	}
	m.prComments[number] = append(m.prComments[number], body)
	return &forge.Comment{ID: 1, Body: body}, nil
}
func (m *mockIssueTracker) ListComments(context.Context, int) ([]*forge.Comment, error) {
	panic("not called")
}
func (m *mockIssueTracker) SetLabels(_ context.Context, number int, labels []string) error {
	m.setLabelsCalls = append(m.setLabelsCalls, setLabelsCall{number: number, labels: labels})
	return m.setLabelsErr
}
func (m *mockIssueTracker) AddLabel(context.Context, int, string) error    { panic("not called") }
func (m *mockIssueTracker) RemoveLabel(context.Context, int, string) error { panic("not called") }
func (m *mockIssueTracker) Assign(context.Context, int, string) error      { panic("not called") }
func (m *mockIssueTracker) Unassign(context.Context, int, string) error    { panic("not called") }
func (m *mockIssueTracker) Watch(context.Context, func(forge.Event)) error { panic("not called") }
func (m *mockIssueTracker) SearchIssues(context.Context, *forge.SearchOptions) ([]*forge.Issue, error) {
	panic("not called")
}

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
	wantTitle := `fix(#42): address Copilot review feedback on "Add feature X"`
	if it.createdIssue.Title != wantTitle {
		t.Errorf("title = %q, want %q", it.createdIssue.Title, wantTitle)
	}
	wantLabels := map[string]bool{models.LabelAgentCodeGen: true, models.LabelPriorityHigh: true, "pr:42": true}
	for _, l := range it.createdIssue.Labels {
		if !wantLabels[l] {
			t.Errorf("unexpected label %q", l)
		}
		delete(wantLabels, l)
	}
	if len(wantLabels) > 0 {
		t.Errorf("missing labels: %v", wantLabels)
	}
	// Verify PR comment was added.
	if comments, ok := it.prComments[42]; !ok || len(comments) == 0 {
		t.Error("expected a comment on the PR")
	} else if !strings.Contains(comments[0], "Copilot feedback detected") {
		t.Errorf("PR comment = %q, want to contain 'Copilot feedback detected'", comments[0])
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
		Labels: []string{models.LabelStatusNeedsHuman},
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

func TestCheckReviewComments_CopilotAuthorNotInTrustedList(t *testing.T) {
	// Copilot comments should be detected even when not in TrustedReviewers.
	pm := &mockPRManager{
		reviewComments: []forge.ReviewComment{
			{Author: "copilot[bot]", Body: "Consider using a constant here", Path: "main.go", EndLine: 5},
		},
	}
	it := &mockIssueTracker{}

	w := &Watcher{
		prManager:    pm,
		issueTracker: it,
		mergeCfg:     autonomy.MergeConfig{TrustedReviewers: []string{"some-other-reviewer"}},
	}

	pr := &forge.PullRequest{Number: 50, Title: "feat: add caching", Head: "feature/caching", Labels: []string{"auto"}}

	hasBlocking, err := w.checkReviewComments(context.Background(), pr)
	if err != nil {
		t.Fatalf("checkReviewComments: %v", err)
	}
	if !hasBlocking {
		t.Error("expected hasBlocking=true for Copilot-authored comment")
	}
	if it.createdIssue == nil {
		t.Fatal("expected remediation issue to be created")
	}
	if !strings.Contains(it.createdIssue.Title, "Copilot review feedback") {
		t.Errorf("title = %q, want to contain 'Copilot review feedback'", it.createdIssue.Title)
	}
}

func TestCheckReviewComments_LooksGoodCommentMergesNormally(t *testing.T) {
	// Approval-like comments should not block merge.
	pm := &mockPRManager{
		reviewComments: []forge.ReviewComment{
			{Author: "copilot[bot]", Body: "Looks good to me! No issues found.", Path: "main.go", EndLine: 1},
		},
	}
	it := &mockIssueTracker{}

	w := &Watcher{
		prManager:    pm,
		issueTracker: it,
		mergeCfg:     autonomy.MergeConfig{TrustedReviewers: []string{"copilot[bot]"}},
	}

	pr := &forge.PullRequest{Number: 60, Title: "docs: update readme", Labels: []string{"auto"}}

	hasBlocking, err := w.checkReviewComments(context.Background(), pr)
	if err != nil {
		t.Fatalf("checkReviewComments: %v", err)
	}
	if hasBlocking {
		t.Error("expected hasBlocking=false for approval-like comment")
	}
	if it.createdIssue != nil {
		t.Error("should not create remediation issue for approval comment")
	}
}

func TestIsCopilotAuthor(t *testing.T) {
	tests := []struct {
		author string
		want   bool
	}{
		{"copilot[bot]", true},
		{"copilot", true},
		{"Copilot", true},
		{"github-copilot[bot]", true},
		{"random-user", false},
		{"github-actions[bot]", false},
		{"bot", false},
	}
	for _, tt := range tests {
		t.Run(tt.author, func(t *testing.T) {
			if got := isCopilotAuthor(tt.author); got != tt.want {
				t.Errorf("isCopilotAuthor(%q) = %v, want %v", tt.author, got, tt.want)
			}
		})
	}
}

func TestParseLinkedIssues(t *testing.T) {
	tests := []struct {
		name string
		pr   *forge.PullRequest
		want []int
	}{
		{
			name: "title with (#N)",
			pr:   &forge.PullRequest{Title: "fix(auth): add WWW-Authenticate header (#123)"},
			want: []int{123},
		},
		{
			name: "body with Closes and Fixes",
			pr:   &forge.PullRequest{Body: "Closes #42\nFixes #43"},
			want: []int{42, 43},
		},
		{
			name: "body with Resolves",
			pr:   &forge.PullRequest{Body: "Resolves #99"},
			want: []int{99},
		},
		{
			name: "body lowercase closes",
			pr:   &forge.PullRequest{Body: "closes #5"},
			want: []int{5},
		},
		{
			name: "no linked issues",
			pr:   &forge.PullRequest{Title: "chore: update deps", Body: "Routine update."},
			want: []int{},
		},
		{
			name: "duplicate references",
			pr:   &forge.PullRequest{Body: "Closes #42\nCloses #42"},
			want: []int{42},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLinkedIssues(tt.pr)
			if len(got) != len(tt.want) {
				t.Fatalf("parseLinkedIssues() = %v, want %v", got, tt.want)
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("parseLinkedIssues()[%d] = %d, want %d", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestCloseLinkedIssues(t *testing.T) {
	it := &mockIssueTracker{}
	w := &Watcher{issueTracker: it}

	pr := &forge.PullRequest{
		Number: 55,
		Title:  "feat: implement thing",
		Body:   "Closes #10",
	}

	w.closeLinkedIssues(context.Background(), pr)

	// Assert UpdateIssue was called with StateClosed.
	if len(it.updateCalls) != 1 {
		t.Fatalf("expected 1 UpdateIssue call, got %d", len(it.updateCalls))
	}
	if it.updateCalls[0].number != 10 {
		t.Errorf("UpdateIssue issue number = %d, want 10", it.updateCalls[0].number)
	}
	if it.updateCalls[0].req.State == nil || *it.updateCalls[0].req.State != forge.StateClosed {
		t.Errorf("UpdateIssue state = %v, want StateClosed", it.updateCalls[0].req.State)
	}

	// Assert SetLabels was called with status:done.
	if len(it.setLabelsCalls) != 1 {
		t.Fatalf("expected 1 SetLabels call, got %d", len(it.setLabelsCalls))
	}
	if it.setLabelsCalls[0].number != 10 {
		t.Errorf("SetLabels issue number = %d, want 10", it.setLabelsCalls[0].number)
	}
	if len(it.setLabelsCalls[0].labels) != 1 || it.setLabelsCalls[0].labels[0] != models.LabelStatusDone {
		t.Errorf("SetLabels labels = %v, want [status:done]", it.setLabelsCalls[0].labels)
	}
}

func TestCloseLinkedIssues_UpdateError_SkipsSetLabels(t *testing.T) {
	it := &mockIssueTracker{updateErr: fmt.Errorf("api error")}
	w := &Watcher{issueTracker: it}

	pr := &forge.PullRequest{Number: 1, Body: "Closes #7"}
	w.closeLinkedIssues(context.Background(), pr)

	if len(it.updateCalls) != 1 {
		t.Fatalf("expected 1 UpdateIssue call, got %d", len(it.updateCalls))
	}
	if len(it.setLabelsCalls) != 0 {
		t.Errorf("SetLabels should not be called when UpdateIssue fails")
	}
}

func TestIsActionableComment(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"actionable suggestion", "Consider using a constant here instead of a magic number", true},
		{"security warning", "This function is vulnerable to SQL injection", true},
		{"bug report", "Missing error check on line 42", true},
		{"looks good", "Looks good to me!", false},
		{"lgtm", "LGTM", false},
		{"no issues", "No issues found in this change", false},
		{"approved", "Approved -- ship it", false},
		{"empty body", "", false},
		{"whitespace only", "   ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isActionableComment(tt.body); got != tt.want {
				t.Errorf("isActionableComment(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}
