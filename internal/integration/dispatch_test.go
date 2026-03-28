//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"
	"time"

	"github.com/herbhall/samverk/internal/agent"
	"github.com/herbhall/samverk/internal/cost"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
)

// --- Mock Provider ---

type mockProvider struct {
	chatFn func(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error)
}

func (m *mockProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	return m.chatFn(ctx, req)
}

func (m *mockProvider) Healthy(_ context.Context) bool { return true }
func (m *mockProvider) Name() string                   { return "mock" }

// --- Mock IssueTracker ---

type mockTracker struct {
	forge.IssueTracker
	mu       sync.Mutex
	comments map[int][]string
}

func newMockTracker() *mockTracker {
	return &mockTracker{comments: make(map[int][]string)}
}

func (m *mockTracker) AddComment(_ context.Context, number int, body string) (*forge.Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.comments[number] = append(m.comments[number], body)
	return &forge.Comment{ID: 1, Body: body}, nil
}

func (m *mockTracker) getComments(number int) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.comments[number]
}

// --- Mock Store ---

type mockStore struct {
	store.Store
	mu       sync.Mutex
	sessions map[string]*models.Session
}

func newMockStore() *mockStore {
	return &mockStore{sessions: make(map[string]*models.Session)}
}

func (m *mockStore) CreateSession(_ context.Context, s *models.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
	return nil
}

func (m *mockStore) GetSession(_ context.Context, id string) (*models.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (m *mockStore) UpdateSession(_ context.Context, s *models.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
	return nil
}

func (m *mockStore) RecordCost(_ context.Context, _ *models.CostRecord) error {
	return nil
}

func (m *mockStore) ComputeCostSince(_ context.Context, _ time.Time) (*models.CostSummary, error) {
	return &models.CostSummary{}, nil
}

func (m *mockStore) GetBudgetStatus(_ context.Context, budget float64) (float64, float64, error) {
	return 0, budget, nil
}

func (m *mockStore) Close() error { return nil }

func (m *mockStore) SaveFailureEvent(_ context.Context, _ *models.FailureEvent) error { return nil }
func (m *mockStore) ListFailureEvents(_ context.Context, _ time.Time, _ int) ([]*models.FailureEvent, error) {
	return nil, nil
}
func (m *mockStore) CountFailuresByIssue(_ context.Context, _ int) (int, error) { return 0, nil }
func (m *mockStore) GetFailureSummary(_ context.Context, _ time.Time) (*models.FailureSummary, error) {
	return &models.FailureSummary{ByClass: map[models.FailureClass]int{}}, nil
}
func (m *mockStore) GetIssueFailureCount(_ context.Context, _ int) (int, error)       { return 0, nil }
func (m *mockStore) IncrementIssueFailureCount(_ context.Context, _ int) (int, error)  { return 1, nil }
func (m *mockStore) ClearIssueFailureCount(_ context.Context, _ int) error             { return nil }
func (m *mockStore) ResetAllFailureCounts(_ context.Context) (int64, error)            { return 0, nil }
func (m *mockStore) SaveCorrectionEvent(_ context.Context, _ *models.CorrectionEvent) error {
	return nil
}
func (m *mockStore) ListCorrectionEvents(_ context.Context, _ int) ([]*models.CorrectionEvent, error) {
	return nil, nil
}

func (m *mockStore) SaveTriageDecision(_ context.Context, _ *store.TriageDecision) error {
	return nil
}
func (m *mockStore) ListTriageDecisions(_ context.Context, _ time.Time, _ int) ([]store.TriageDecision, error) {
	return nil, nil
}
func (m *mockStore) GetTriageDecisionForIssue(_ context.Context, _ int) (*store.TriageDecision, error) {
	return nil, store.ErrNotFound
}
func (m *mockStore) UpdateTriageReview(_ context.Context, _ int64, _, _ string) error { return nil }

func (m *mockStore) getSession(id string) *models.Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s == nil {
		return nil
	}
	cp := *s
	return &cp
}

// --- Mock RepoWriter ---

type mockRepoWriter struct {
	mu       sync.Mutex
	branches []string
	files    map[string]string // branch/path -> content
}

func newMockRepoWriter() *mockRepoWriter {
	return &mockRepoWriter{files: make(map[string]string)}
}

func (m *mockRepoWriter) CreateBranch(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.branches = append(m.branches, name)
	return nil
}

