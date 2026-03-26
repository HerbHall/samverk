package agent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
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

	reg := provider.NewRegistry(nil)
	reg.Register("test", "test", mp, "test-model")
	reg.SetRouting(map[string][]string{
		"default": {"test"},
	})

	costs := cost.NewTracker(ms, 0, 24)
	return NewPool(reg, &mockTracker{}, ms, costs, workers, zap.NewNop())
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

func TestPool_ConcurrentSubmitAndShutdown(t *testing.T) {
	// This test verifies that concurrent Submit and Shutdown calls do not
	// panic with "send on closed channel". Before the fix in #324,
	// Submit released the lock before the channel send, creating a
	// TOCTOU race where Shutdown could close the channel in between.
	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return &provider.ChatResponse{
				Message: provider.Message{Role: provider.RoleAssistant, Content: "done"},
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}

	// Run multiple iterations to increase the chance of hitting the race.
	for iter := range 50 {
		pool := newTestPool(t, 2, mp)

		var wg sync.WaitGroup

		// Goroutines that submit tasks as fast as possible.
		for g := range 4 {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				for i := range 5 {
					err := pool.Submit(Task{
						Issue:     &forge.Issue{Number: iter*100 + g*10 + i, Title: "Race", Body: "body"},
						AgentType: models.AgentTypeCodeGen,
						SessionID: "sess-race",
					})
					if errors.Is(err, ErrPoolShutdown) {
						return
					}
				}
			}(g)
		}

		// Shut down concurrently — no delay, maximum race pressure.
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.Shutdown()
		}()

		wg.Wait()
		_ = iter // suppress unused warning
	}
	// If we get here without panicking, the fix works.
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

func TestWorkers(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 3, mp)
	defer pool.Shutdown()

	if got := pool.Workers(); got != 3 {
		t.Errorf("Workers() = %d, want 3", got)
	}
}

func TestAddWorkers(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 2, mp)
	pool.SetMaxWorkers(10)
	defer pool.Shutdown()

	if err := pool.AddWorkers(3); err != nil {
		t.Fatalf("AddWorkers returned error: %v", err)
	}
	if got := pool.Workers(); got != 5 {
		t.Errorf("Workers() = %d, want 5 after adding 3", got)
	}
}

func TestAddWorkers_Noop(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 2, mp)
	defer pool.Shutdown()

	if err := pool.AddWorkers(0); err != nil {
		t.Fatalf("AddWorkers(0) returned error: %v", err)
	}
	if err := pool.AddWorkers(-1); err != nil {
		t.Fatalf("AddWorkers(-1) returned error: %v", err)
	}
	if got := pool.Workers(); got != 2 {
		t.Errorf("Workers() = %d, want 2 after noop adds", got)
	}
}

func TestAddWorkers_AfterShutdown(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 1, mp)
	pool.Shutdown()

	if err := pool.AddWorkers(2); !errors.Is(err, ErrPoolShutdown) {
		t.Errorf("AddWorkers after Shutdown: got %v, want ErrPoolShutdown", err)
	}
}

func TestAddWorkers_NewWorkersProcessTasks(t *testing.T) {
	var processed atomic.Int32
	// Gate blocks all workers until we explicitly release them.
	ready := make(chan struct{})
	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			<-ready
			processed.Add(1)
			return &provider.ChatResponse{
				Message: provider.Message{Role: provider.RoleAssistant, Content: "ok"},
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}

	// Start with 1 worker, then scale up to handle 3 concurrent tasks.
	pool := newTestPool(t, 1, mp)

	for i := range 3 {
		if err := pool.Submit(Task{
			Issue:     &forge.Issue{Number: i + 1, Title: "t", Body: "b"},
			AgentType: models.AgentTypeCodeGen,
			SessionID: "sess-" + string(rune('a'+i)),
		}); err != nil {
			t.Fatalf("Submit %d: %v", i, err)
		}
	}

	if err := pool.AddWorkers(2); err != nil {
		t.Fatalf("AddWorkers: %v", err)
	}

	// Release all workers and wait for all tasks to complete.
	close(ready)
	pool.Shutdown()

	if got := processed.Load(); got != 3 {
		t.Errorf("processed = %d, want 3", got)
	}
}

