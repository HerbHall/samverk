package dispatcher

import (
	"context"
	"fmt"
	"testing"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/agent"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// mockForgeAll implements IssueTracker + RepoWriter + PullRequestManager for testing.
type mockForgeAll struct {
	issues   map[int]*forge.Issue
	comments map[int][]*forge.Comment
	prs      []*forge.PullRequest
	branches []string
	files    map[string]string // branch:path -> content

	// Tracking
	labelsAdded   map[int][]string
	labelsRemoved map[int][]string
	commentsAdded map[int][]string
	prCreated     *forge.CreatePRRequest
	createPRErr   error
	createBrErr   error
	writeFileErr  error
}

func newMockForgeAll() *mockForgeAll {
	return &mockForgeAll{
		issues:        make(map[int]*forge.Issue),
		comments:      make(map[int][]*forge.Comment),
		files:         make(map[string]string),
		labelsAdded:   make(map[int][]string),
		labelsRemoved: make(map[int][]string),
		commentsAdded: make(map[int][]string),
	}
}

// IssueTracker methods.
func (m *mockForgeAll) CreateIssue(_ context.Context, _ *forge.CreateIssueRequest) (*forge.Issue, error) {
	return nil, nil
}
func (m *mockForgeAll) GetIssue(_ context.Context, number int) (*forge.Issue, error) {
	if i, ok := m.issues[number]; ok {
		return i, nil
	}
	return nil, fmt.Errorf("issue %d not found", number)
}
func (m *mockForgeAll) UpdateIssue(_ context.Context, _ int, _ *forge.UpdateIssueRequest) (*forge.Issue, error) {
	return nil, nil
}
func (m *mockForgeAll) ListIssues(_ context.Context, _ *forge.ListOptions) ([]*forge.Issue, error) {
	result := make([]*forge.Issue, 0, len(m.issues))
	for _, i := range m.issues {
		result = append(result, i)
	}
	return result, nil
}
func (m *mockForgeAll) SearchIssues(_ context.Context, _ *forge.SearchOptions) ([]*forge.Issue, error) {
	return nil, nil
}
func (m *mockForgeAll) AddComment(_ context.Context, number int, body string) (*forge.Comment, error) {
	m.commentsAdded[number] = append(m.commentsAdded[number], body)
	return &forge.Comment{Body: body}, nil
}
func (m *mockForgeAll) ListComments(_ context.Context, number int) ([]*forge.Comment, error) {
	return m.comments[number], nil
}
func (m *mockForgeAll) SetLabels(_ context.Context, _ int, _ []string) error { return nil }
func (m *mockForgeAll) AddLabel(_ context.Context, number int, label string) error {
	m.labelsAdded[number] = append(m.labelsAdded[number], label)
	return nil
}
func (m *mockForgeAll) RemoveLabel(_ context.Context, number int, label string) error {
	m.labelsRemoved[number] = append(m.labelsRemoved[number], label)
	return nil
}
func (m *mockForgeAll) Assign(_ context.Context, _ int, _ string) error   { return nil }
func (m *mockForgeAll) Unassign(_ context.Context, _ int, _ string) error { return nil }
func (m *mockForgeAll) Watch(_ context.Context, _ func(forge.Event)) error {
	select {}
}

// RepoWriter methods.
func (m *mockForgeAll) CreateBranch(_ context.Context, name string) error {
	if m.createBrErr != nil {
		return m.createBrErr
	}
	m.branches = append(m.branches, name)
	return nil
}
func (m *mockForgeAll) CreateOrUpdateFile(_ context.Context, branch, path, content, _ string) error {
	if m.writeFileErr != nil {
		return m.writeFileErr
	}
	m.files[branch+":"+path] = content
	return nil
}

// PullRequestManager methods.
func (m *mockForgeAll) CreatePullRequest(_ context.Context, req *forge.CreatePRRequest) (*forge.PullRequest, error) {
	if m.createPRErr != nil {
		return nil, m.createPRErr
	}
	m.prCreated = req
	pr := &forge.PullRequest{Number: 999, Title: req.Title, Head: req.Head, State: forge.StateOpen}
	m.prs = append(m.prs, pr)
	return pr, nil
}
func (m *mockForgeAll) GetPullRequest(_ context.Context, _ int) (*forge.PullRequest, error) {
	return nil, nil
}
func (m *mockForgeAll) ListPullRequests(_ context.Context, _ *forge.ListPROptions) ([]*forge.PullRequest, error) {
	return m.prs, nil
}
func (m *mockForgeAll) MergePullRequest(_ context.Context, _ int, _ forge.MergeMethod, _ string) error {
	return nil
}
func (m *mockForgeAll) GetPRChecks(_ context.Context, _ int) ([]forge.Check, error) {
	return nil, nil
}
func (m *mockForgeAll) ListReviewComments(_ context.Context, _ int) ([]forge.ReviewComment, error) {
	return nil, nil
}

func newTestPool(mockForge *mockForgeAll) *agent.Pool {
	logger := zap.NewNop()
	pool := agent.NewPool(nil, mockForge, nil, nil, 0, logger)
	pool.SetRepoWriter(mockForge)
	pool.SetPRManager(mockForge)
	return pool
}

func newTestDispatcherWithPool(tracker *mockForgeAll) *Dispatcher {
	pool := newTestPool(tracker)
	return &Dispatcher{
		trackers: map[string]forge.IssueTracker{
			"samverk/samverk": tracker,
		},
		trackerEntries: []TrackerEntry{
			{Owner: "samverk", Repo: "samverk", Tracker: tracker},
		},
		pool:          pool,
		claimed:       make(map[string]*claimedIssue),
		issueFailures: make(map[string]int),
		logger:        zap.NewNop(),
	}
}

func TestTryApplyEdits_Success(t *testing.T) {
	mock := newMockForgeAll()
	mock.issues[10] = &forge.Issue{
		Number: 10,
		Title:  "feat: add widget",
		Labels: []string{"agent:code-gen", "status:claimed"},
	}
	mock.comments[10] = []*forge.Comment{
		{Body: "EDIT internal/widget.go\npackage widget\nEND\n\nPR_TITLE: feat: add widget"},
	}

	d := newTestDispatcherWithPool(mock)
	result := agent.TaskResult{
		IssueNumber: 10,
		Owner:       "samverk",
		Repo:        "samverk",
		AgentType:   models.AgentTypeCodeGen,
		Success:     true,
	}

	got := d.tryApplyEdits(context.Background(), result, mock)
	if !got {
		t.Fatal("expected tryApplyEdits to return true")
	}
	if mock.prCreated == nil {
		t.Fatal("expected PR to be created")
	}
	if mock.prCreated.Title != "feat: add widget" {
		t.Errorf("PR title = %q, want %q", mock.prCreated.Title, "feat: add widget")
	}
	if len(mock.branches) != 1 {
		t.Errorf("branches created = %d, want 1", len(mock.branches))
	}
	if _, ok := mock.files[mock.branches[0]+":internal/widget.go"]; !ok {
		t.Error("expected file to be written to branch")
	}
}

func TestTryApplyEdits_NonCodeAgent(t *testing.T) {
	mock := newMockForgeAll()
	d := newTestDispatcherWithPool(mock)
	result := agent.TaskResult{
		IssueNumber: 10,
		AgentType:   models.AgentTypeDocs,
		Success:     true,
	}

	got := d.tryApplyEdits(context.Background(), result, mock)
	if got {
		t.Error("expected tryApplyEdits to return false for docs agent")
	}
}

func TestTryApplyEdits_NoEditBlocks(t *testing.T) {
	mock := newMockForgeAll()
	mock.issues[10] = &forge.Issue{Number: 10, Title: "fix: typo"}
	mock.comments[10] = []*forge.Comment{
		{Body: "I reviewed the code and found no issues."},
	}

	d := newTestDispatcherWithPool(mock)
	result := agent.TaskResult{
		IssueNumber: 10,
		AgentType:   models.AgentTypeCodeGen,
		Success:     true,
	}

	got := d.tryApplyEdits(context.Background(), result, mock)
	if got {
		t.Error("expected tryApplyEdits to return false when no EDIT blocks")
	}
}

func TestTryApplyEdits_PRAlreadyExists(t *testing.T) {
	mock := newMockForgeAll()
	mock.issues[10] = &forge.Issue{Number: 10, Title: "feat: add widget"}
	mock.comments[10] = []*forge.Comment{
		{Body: "EDIT foo.go\npackage foo\nEND"},
	}
	// Simulate existing PR with matching branch.
	branch := agent.BranchSlug(10, "feat: add widget")
	mock.prs = []*forge.PullRequest{
		{Number: 100, Head: branch, State: forge.StateOpen},
	}

	d := newTestDispatcherWithPool(mock)
	result := agent.TaskResult{
		IssueNumber: 10,
		AgentType:   models.AgentTypeCodeGen,
		Success:     true,
	}

	got := d.tryApplyEdits(context.Background(), result, mock)
	if got {
		t.Error("expected tryApplyEdits to return false when PR already exists")
	}
}

func TestTryApplyEdits_CreateBranchFails(t *testing.T) {
	mock := newMockForgeAll()
	mock.issues[10] = &forge.Issue{Number: 10, Title: "feat: add widget"}
	mock.comments[10] = []*forge.Comment{
		{Body: "EDIT foo.go\npackage foo\nEND"},
	}
	mock.createBrErr = fmt.Errorf("branch creation failed") // simulate branch creation failure

	d := newTestDispatcherWithPool(mock)
	result := agent.TaskResult{
		IssueNumber: 10,
		AgentType:   models.AgentTypeCodeGen,
		Success:     true,
	}

	got := d.tryApplyEdits(context.Background(), result, mock)
	if got {
		t.Error("expected tryApplyEdits to return false on branch creation failure")
	}
	// Should have posted an error comment.
	if len(mock.commentsAdded[10]) == 0 {
		t.Error("expected error comment to be posted")
	}
}

func TestLabelSliceContains(t *testing.T) {
	labels := []string{"agent:code-gen", "status:needs-qc", "feat"}

	if !labelSliceContains(labels, "status:needs-qc") {
		t.Error("expected labelSliceContains to find status:needs-qc")
	}
	if labelSliceContains(labels, "status:done") {
		t.Error("expected labelSliceContains to not find status:done")
	}
}

func TestAgentTypeFromLabels(t *testing.T) {
	tests := []struct {
		labels []string
		want   models.AgentType
	}{
		{[]string{"agent:code-gen", "feat"}, models.AgentTypeCodeGen},
		{[]string{"agent:test", "priority:high"}, models.AgentTypeTest},
		{[]string{"agent:docs", "chore"}, models.AgentTypeDocs},
		{[]string{"feat", "priority:high"}, ""},
	}

	for _, tt := range tests {
		got := agentTypeFromLabels(tt.labels)
		if got != tt.want {
			t.Errorf("agentTypeFromLabels(%v) = %q, want %q", tt.labels, got, tt.want)
		}
	}
}