func (m *mockRepoWriter) CreateOrUpdateFile(_ context.Context, branch, path, content, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[branch+"/"+path] = content
	return nil
}

// --- Mock PRManager ---

type mockPRManager struct {
	forge.PullRequestManager
	mu   sync.Mutex
	prs  []*forge.CreatePRRequest
	last int
}

func (m *mockPRManager) CreatePullRequest(_ context.Context, req *forge.CreatePRRequest) (*forge.PullRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.last++
	m.prs = append(m.prs, req)
	return &forge.PullRequest{Number: m.last, Title: req.Title, Head: req.Head, Base: req.Base}, nil
}

// --- Tests ---

func TestResearchAgentHappyPath(t *testing.T) {
	ms := newMockStore()
	mt := newMockTracker()
	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return &provider.ChatResponse{
				Message:          provider.Message{Role: provider.RoleAssistant, Content: "Research findings: the answer is 42."},
				PromptTokens:     100,
				CompletionTokens: 50,
			}, nil
		},
	}

	sessionID := "sess-research-1"
	ms.sessions[sessionID] = &models.Session{
		ID:        sessionID,
		Status:    models.SessionStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	costs := cost.NewTracker(ms, 0, 24)
	runner := agent.NewRunner(mp, "test-model", mt, ms, costs, zap.NewNop())

	task := agent.Task{
		Issue: &forge.Issue{
			Number: 42,
			Title:  "Research: competitive landscape",
			Body:   "Analyze the competitive landscape for network tools.",
			Labels: []string{models.LabelAgentResearch},
		},
		AgentType: models.AgentTypeResearch,
		SessionID: sessionID,
	}

	err := runner.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify session completed.
	session := ms.getSession(sessionID)
	if session.Status != models.SessionStatusCompleted {
		t.Errorf("session status = %q, want %q", session.Status, models.SessionStatusCompleted)
	}
	if session.FinishedAt == nil {
		t.Error("session.FinishedAt should be set")
	}

	// Verify comment posted.
	comments := mt.getComments(42)
	if len(comments) != 1 {
		t.Fatalf("got %d comments, want 1", len(comments))
	}
	if !strings.Contains(comments[0], "Research findings") {
		t.Errorf("comment = %q, want to contain 'Research findings'", comments[0])
	}
}

func TestCodeGenAgentOpensPR(t *testing.T) {
	ms := newMockStore()
	mt := newMockTracker()
	rw := newMockRepoWriter()
	pm := &mockPRManager{}

	editResponse := "EDIT internal/foo.go\npackage foo\n\nfunc Hello() string { return \"hello\" }\nEND\n\nPR_TITLE: feat: add hello function"

	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return &provider.ChatResponse{
				Message:          provider.Message{Role: provider.RoleAssistant, Content: editResponse},
				PromptTokens:     200,
				CompletionTokens: 100,
			}, nil
		},
	}

	sessionID := "sess-codegen-1"
	ms.sessions[sessionID] = &models.Session{
		ID:        sessionID,
		Status:    models.SessionStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	costs := cost.NewTracker(ms, 0, 24)
	runner := agent.NewRunner(mp, "test-model", mt, ms, costs, zap.NewNop())
	runner.SetRepoWriter(rw)
	runner.SetPRManager(pm)

	task := agent.Task{
		Issue: &forge.Issue{
			Number: 10,
			Title:  "Add hello function",
			Body:   "Create a hello function in internal/foo.go",
			Labels: []string{models.LabelAgentCodeGen},
		},
		AgentType: models.AgentTypeCodeGen,
		SessionID: sessionID,
	}

	err := runner.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify session completed.
	session := ms.getSession(sessionID)
	if session.Status != models.SessionStatusCompleted {
		t.Errorf("session status = %q, want %q", session.Status, models.SessionStatusCompleted)
	}

	// Verify branch created.
	if len(rw.branches) != 1 {
		t.Fatalf("got %d branches, want 1", len(rw.branches))
	}
	if !strings.HasPrefix(rw.branches[0], "agent/issue-10-") {
		t.Errorf("branch = %q, want prefix 'agent/issue-10-'", rw.branches[0])
	}

	// Verify file written.
	branch := rw.branches[0]
	content, ok := rw.files[branch+"/internal/foo.go"]
	if !ok {
		t.Fatal("file internal/foo.go not written to branch")
	}
	if !strings.Contains(content, "package foo") {
		t.Errorf("file content missing 'package foo': %q", content)
	}

	// Verify PR created.
	if len(pm.prs) != 1 {
		t.Fatalf("got %d PRs, want 1", len(pm.prs))
	}
	if pm.prs[0].Title != "feat: add hello function" {
		t.Errorf("PR title = %q, want 'feat: add hello function'", pm.prs[0].Title)
	}

	// Verify PR link comment on issue.
	comments := mt.getComments(10)
	if len(comments) != 1 {
		t.Fatalf("got %d comments, want 1", len(comments))
	}
	if !strings.Contains(comments[0], "PR opened") {
		t.Errorf("comment = %q, want to contain 'PR opened'", comments[0])
	}
}

