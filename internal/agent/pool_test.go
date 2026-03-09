package agent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/cost"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
)

// newTestPool creates a pool wired with mocks for testing.
func newTestPool(t *testing.T, workers int, mp *mockProvider) *Pool {
	t.Helper()

	ms := &mockStore{
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

	reg := provider.NewRegistry()
	reg.Register("test", "test", mp, "test-model")
	reg.SetRouting(map[string][]string{
		"default": {"test"},
	})

	costs := cost.NewTracker(ms, 0, 24)
	return NewPool(reg, &mockTracker{}, ms, costs, workers)
}

func TestPoolSubmitAndProcess(t *testing.T) {
	var processed atomic.Int32
	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			processed.Add(1)
			return &provider.ChatResponse{
				Message: provider.Message{
					Role:    provider.RoleAssistant,
					Content: "done",
				},
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}

	pool := newTestPool(t, 2, mp)
	defer pool.Shutdown()

	task := Task{
		Issue: &forge.Issue{
			Number: 1,
			Title:  "Test issue",
			Body:   "Fix the bug",
		},
		AgentType: models.AgentTypeCodeGen,
		SessionID: "sess-1",
	}

	if err := pool.Submit(task); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}

	// Allow time for processing.
	time.Sleep(100 * time.Millisecond)

	if got := processed.Load(); got != 1 {
		t.Errorf("processed = %d, want 1", got)
	}
}

func TestPoolShutdown(t *testing.T) {
	var processed atomic.Int32
	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			processed.Add(1)
			return &provider.ChatResponse{
				Message: provider.Message{
					Role:    provider.RoleAssistant,
					Content: "done",
				},
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}

	pool := newTestPool(t, 2, mp)

	for i := range 5 {
		task := Task{
			Issue: &forge.Issue{
				Number: i + 1,
				Title:  "Test issue",
				Body:   "Fix the bug",
			},
			AgentType: models.AgentTypeCodeGen,
			SessionID: "sess-" + string(rune('a'+i)),
		}
		if err := pool.Submit(task); err != nil {
			t.Fatalf("Submit returned error: %v", err)
		}
	}

	pool.Shutdown()

	if got := processed.Load(); got != 5 {
		t.Errorf("processed = %d, want 5", got)
	}
}

func TestPoolSubmitAfterShutdown(t *testing.T) {
	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return &provider.ChatResponse{
				Message: provider.Message{Role: provider.RoleAssistant, Content: "ok"},
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}

	pool := newTestPool(t, 1, mp)
	pool.Shutdown()

	err := pool.Submit(Task{
		Issue: &forge.Issue{
			Number: 1,
			Title:  "Test",
			Body:   "body",
		},
		AgentType: models.AgentTypeCodeGen,
		SessionID: "sess-1",
	})

	if !errors.Is(err, ErrPoolShutdown) {
		t.Fatalf("expected ErrPoolShutdown, got %v", err)
	}
}

func TestPool_OnCompleteCallback_Success(t *testing.T) {
	var results []TaskResult
	var mu sync.Mutex

	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return &provider.ChatResponse{
				Message: provider.Message{Role: provider.RoleAssistant, Content: "done"},
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}

	pool := newTestPool(t, 1, mp)
	defer pool.Shutdown()

	pool.SetOnComplete(func(r TaskResult) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})

	task := Task{
		Issue:     &forge.Issue{Number: 42, Title: "Test", Body: "body"},
		AgentType: models.AgentTypeCodeGen,
		SessionID: "sess-42",
	}
	if err := pool.Submit(task); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Wait for processing.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(results)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(results) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(results))
	}
	if !results[0].Success {
		t.Errorf("expected Success=true, got false (err=%s)", results[0].Error)
	}
	if results[0].IssueNumber != 42 {
		t.Errorf("IssueNumber: got %d, want 42", results[0].IssueNumber)
	}
	if results[0].SessionID != "sess-42" {
		t.Errorf("SessionID: got %q, want sess-42", results[0].SessionID)
	}
}

func TestPool_OnCompleteCallback_RunnerFailure(t *testing.T) {
	var results []TaskResult
	var mu sync.Mutex

	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return nil, errors.New("provider error")
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}

	pool := newTestPool(t, 1, mp)
	defer pool.Shutdown()

	pool.SetOnComplete(func(r TaskResult) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})

	task := Task{
		Issue:     &forge.Issue{Number: 99, Title: "Fail", Body: "body"},
		AgentType: models.AgentTypeCodeGen,
		SessionID: "sess-99",
	}
	if err := pool.Submit(task); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(results)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(results) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(results))
	}
	if results[0].Success {
		t.Errorf("expected Success=false on runner failure")
	}
	if results[0].Error == "" {
		t.Errorf("expected non-empty Error on runner failure")
	}
}