func TestRemoveWorkers_ReducesCount(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 4, mp)
	defer pool.Shutdown()

	removed := pool.RemoveWorkers(2)
	if removed != 2 {
		t.Errorf("RemoveWorkers(2) = %d, want 2", removed)
	}
	if got := pool.Workers(); got != 2 {
		t.Errorf("Workers() = %d, want 2 after removing 2", got)
	}
}

func TestRemoveWorkers_MinimumOne(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 1, mp)
	defer pool.Shutdown()

	removed := pool.RemoveWorkers(5)
	if removed != 0 {
		t.Errorf("RemoveWorkers(5) on single-worker pool = %d, want 0", removed)
	}
	if got := pool.Workers(); got != 1 {
		t.Errorf("Workers() = %d, want 1 (minimum preserved)", got)
	}
}

func TestRemoveWorkers_Noop(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 3, mp)
	defer pool.Shutdown()

	if removed := pool.RemoveWorkers(0); removed != 0 {
		t.Errorf("RemoveWorkers(0) = %d, want 0", removed)
	}
	if removed := pool.RemoveWorkers(-1); removed != 0 {
		t.Errorf("RemoveWorkers(-1) = %d, want 0", removed)
	}
	if got := pool.Workers(); got != 3 {
		t.Errorf("Workers() = %d, want 3 after noop removes", got)
	}
}

func TestRemoveWorkers_ClampsToMinimum(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	// 3 workers; remove 10 — only 2 can be removed (minimum 1 preserved).
	pool := newTestPool(t, 3, mp)
	defer pool.Shutdown()

	removed := pool.RemoveWorkers(10)
	if removed != 2 {
		t.Errorf("RemoveWorkers(10) on 3-worker pool = %d, want 2", removed)
	}
	if got := pool.Workers(); got != 1 {
		t.Errorf("Workers() = %d, want 1 after clamped remove", got)
	}
}

func TestRemoveWorkers_GracefulDuringActiveTask(t *testing.T) {
	// Verify that a worker finishes its current task before exiting when
	// a quit signal is pending.
	var processed atomic.Int32
	taskStarted := make(chan struct{})
	releaseTask := make(chan struct{})

	mp := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			close(taskStarted)
			<-releaseTask
			processed.Add(1)
			return &provider.ChatResponse{
				Message: provider.Message{Role: provider.RoleAssistant, Content: "done"},
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}

	pool := newTestPool(t, 2, mp)

	// Submit a task and wait for it to start.
	if err := pool.Submit(Task{
		Issue:     &forge.Issue{Number: 1, Title: "t", Body: "b"},
		AgentType: models.AgentTypeCodeGen,
		SessionID: "sess-1",
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	<-taskStarted

	// Signal one worker to exit. It should not exit mid-task.
	removed := pool.RemoveWorkers(1)
	if removed != 1 {
		t.Fatalf("RemoveWorkers(1) = %d, want 1", removed)
	}

	// Release the task; the worker finishes, then exits.
	close(releaseTask)
	pool.Shutdown()

	if got := processed.Load(); got != 1 {
		t.Errorf("processed = %d, want 1 (task must finish before worker exits)", got)
	}
}

func TestResize_ScalesUp(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 2, mp)
	pool.SetMaxWorkers(10)
	defer pool.Shutdown()

	if err := pool.Resize(5); err != nil {
		t.Fatalf("Resize(5): %v", err)
	}
	if got := pool.Workers(); got != 5 {
		t.Errorf("Workers() = %d, want 5", got)
	}
}

func TestResize_ScalesDown(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 5, mp)
	pool.SetMaxWorkers(10)
	defer pool.Shutdown()

	if err := pool.Resize(2); err != nil {
		t.Fatalf("Resize(2): %v", err)
	}
	if got := pool.Workers(); got != 2 {
		t.Errorf("Workers() = %d, want 2", got)
	}
}

