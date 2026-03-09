package agent

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"time"

	"github.com/herbhall/samverk/internal/cost"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/pkg/models"
)

// newTestRunner creates a runner with the given mocks for testing.
func newTestRunner(mp *mockProvider, mt *mockTracker, ms *mockStore) *Runner {
	costs := cost.NewTracker(ms, 0, 24)
	return NewRunner(mp, "test-model", mt, ms, costs, zap.NewNop())
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
	runner := NewRunner(mp, "test-model", mt, ms, costs, zap.NewNop())
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

func TestRunnerResumeFromCheckpoint(t *testing.T) {
	// When a prior CHECKPOINT comment exists, the runner should inject a
	// resume prompt into the messages sent to the provider.
	var capturedMessages []provider.Message

	ms := newDefaultMockStore()
	mp := &mockProvider{
		chatFn: func(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
			capturedMessages = req.Messages
			return &provider.ChatResponse{
				Message: provider.Message{
					Role:    provider.RoleAssistant,
					Content: "Continued from checkpoint.",
				},
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test-provider" },
	}

	checkpointBody := FormatCheckpoint("sess-old", "EDIT main.go\npackage main\nEND_EDIT")
	mt := &mockTracker{
		addCommentFn: func(_ context.Context, _ int, _ string) (*forge.Comment, error) {
			return &forge.Comment{ID: 1}, nil
		},
		listCommentsFn: func(_ context.Context, _ int) ([]*forge.Comment, error) {
			return []*forge.Comment{
				{Body: checkpointBody},
			}, nil
		},
	}

	runner := newTestRunner(mp, mt, ms)
	task := newDefaultTask()

	err := runner.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Should have 3 messages: system prompt, resume prompt, user message.
	if len(capturedMessages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(capturedMessages))
	}
	if capturedMessages[1].Role != provider.RoleSystem {
		t.Errorf("resume message role = %q, want %q", capturedMessages[1].Role, provider.RoleSystem)
	}
	if !searchString(capturedMessages[1].Content, "<prior-work>") {
		t.Error("resume message should contain <prior-work> tags")
	}
}

func TestRunnerNoCheckpoint(t *testing.T) {
	// When no checkpoint exists, messages should be the standard 2 (system + user).
	var capturedMessages []provider.Message

	ms := newDefaultMockStore()
	mp := &mockProvider{
		chatFn: func(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
			capturedMessages = req.Messages
			return &provider.ChatResponse{
				Message: provider.Message{
					Role:    provider.RoleAssistant,
					Content: "Fresh start.",
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
		listCommentsFn: func(_ context.Context, _ int) ([]*forge.Comment, error) {
			return nil, nil // no comments
		},
	}

	runner := newTestRunner(mp, mt, ms)
	task := newDefaultTask()

	err := runner.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(capturedMessages) != 2 {
		t.Errorf("expected 2 messages (no checkpoint), got %d", len(capturedMessages))
	}
}

func TestFailTaskPostsCheckpoint(t *testing.T) {
	var postedComments []string

	ms := newDefaultMockStore()
	ms.getSessionFn = func(_ context.Context, id string) (*models.Session, error) {
		return &models.Session{
			ID:            id,
			Status:        models.SessionStatusActive,
			PartialOutput: "some partial work",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}, nil
	}

	mt := &mockTracker{
		addCommentFn: func(_ context.Context, _ int, body string) (*forge.Comment, error) {
			postedComments = append(postedComments, body)
			return &forge.Comment{ID: 1, Body: body}, nil
		},
	}

	runner := newTestRunner(&mockProvider{
		nameFn: func() string { return "test" },
	}, mt, ms)
	task := newDefaultTask()

	runner.failTask(context.Background(), task, "provider timeout")

	// Should post 2 comments: checkpoint + error.
	if len(postedComments) != 2 {
		t.Fatalf("expected 2 comments (checkpoint + error), got %d", len(postedComments))
	}

	// First comment should be the checkpoint.
	if !searchString(postedComments[0], "CHECKPOINT [") {
		t.Errorf("first comment should be CHECKPOINT, got: %s", postedComments[0])
	}
	if !searchString(postedComments[0], "some partial work") {
		t.Error("checkpoint should contain partial output")
	}

	// Second comment should be the error.
	if !searchString(postedComments[1], "Agent error:") {
		t.Errorf("second comment should be error, got: %s", postedComments[1])
	}
}

func TestFailTaskSkipsCheckpointWhenEmpty(t *testing.T) {
	var postedComments []string

	ms := newDefaultMockStore()
	ms.getSessionFn = func(_ context.Context, id string) (*models.Session, error) {
		return &models.Session{
			ID:            id,
			Status:        models.SessionStatusActive,
			PartialOutput: "", // no partial output
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}, nil
	}

	mt := &mockTracker{
		addCommentFn: func(_ context.Context, _ int, body string) (*forge.Comment, error) {
			postedComments = append(postedComments, body)
			return &forge.Comment{ID: 1, Body: body}, nil
		},
	}

	runner := newTestRunner(&mockProvider{
		nameFn: func() string { return "test" },
	}, mt, ms)
	task := newDefaultTask()

	runner.failTask(context.Background(), task, "some error")

	// Should post only 1 comment: the error (no checkpoint).
	if len(postedComments) != 1 {
		t.Fatalf("expected 1 comment (error only), got %d", len(postedComments))
	}
	if !searchString(postedComments[0], "Agent error:") {
		t.Errorf("comment should be error, got: %s", postedComments[0])
	}
}

// Old buildSystemPrompt tests removed — replaced by prompts_test.go
// which covers BuildSystemPrompt with per-agent-type verification.