func TestBudgetExceeded(t *testing.T) {
	ms := newMockStore()
	mt := newMockTracker()

	// Pre-set budget status: already spent $10 of $5 budget.
	budgetStore := &budgetExceededStore{mockStore: ms}

	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			t.Error("Chat should not be called when budget is exceeded")
			return nil, errors.New("should not reach here")
		},
	}

	sessionID := "sess-budget-1"
	ms.sessions[sessionID] = &models.Session{
		ID:        sessionID,
		Status:    models.SessionStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	costs := cost.NewTracker(budgetStore, 5.0, 24)
	runner := agent.NewRunner(mp, "test-model", mt, budgetStore, costs, zap.NewNop())

	task := agent.Task{
		Issue: &forge.Issue{
			Number: 99,
			Title:  "Should fail",
			Body:   "This should fail due to budget.",
			Labels: []string{models.LabelAgentResearch},
		},
		AgentType: models.AgentTypeResearch,
		SessionID: sessionID,
	}

	err := runner.Run(context.Background(), task)
	if !errors.Is(err, cost.ErrBudgetExceeded) {
		t.Fatalf("Run() error = %v, want ErrBudgetExceeded", err)
	}

	// Verify session marked failed.
	session := ms.getSession(sessionID)
	if session.Status != models.SessionStatusFailed {
		t.Errorf("session status = %q, want %q", session.Status, models.SessionStatusFailed)
	}

	// Verify error comment posted.
	comments := mt.getComments(99)
	if len(comments) != 1 {
		t.Fatalf("got %d comments, want 1", len(comments))
	}
	if !strings.Contains(comments[0], "budget exceeded") {
		t.Errorf("comment = %q, want to contain 'budget exceeded'", comments[0])
	}
}

func TestProviderError(t *testing.T) {
	ms := newMockStore()
	mt := newMockTracker()

	providerErr := errors.New("connection refused")
	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return nil, providerErr
		},
	}

	sessionID := "sess-error-1"
	ms.sessions[sessionID] = &models.Session{
		ID:        sessionID,
		Status:    models.SessionStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	costs := cost.NewTracker(ms, 0, 24)
	runner := agent.NewRunner(mp, "test-model", mt, ms, costs, zap.NewNop())

	task := agent.Task{
		Issue: &forge.Issue{
			Number: 77,
			Title:  "Should error",
			Body:   "Provider will return an error.",
			Labels: []string{models.LabelAgentDocs},
		},
		AgentType: models.AgentTypeDocs,
		SessionID: sessionID,
	}

	err := runner.Run(context.Background(), task)
	if err == nil {
		t.Fatal("Run() should return error")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error = %v, want to contain 'connection refused'", err)
	}

	// Verify session marked failed.
	session := ms.getSession(sessionID)
	if session.Status != models.SessionStatusFailed {
		t.Errorf("session status = %q, want %q", session.Status, models.SessionStatusFailed)
	}

	// Verify error comment posted.
	comments := mt.getComments(77)
	if len(comments) != 1 {
		t.Fatalf("got %d comments, want 1", len(comments))
	}
	if !strings.Contains(comments[0], "provider error") {
		t.Errorf("comment = %q, want to contain 'provider error'", comments[0])
	}
}

// budgetExceededStore wraps mockStore but reports spent > budget.
type budgetExceededStore struct {
	*mockStore
}

func (b *budgetExceededStore) GetBudgetStatus(_ context.Context, budget float64) (float64, float64, error) {
	return 10.0, budget - 10.0, nil
}

func (b *budgetExceededStore) ComputeCostSince(_ context.Context, _ time.Time) (*models.CostSummary, error) {
	return &models.CostSummary{TotalCostUSD: 10.0}, nil
}