func TestResize_EnforcesMinimumOne(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 3, mp)
	defer pool.Shutdown()

	if err := pool.Resize(0); err != nil {
		t.Fatalf("Resize(0): %v", err)
	}
	if got := pool.Workers(); got != 1 {
		t.Errorf("Workers() = %d, want 1 (minimum enforced)", got)
	}
}

func TestResize_RespectsMax(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 2, mp)
	pool.SetMaxWorkers(4)
	defer pool.Shutdown()

	if err := pool.Resize(10); !errors.Is(err, ErrMaxWorkers) {
		t.Fatalf("Resize(10) with max=4: got %v, want ErrMaxWorkers", err)
	}
	if got := pool.Workers(); got != 2 {
		t.Errorf("Workers() = %d, want 2 (unchanged after rejected Resize)", got)
	}
}

func TestAddWorkers_RespectsMax(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 2, mp)
	pool.SetMaxWorkers(3)
	defer pool.Shutdown()

	// Adding 2 would put us at 4 > max 3.
	if err := pool.AddWorkers(2); !errors.Is(err, ErrMaxWorkers) {
		t.Fatalf("AddWorkers(2) with max=3: got %v, want ErrMaxWorkers", err)
	}
	if got := pool.Workers(); got != 2 {
		t.Errorf("Workers() = %d, want 2 (unchanged after rejected add)", got)
	}
}

func TestResize_RapidSequential(t *testing.T) {
	// Rapid Resize calls should not panic or corrupt state.
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 3, mp)
	pool.SetMaxWorkers(20)
	defer pool.Shutdown()

	targets := []int{5, 2, 8, 1, 4, 6, 3}
	for _, target := range targets {
		if err := pool.Resize(target); err != nil {
			t.Fatalf("Resize(%d): %v", target, err)
		}
	}
	// Final target was 3; workers should be 3.
	if got := pool.Workers(); got != 3 {
		t.Errorf("Workers() = %d, want 3 after rapid resize sequence", got)
	}
}

func TestPool_FailoverOnProviderTimeout(t *testing.T) {
	var results []TaskResult
	var mu sync.Mutex

	// First provider returns a retryable timeout error.
	// Second provider succeeds.
	callCount := 0
	firstProvider := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			callCount++
			return nil, &provider.ErrProviderTimeout{
				Provider:    "first",
				TimeoutType: provider.TimeoutStale,
				Duration:    30 * time.Second,
			}
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "first" },
	}

	secondProvider := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return &provider.ChatResponse{
				Message: provider.Message{Role: provider.RoleAssistant, Content: "done via fallback"},
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "second" },
	}

	ms := &mockStore{
		getSessionFn: func(_ context.Context, id string) (*models.Session, error) {
			return &models.Session{
				ID:        id,
				Status:    models.SessionStatusPending,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil
		},
		updateSessionFn: func(_ context.Context, _ *models.Session) error { return nil },
		computeCostFn: func(_ context.Context, _ time.Time) (*models.CostSummary, error) {
			return &models.CostSummary{}, nil
		},
	}

	reg := provider.NewRegistry(nil)
	reg.Register("first", "claude-cli", firstProvider, "model-1")
	reg.Register("second", "claude", secondProvider, "model-2")
	reg.SetRouting(map[string][]string{
		"default": {"first", "second"},
	})

	costs := cost.NewTracker(ms, 0, 24)
	pool := NewPool(reg, &mockTracker{}, ms, costs, 1, zap.NewNop())
	defer pool.Shutdown()

	pool.SetOnComplete(func(r TaskResult) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})

	task := Task{
		Issue:     &forge.Issue{Number: 100, Title: "Timeout test", Body: "body"},
		AgentType: models.AgentTypeCodeGen,
		SessionID: "sess-timeout",
	}
	if err := pool.Submit(task); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
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
		t.Errorf("expected Success=true after failover, got false (err=%s)", results[0].Error)
	}
}

