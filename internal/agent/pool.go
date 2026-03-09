// Package agent provides the agent runtime pool and runner for processing
// tasks as goroutines with session lifecycle management and cost recording.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/herbhall/samverk/internal/cost"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/metrics"
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
	Issue         *forge.Issue
	AgentType     models.AgentType
	SessionID     string
	ProviderKey   string // routing chain key; defaults to string(AgentType) when empty
	HeartbeatFunc func() // called periodically while running; signals dispatcher that work is in progress; may be nil
}

// TaskResult reports the outcome of a pool task back to the dispatcher.
type TaskResult struct {
	IssueNumber int
	SessionID   string
	AgentType   models.AgentType
	Success     bool
	Error       string
}

// workerQuitBuf is the maximum number of pending quit signals.
// Generous to accommodate burst RemoveWorkers calls.
const workerQuitBuf = 64

// Pool manages a set of worker goroutines that process agent tasks.
// Workers may be added or removed at runtime via AddWorkers/RemoveWorkers/Resize.
type Pool struct {
	registry   *provider.Registry
	tracker    forge.IssueTracker
	store      store.Store
	costs      *cost.Tracker
	workers    int        // current target worker count (under mu)
	maxWorkers int        // upper bound enforced by AddWorkers/Resize; 0 = no limit (under mu)
	tasks      chan Task
	wg         sync.WaitGroup
	logger     *slog.Logger
	done       chan struct{}
	mu         sync.Mutex
	shutdown   bool
	active     atomic.Int32                        // currently processing workers
	onComplete atomic.Pointer[func(TaskResult)]   // callback to notify dispatcher; atomic to avoid mu deadlock
	workerQuit chan struct{}                        // each send causes one worker to exit after its current task
}

// NewPool creates a pool with the given number of worker goroutines and starts
// them immediately. If workers is <= 0, it defaults to 3. The default maximum
// is runtime.NumCPU(); call SetMaxWorkers to override before scaling.
func NewPool(registry *provider.Registry, tracker forge.IssueTracker, st store.Store, costs *cost.Tracker, workers int) *Pool {
	if workers <= 0 {
		workers = defaultWorkers
	}

	p := &Pool{
		registry:   registry,
		tracker:    tracker,
		store:      st,
		costs:      costs,
		workers:    workers,
		maxWorkers: runtime.NumCPU(),
		tasks:      make(chan Task, workers*2),
		logger:     slog.Default(),
		done:       make(chan struct{}),
		workerQuit: make(chan struct{}, workerQuitBuf),
	}

	p.wg.Add(workers)
	for range workers {
		go p.worker()
	}

	return p
}

// SetMaxWorkers sets the maximum number of workers the pool may hold at any time.
// A value <= 0 removes the limit. Must be called before scaling; not safe to
// call concurrently with AddWorkers or Resize.
func (p *Pool) SetMaxWorkers(limit int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.maxWorkers = limit
}

// ErrMaxWorkers is returned when AddWorkers or Resize would exceed the
// configured maximum worker count.
var ErrMaxWorkers = errors.New("pool: would exceed maximum worker count")

// AddWorkers starts n additional worker goroutines. Returns ErrPoolShutdown if
// the pool has already been shut down, or ErrMaxWorkers if the addition would
// exceed the configured maximum. No-op if n <= 0.
func (p *Pool) AddWorkers(n int) error {
	if n <= 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.shutdown {
		return ErrPoolShutdown
	}
	if p.maxWorkers > 0 && p.workers+n > p.maxWorkers {
		return fmt.Errorf("%w: current=%d add=%d max=%d", ErrMaxWorkers, p.workers, n, p.maxWorkers)
	}
	p.workers += n
	p.wg.Add(n)
	for range n {
		go p.worker()
	}
	p.logger.Info("agent pool scaled up", slog.Int("added", n), slog.Int("workers", p.workers))
	return nil
}

