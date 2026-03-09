// Package agent provides the agent runtime pool and runner for processing
// tasks as goroutines with session lifecycle management and cost recording.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/herbhall/samverk/internal/cost"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
)

// ErrPoolShutdown is returned when Submit is called on a shut-down pool.
var ErrPoolShutdown = errors.New("pool is shut down")

// defaultWorkers is the number of worker goroutines when none is specified.
const defaultWorkers = 3

// Task represents a unit of work to be processed by the agent pool.
type Task struct {
	Issue       *forge.Issue
	AgentType   models.AgentType
	SessionID   string
	ProviderKey string // routing chain key; defaults to string(AgentType) when empty
}

// TaskResult reports the outcome of a pool task back to the dispatcher.
type TaskResult struct {
	IssueNumber int
	SessionID   string
	AgentType   models.AgentType
	Success     bool
	Error       string
}

// Pool manages a fixed set of worker goroutines that process agent tasks.
type Pool struct {
	registry   *provider.Registry
	tracker    forge.IssueTracker
	store      store.Store
	costs      *cost.Tracker
	workers    int
	tasks      chan Task
	wg         sync.WaitGroup
	logger     *slog.Logger
	done       chan struct{}
	mu         sync.Mutex
	shutdown   bool
	onComplete func(TaskResult) // callback to notify dispatcher of task completion
}

// NewPool creates a pool with the given number of worker goroutines and starts
// them immediately. If workers is <= 0, it defaults to 3.
func NewPool(registry *provider.Registry, tracker forge.IssueTracker, st store.Store, costs *cost.Tracker, workers int) *Pool {
	if workers <= 0 {
		workers = defaultWorkers
	}

	p := &Pool{
		registry: registry,
		tracker:  tracker,
		store:    st,
		costs:    costs,
		workers:  workers,
		tasks:    make(chan Task, workers*2),
		logger:   slog.Default(),
		done:     make(chan struct{}),
	}

	p.wg.Add(workers)
	for range workers {
		go p.worker()
	}

	return p
}

// Submit enqueues a task for processing. Returns ErrPoolShutdown if the pool
// has been shut down.
func (p *Pool) Submit(task Task) error {
	p.mu.Lock()
	if p.shutdown {
		p.mu.Unlock()
		return ErrPoolShutdown
	}
	p.mu.Unlock()

	p.tasks <- task
	return nil
}

// Shutdown signals all workers to stop and waits for in-flight tasks to
// complete. After Shutdown returns, Submit will return ErrPoolShutdown.
func (p *Pool) Shutdown() {
	p.mu.Lock()
	if p.shutdown {
		p.mu.Unlock()
		return
	}
	p.shutdown = true
	p.mu.Unlock()

	p.logger.Info("agent pool draining", slog.Int("queued", len(p.tasks)))
	close(p.tasks)
	p.wg.Wait()
	p.logger.Info("agent pool drained")
	close(p.done)
}

// Done returns a channel that is closed after Shutdown completes.
func (p *Pool) Done() <-chan struct{} {
	return p.done
}

// SetOnComplete registers a callback invoked after each task finishes.
func (p *Pool) SetOnComplete(fn func(TaskResult)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onComplete = fn
}

// worker is the main loop for a single worker goroutine.
func (p *Pool) worker() {
	defer p.wg.Done()

	for task := range p.tasks {
		p.processTask(task)
	}
}

// processTask resolves a provider, creates a runner, and executes the task.
// It uses task.ProviderKey for registry lookup when set, falling back to
// string(task.AgentType). If the selected chain has no healthy provider,
// it retries with the "default" chain before giving up.
func (p *Pool) processTask(task Task) {
	ctx := context.Background()
	logger := p.logger.With(
		slog.String("session_id", task.SessionID),
		slog.Int("issue", task.Issue.Number),
		slog.String("agent_type", string(task.AgentType)),
	)

	routingKey := task.ProviderKey
	if routingKey == "" {
		routingKey = string(task.AgentType)
	}

	prov, model, err := p.registry.Get(ctx, routingKey)
	if err != nil && routingKey != "default" {
		// Selected provider chain is unhealthy; fall back to the default chain.
		logger.Warn("selected provider chain unhealthy, falling back to default",
			slog.String("provider_key", routingKey),
			slog.String("error", err.Error()),
		)
		prov, model, err = p.registry.Get(ctx, "default")
	}
	if err != nil {
		logger.Error("no healthy provider", slog.String("error", err.Error()))
		p.failSession(ctx, task.SessionID, fmt.Sprintf("no healthy provider: %v", err))
		// Notify dispatcher even on provider failure.
		p.mu.Lock()
		cb := p.onComplete
		p.mu.Unlock()
		if cb != nil {
			cb(TaskResult{
				IssueNumber: task.Issue.Number,
				SessionID:   task.SessionID,
				AgentType:   task.AgentType,
				Success:     false,
				Error:       err.Error(),
			})
		}
		return
	}

	runner := NewRunner(prov, model, p.tracker, p.store, p.costs)
	runErr := runner.Run(ctx, task)

	// Notify dispatcher of completion (success or failure).
	result := TaskResult{
		IssueNumber: task.Issue.Number,
		SessionID:   task.SessionID,
		AgentType:   task.AgentType,
		Success:     runErr == nil,
	}
	if runErr != nil {
		result.Error = runErr.Error()
		logger.Error("runner failed", slog.String("error", runErr.Error()))
	} else {
		logger.Info("task completed")
	}

	p.mu.Lock()
	cb := p.onComplete
	p.mu.Unlock()
	if cb != nil {
		cb(result)
	}
}

// failSession marks a session as failed when the pool cannot even start it.
func (p *Pool) failSession(ctx context.Context, sessionID, errMsg string) {
	session, err := p.store.GetSession(ctx, sessionID)
	if err != nil {
		p.logger.Error("failed to get session for failure update",
			slog.String("session_id", sessionID),
			slog.String("error", err.Error()),
		)
		return
	}

	now := time.Now()
	session.Status = models.SessionStatusFailed
	session.Error = errMsg
	session.FinishedAt = &now
	session.UpdatedAt = now

	if err = p.store.UpdateSession(ctx, session); err != nil {
		p.logger.Error("failed to update session status",
			slog.String("session_id", sessionID),
			slog.String("error", err.Error()),
		)
	}
}