func TestPool_NoFailoverOnNonTimeout(t *testing.T) {
	var results []TaskResult
	var mu sync.Mutex

	// Provider returns a non-retryable error. No failover should happen.
	firstProvider := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			return nil, errors.New("permanent failure")
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "first" },
	}

	secondCallCount := atomic.Int32{}
	secondProvider := &mockProvider{
		chatFn: func(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
			secondCallCount.Add(1)
			return &provider.ChatResponse{
				Message: provider.Message{Role: provider.RoleAssistant, Content: "should not be called"},
			}, nil
		},
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "second" },
	}

	ms := &mockStore{
		getSessionFn: func(_ context.Context, id string) (*models.Session, error) {
			return &models.Session{
				ID:        id,
				Status:    models.SessionStatusPending,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil
		},
		updateSessionFn: func(_ context.Context, _ *models.Session) error { return nil },
		computeCostFn: func(_ context.Context, _ time.Time) (*models.CostSummary, error) {
			return &models.CostSummary{}, nil
		},
	}

	reg := provider.NewRegistry(nil)
	reg.Register("first", "claude-cli", firstProvider, "model-1")
	reg.Register("second", "claude", secondProvider, "model-2")
	reg.SetRouting(map[string][]string{
		"default": {"first", "second"},
	})

	costs := cost.NewTracker(ms, 0, 24)
	pool := NewPool(reg, &mockTracker{}, ms, costs, 1, zap.NewNop())
	defer pool.Shutdown()

	pool.SetOnComplete(func(r TaskResult) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})

	task := Task{
		Issue:     &forge.Issue{Number: 101, Title: "Non-timeout test", Body: "body"},
		AgentType: models.AgentTypeCodeGen,
		SessionID: "sess-noretry",
	}
	if err := pool.Submit(task); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
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
		t.Error("expected Success=false for non-retryable error")
	}
	if secondCallCount.Load() != 0 {
		t.Errorf("second provider should not have been called, but was called %d times", secondCallCount.Load())
	}
}

func TestPoolSetRepoDir(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 1, mp)
	defer pool.Shutdown()

	if pool.repoDir != "" {
		t.Errorf("repoDir should be empty initially, got %q", pool.repoDir)
	}

	pool.SetRepoDir("/tmp/test-repo")
	if pool.repoDir != "/tmp/test-repo" {
		t.Errorf("repoDir = %q, want /tmp/test-repo", pool.repoDir)
	}
}

func TestPoolFetchLatest_EmptyRepoDir(t *testing.T) {
	mp := &mockProvider{
		healthyFn: func(_ context.Context) bool { return true },
		nameFn:    func() string { return "test" },
	}
	pool := newTestPool(t, 1, mp)
	defer pool.Shutdown()

	// Should be a no-op, not panic.
	pool.fetchLatestFor("test", "repo")
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
	addCommentFn   func(ctx context.Context, number int, body string) (*forge.Comment, error)
	listCommentsFn func(ctx context.Context, number int) ([]*forge.Comment, error)
}

func (m *mockTracker) AddComment(ctx context.Context, number int, body string) (*forge.Comment, error) {
	if m.addCommentFn != nil {
		return m.addCommentFn(ctx, number, body)
	}
	return &forge.Comment{ID: 1, Body: body}, nil
}

func (m *mockTracker) ListComments(ctx context.Context, number int) ([]*forge.Comment, error) {
	if m.listCommentsFn != nil {
		return m.listCommentsFn(ctx, number)
	}
	return nil, nil
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

func (m *mockStore) UpdateTaskProfile(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockStore) SaveFailureEvent(_ context.Context, _ *models.FailureEvent) error {
	return nil
}

func (m *mockStore) RecentFailuresForIssue(_ context.Context, _, _ int) ([]string, error) {
	return nil, nil
}

func (m *mockStore) Close() error {
	return nil
}