// Resize sets the pool's worker count to target, adding or removing workers as
// needed. It enforces the same minimum (1) and maximum bounds as AddWorkers and
// RemoveWorkers. Returns ErrPoolShutdown if the pool has been shut down.
func (p *Pool) Resize(target int) error {
	if target < 1 {
		target = 1
	}
	p.mu.Lock()
	if p.shutdown {
		p.mu.Unlock()
		return ErrPoolShutdown
	}
	if p.maxWorkers > 0 && target > p.maxWorkers {
		p.mu.Unlock()
		return fmt.Errorf("%w: target=%d max=%d", ErrMaxWorkers, target, p.maxWorkers)
	}
	delta := target - p.workers
	p.mu.Unlock()

	switch {
	case delta > 0:
		return p.AddWorkers(delta)
	case delta < 0:
		p.RemoveWorkers(-delta)
	}
	return nil
}

// RemoveWorkers signals n workers to exit after completing their current task.
// To protect liveness, at least one worker is always kept running; the actual
// number removed may be less than n. Returns the number of quit signals sent.
func (p *Pool) RemoveWorkers(n int) int {
	if n <= 0 {
		return 0
	}
	p.mu.Lock()
	// Keep at least one worker alive.
	available := p.workers - 1
	if available <= 0 {
		p.mu.Unlock()
		return 0
	}
	if n > available {
		n = available
	}
	p.workers -= n
	p.mu.Unlock()

	sent := 0
sendLoop:
	for range n {
		select {
		case p.workerQuit <- struct{}{}:
			sent++
		default:
			// Buffer full; stop here. Remaining workers stay alive.
			p.mu.Lock()
			p.workers += (n - sent)
			p.mu.Unlock()
			break sendLoop
		}
	}
	if sent > 0 {
		p.logger.Info("agent pool scaled down", slog.Int("removed", sent), slog.Int("workers", p.Workers()))
	}
	return sent
}

// Workers returns the current target worker count.
func (p *Pool) Workers() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.workers
}

// Snapshot returns a point-in-time view of pool state for the scaling policy.
func (p *Pool) Snapshot() metrics.PoolSnapshot {
	p.mu.Lock()
	total := p.workers
	queue := len(p.tasks)
	p.mu.Unlock()
	active := int(p.active.Load())
	if active > total {
		active = total
	}
	return metrics.PoolSnapshot{
		CollectedAt:   time.Now(),
		TotalWorkers:  total,
		ActiveWorkers: active,
		IdleWorkers:   total - active,
		QueueDepth:    queue,
	}
}

// Submit enqueues a task for processing. Returns ErrPoolShutdown if the pool
// has been shut down.
//
// The lock is held across the channel send to prevent a TOCTOU race with
// Shutdown: without it, Shutdown can close the channel between the flag
// check and the send, causing a panic. The channel is buffered (workers×2)
// so the send is almost always non-blocking. In the rare case the buffer
// is full, workers will drain it without acquiring mu, so no deadlock.
func (p *Pool) Submit(task Task) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.shutdown {
		return ErrPoolShutdown
	}
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
	p.onComplete.Store(&fn)
}

// worker is the main loop for a single worker goroutine.
// After each task it checks workerQuit; if a signal is pending, it exits.
func (p *Pool) worker() {
	defer p.wg.Done()

	for task := range p.tasks {
		p.processTask(task)
		select {
		case <-p.workerQuit:
			return
		default:
		}
	}
}

// processTask resolves a provider, creates a runner, and executes the task.
// It uses task.ProviderKey for registry lookup when set, falling back to
// string(task.AgentType). If the selected chain has no healthy provider,
// it retries with the "default" chain before giving up.
func (p *Pool) processTask(task Task) {
	p.active.Add(1)
	defer p.active.Add(-1)

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
		if cbPtr := p.onComplete.Load(); cbPtr != nil {
			(*cbPtr)(TaskResult{
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
	start := time.Now()
	runErr := runner.Run(ctx, task)
	duration := time.Since(start)

	// Notify dispatcher of completion (success or failure).
	result := TaskResult{
		IssueNumber: task.Issue.Number,
		SessionID:   task.SessionID,
		AgentType:   task.AgentType,
		Success:     runErr == nil,
	}
	if runErr != nil {
		result.Error = runErr.Error()
		logger.Error("task failed",
			slog.String("error", runErr.Error()),
			slog.Duration("duration", duration.Truncate(time.Millisecond)),
		)
	} else {
		logger.Info("task done",
			slog.Duration("duration", duration.Truncate(time.Millisecond)),
		)
	}

	if cbPtr := p.onComplete.Load(); cbPtr != nil {
		(*cbPtr)(result)
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
