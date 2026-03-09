package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/cost"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/pkg/models"
)

// newTestRunner creates a runner with the given mocks for testing.
func newTestRunner(mp *mockProvider, mt *mockTracker, ms *mockStore) *Runner {
	costs := cost.NewTracker(ms, 0, 24)
	return NewRunner(mp, "test-model", mt, ms, costs)
}

func newDefaultMockStore() *mockStore {
	return &mockStore{
		getSessionFn: func(_ context.Context, id string) (*models.Session, error) {
			return &models.Session{
				ID:        id,
				Status:    models.SessionStatusPending,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil
		},
		updateSessionFn: func(_ context.Context, _ *models.Session) error {
			return nil
		},
		recordCostFn: func(_ context.Context, _ *models.CostRecord) error {
			return nil
		},
		computeCostFn: func(_ context.Context, _ time.Time) (*models.CostSummary, error) {
			return &models.CostSummary{}, nil
		},
	}
}

func newDefaultTask() Task {
	return Task{
		Issue: &forge.Issue{
			Number: 42,
			Title:  "Fix login bug",
			Body:   "Users cannot log in when password contains special characters.",
			Labels: []string{"bug", "auth"},
		},
		AgentType: models.AgentTypeCodeGen,
		SessionID: "sess-test-1",
	}
}

func TestRunnerSuccess(t *testing.T) {
	var (
		commentPosted string
		sessionStates []models.SessionStatus
	)

	ms := newDefaultMockStore()
	ms.updateSessionFn = func(_ context.Context, s *models.Session) error {
		sessionStates = append(sessionStates, s.Status)
		return nil
	}

	mp := &mockProvider{
		chatFn: func(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
			if len(req.Messages) != 2 {
				t.Errorf("expected 2 messages, got %d", len(req.Messages))
			}
			if req.Messages[0].Role != provider.RoleSystem {
				t.Errorf("first message role = %q, want %q", req.Messages[0].Role, provider.RoleSystem)
			}
			return &provider.ChatResponse{
				Message: provider.Message{
					Role:    provider.RoleAssistant,
					Content: "I fixed the login bug.",
				},
				PromptTokens:     100,
				CompletionTokens: 50,
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test-provider" },
	}

	mt := &mockTracker{
		addCommentFn: func(_ context.Context, number int, body string) (*forge.Comment, error) {
			if number != 42 {
				t.Errorf("AddComment number = %d, want 42", number)
			}
			commentPosted = body
			return &forge.Comment{ID: 1, Body: body}, nil
		},
	}

	runner := newTestRunner(mp, mt, ms)
	task := newDefaultTask()

	err := runner.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if commentPosted != "I fixed the login bug." {
		t.Errorf("comment = %q, want %q", commentPosted, "I fixed the login bug.")
	}

	// Session should transition: active -> completed.
	if len(sessionStates) < 2 {
		t.Fatalf("expected at least 2 session updates, got %d", len(sessionStates))
	}
	if sessionStates[0] != models.SessionStatusActive {
		t.Errorf("first status = %q, want %q", sessionStates[0], models.SessionStatusActive)
	}
	if sessionStates[len(sessionStates)-1] != models.SessionStatusCompleted {
		t.Errorf("last status = %q, want %q", sessionStates[len(sessionStates)-1], models.SessionStatusCompleted)
	}
}

func TestRunnerBudgetExceeded(t *testing.T) {
	var sessionStates []models.SessionStatus

	ms := newDefaultMockStore()
	ms.computeCostFn = func(_ context.Context, _ time.Time) (*models.CostSummary, error) {
		return &models.CostSummary{TotalCostUSD: 10.0}, nil
	}
	ms.updateSessionFn = func(_ context.Context, s *models.Session) error {
		sessionStates = append(sessionStates, s.Status)
		return nil
	}

	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			t.Error("Chat should not be called when budget is exceeded")
			return nil, errors.New("should not reach here")
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test-provider" },
	}

	mt := &mockTracker{}

	// Budget = $5, spent = $10 -> exceeded.
	costs := cost.NewTracker(ms, 5.0, 24)
	runner := NewRunner(mp, "test-model", mt, ms, costs)
	task := newDefaultTask()

	err := runner.Run(context.Background(), task)
	if !errors.Is(err, cost.ErrBudgetExceeded) {
		t.Fatalf("expected ErrBudgetExceeded, got %v", err)
	}

	// Session should transition: active -> failed.
	hasActive := false
	hasFailed := false
	for _, s := range sessionStates {
		if s == models.SessionStatusActive {
			hasActive = true
		}
		if s == models.SessionStatusFailed {
			hasFailed = true
		}
	}
	if !hasActive {
		t.Error("expected session to be set to active before budget check")
	}
	if !hasFailed {
		t.Error("expected session to be set to failed after budget exceeded")
	}
}

func TestRunnerProviderError(t *testing.T) {
	var (
		sessionStates []models.SessionStatus
		errorComment  string
	)

	ms := newDefaultMockStore()
	ms.updateSessionFn = func(_ context.Context, s *models.Session) error {
		sessionStates = append(sessionStates, s.Status)
		return nil
	}

	providerErr := errors.New("model unavailable")
	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return nil, providerErr
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test-provider" },
	}

	mt := &mockTracker{
		addCommentFn: func(_ context.Context, _ int, body string) (*forge.Comment, error) {
			errorComment = body
			return &forge.Comment{ID: 1, Body: body}, nil
		},
	}

	runner := newTestRunner(mp, mt, ms)
	task := newDefaultTask()

	err := runner.Run(context.Background(), task)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Session should end as failed.
	hasFailed := false
	for _, s := range sessionStates {
		if s == models.SessionStatusFailed {
			hasFailed = true
		}
	}
	if !hasFailed {
		t.Error("expected session to be set to failed after provider error")
	}

	if errorComment == "" {
		t.Error("expected error comment to be posted")
	}
}

func TestRunnerTrackerError(t *testing.T) {
	var sessionStates []models.SessionStatus

	ms := newDefaultMockStore()
	ms.updateSessionFn = func(_ context.Context, s *models.Session) error {
		sessionStates = append(sessionStates, s.Status)
		return nil
	}

	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return &provider.ChatResponse{
				Message: provider.Message{
					Role:    provider.RoleAssistant,
					Content: "Here is the fix.",
				},
				PromptTokens:     100,
				CompletionTokens: 50,
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test-provider" },
	}

	trackerErr := errors.New("GitHub API rate limited")
	callCount := 0
	mt := &mockTracker{
		addCommentFn: func(_ context.Context, _ int, _ string) (*forge.Comment, error) {
			callCount++
			if callCount == 1 {
				// First call: posting the response comment fails.
				return nil, trackerErr
			}
			// Second call: posting the error comment succeeds.
			return &forge.Comment{ID: 1}, nil
		},
	}

	runner := newTestRunner(mp, mt, ms)
	task := newDefaultTask()

	err := runner.Run(context.Background(), task)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Session should end as failed.
	hasFailed := false
	for _, s := range sessionStates {
		if s == models.SessionStatusFailed {
			hasFailed = true
		}
	}
	if !hasFailed {
		t.Error("expected session to be set to failed after tracker error")
	}
}

func TestRunnerHeartbeat(t *testing.T) {
	var heartbeatCalls int

	ms := newDefaultMockStore()
	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return &provider.ChatResponse{
				Message: provider.Message{
					Role:    provider.RoleAssistant,
					Content: "done",
				},
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test-provider" },
	}
	mt := &mockTracker{
		addCommentFn: func(_ context.Context, _ int, _ string) (*forge.Comment, error) {
			return &forge.Comment{ID: 1}, nil
		},
	}

	runner := newTestRunner(mp, mt, ms)
	task := newDefaultTask()
	task.HeartbeatFunc = func() {
		heartbeatCalls++
	}

	err := runner.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// HeartbeatFunc must be called at least once (immediately before Chat).
	if heartbeatCalls < 1 {
		t.Errorf("HeartbeatFunc called %d times, want >= 1", heartbeatCalls)
	}
}

func TestRunnerHeartbeatNil(t *testing.T) {
	// When HeartbeatFunc is nil the runner must not panic.
	ms := newDefaultMockStore()
	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return &provider.ChatResponse{
				Message: provider.Message{
					Role:    provider.RoleAssistant,
					Content: "done",
				},
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test-provider" },
	}
	mt := &mockTracker{
		addCommentFn: func(_ context.Context, _ int, _ string) (*forge.Comment, error) {
			return &forge.Comment{ID: 1}, nil
		},
	}

	runner := newTestRunner(mp, mt, ms)
	task := newDefaultTask()
	// HeartbeatFunc intentionally left nil.

	if err := runner.Run(context.Background(), task); err != nil {
		t.Fatalf("Run returned error with nil HeartbeatFunc: %v", err)
	}
}

// Old buildSystemPrompt tests removed — replaced by prompts_test.go
// which covers BuildSystemPrompt with per-agent-type verification.