func TestPool_OnCompleteCallback_ProviderFailure(t *testing.T) {
	var results []TaskResult
	var mu sync.Mutex

	// Provider reports unhealthy so registry.Get returns an error.
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return false },
		nameFn:    func() string { return "test" },
	}

	pool := newTestPool(t, 1, mp)
	defer pool.Shutdown()

	pool.SetOnComplete(func(r TaskResult) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})

	task := Task{
		Issue:     &forge.Issue{Number: 7, Title: "No provider", Body: "body"},
		AgentType: models.AgentTypeCodeGen,
		SessionID: "sess-7",
	}
	if err := pool.Submit(task); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(results)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(results) != 1 {
		t.Fatalf("expected 1 callback on provider failure, got %d", len(results))
	}
	if results[0].Success {
		t.Errorf("expected Success=false on provider failure")
	}
	if results[0].Error == "" {
		t.Errorf("expected non-empty Error on provider failure")
	}
}

func TestPoolDefaultWorkers(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}

	pool := newTestPool(t, 0, mp)
	defer pool.Shutdown()

	if pool.workers != defaultWorkers {
		t.Errorf("workers = %d, want %d", pool.workers, defaultWorkers)
	}
}

// --- mock types for pool tests ---

type mockProvider struct {
	chatFn    func(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error)
	healthyFn func(ctx context.Context) bool
	nameFn    func() string
}

func (m *mockProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	if m.chatFn != nil {
		return m.chatFn(ctx, req)
	}
	return &provider.ChatResponse{}, nil
}

func (m *mockProvider) Healthy(ctx context.Context) bool {
	if m.healthyFn != nil {
		return m.healthyFn(ctx)
	}
	return true
}

func (m *mockProvider) Name() string {
	if m.nameFn != nil {
		return m.nameFn()
	}
	return "mock"
}

type mockTracker struct {
	forge.IssueTracker
	addCommentFn func(ctx context.Context, number int, body string) (*forge.Comment, error)
}

func (m *mockTracker) AddComment(ctx context.Context, number int, body string) (*forge.Comment, error) {
	if m.addCommentFn != nil {
		return m.addCommentFn(ctx, number, body)
	}
	return &forge.Comment{ID: 1, Body: body}, nil
}

type mockStore struct {
	store.Store
	createSessionFn func(ctx context.Context, s *models.Session) error
	getSessionFn    func(ctx context.Context, id string) (*models.Session, error)
	updateSessionFn func(ctx context.Context, s *models.Session) error
	recordCostFn    func(ctx context.Context, r *models.CostRecord) error
	computeCostFn   func(ctx context.Context, since time.Time) (*models.CostSummary, error)
	getBudgetFn     func(ctx context.Context, dailyBudgetUSD float64) (float64, float64, error)
}

func (m *mockStore) CreateSession(ctx context.Context, s *models.Session) error {
	if m.createSessionFn != nil {
		return m.createSessionFn(ctx, s)
	}
	return nil
}

func (m *mockStore) GetSession(ctx context.Context, id string) (*models.Session, error) {
	if m.getSessionFn != nil {
		return m.getSessionFn(ctx, id)
	}
	return &models.Session{ID: id}, nil
}

func (m *mockStore) UpdateSession(ctx context.Context, s *models.Session) error {
	if m.updateSessionFn != nil {
		return m.updateSessionFn(ctx, s)
	}
	return nil
}

func (m *mockStore) RecordCost(ctx context.Context, r *models.CostRecord) error {
	if m.recordCostFn != nil {
		return m.recordCostFn(ctx, r)
	}
	return nil
}

func (m *mockStore) ComputeCostSince(ctx context.Context, since time.Time) (*models.CostSummary, error) {
	if m.computeCostFn != nil {
		return m.computeCostFn(ctx, since)
	}
	return &models.CostSummary{}, nil
}

func (m *mockStore) GetBudgetStatus(ctx context.Context, dailyBudgetUSD float64) (spent, remaining float64, err error) {
	if m.getBudgetFn != nil {
		return m.getBudgetFn(ctx, dailyBudgetUSD)
	}
	return 0, dailyBudgetUSD, nil
}

func (m *mockStore) Close() error {
	return nil
}
