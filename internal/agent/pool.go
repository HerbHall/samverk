// Package agent provides the agent runtime pool and runner for processing
// tasks as goroutines with session lifecycle management and cost recording.
package agent

import (
	"context"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/herbhall/samverk/internal/cost"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/metrics"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/internal/synapset"
	"github.com/herbhall/samverk/pkg/models"
)

// ErrPoolShutdown is returned when Submit is called on a shut-down pool.
var ErrPoolShutdown = errors.New("pool is shut down")

// defaultWorkers is the number of worker goroutines when none is specified.
const defaultWorkers = 3

// spawnStagger is the minimum interval between consecutive CLI process spawns.
// Prevents overwhelming the system when multiple workers start simultaneously.
const spawnStagger = 200 * time.Millisecond

// Task represents a unit of work to be processed by the agent pool.
type Task struct {
	Issue         *forge.Issue
	Tracker       forge.IssueTracker        // per-task tracker for comments/labels; when nil, pool falls back to its default
	Owner         string                    // repository owner for this task
	Repo          string                    // repository name for this task
	AgentType     models.AgentType
	SessionID     string
	ProviderKey   string                    // routing chain key; defaults to string(AgentType) when empty
	Timeout       time.Duration             // per-task timeout; 0 means no deadline
	HeartbeatFunc func()                    // called periodically while running; signals dispatcher that work is in progress; may be nil
	Frontmatter   *models.IssueFrontmatter  // parsed frontmatter from issue body; may be nil
}

// TaskResult reports the outcome of a pool task back to the dispatcher.
type TaskResult struct {
	IssueNumber int
	Owner       string // repository owner
	Repo        string // repository name
	SessionID   string
	AgentType   models.AgentType
	ProviderKey string // routing chain key used for this task
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
	logger     *zap.Logger
	done       chan struct{}
	mu         sync.Mutex
	shutdown   bool
	active     atomic.Int32                        // currently processing workers
	onComplete atomic.Pointer[func(TaskResult)]   // callback to notify dispatcher; atomic to avoid mu deadlock
	workerQuit chan struct{}                        // each send causes one worker to exit after its current task
	synapset   *synapset.Client                    // optional Synapset memory client; nil disables enrichment
	repoWriter forge.RepoWriter                     // optional; enables branch/file creation for agent PRs
	prManager  forge.PullRequestManager             // optional; enables PR creation for agent output
	repoDir    string                               // default git clone path; used when no per-project dir is set
	repoDirs   map[string]string                    // per-project repo dirs keyed by "owner/repo"
	fetchMu    sync.Mutex                           // serializes FetchLatest calls to avoid concurrent git index locks
	spawnMu    sync.Mutex                           // serializes CLI process spawns to avoid overwhelming the system
	lastSpawn  time.Time                            // timestamp of the last provider spawn for stagger enforcement
}

// NewPool creates a pool with the given number of worker goroutines and starts
// them immediately. If workers is <= 0, it defaults to 3. The default maximum
// is runtime.NumCPU(); call SetMaxWorkers to override before scaling.
func NewPool(registry *provider.Registry, tracker forge.IssueTracker, st store.Store, costs *cost.Tracker, workers int, logger *zap.Logger) *Pool {
	if workers <= 0 {
		workers = defaultWorkers
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	p := &Pool{
		registry:   registry,
		tracker:    tracker,
		store:      st,
		costs:      costs,
		workers:    workers,
		maxWorkers: runtime.NumCPU(),
		tasks:      make(chan Task, workers*2),
		logger:     logger,
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
	p.logger.Info("agent pool scaled up", zap.Int("added", n), zap.Int("workers", p.workers))
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
		p.logger.Info("agent pool scaled down", zap.Int("removed", sent), zap.Int("workers", p.Workers()))
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

	p.logger.Info("agent pool draining", zap.Int("queued", len(p.tasks)))
	close(p.tasks)
	p.wg.Wait()
	p.logger.Info("agent pool drained")
	close(p.done)
}

// Done returns a channel that is closed after Shutdown completes.
func (p *Pool) Done() <-chan struct{} {
	return p.done
}

// SetSynapset configures a Synapset memory client for all runners in the pool.
// When set, runners enrich prompts with relevant memories before provider calls
// and store task outcomes after completion.
func (p *Pool) SetSynapset(sc *synapset.Client) {
	p.synapset = sc
}

// SetRepoWriter configures the file writer used by runners to create branches
// and push file changes for agent-generated code.
func (p *Pool) SetRepoWriter(rw forge.RepoWriter) {
	p.repoWriter = rw
}

// SetPRManager configures the pull request manager used by runners to open PRs
// for agent-generated changes.
func (p *Pool) SetPRManager(pm forge.PullRequestManager) {
	p.prManager = pm
}

// SetRepoDir configures the default git repository path used to create
// isolated worktrees for agent sessions when no per-project dir is configured.
func (p *Pool) SetRepoDir(dir string) {
	p.repoDir = dir
}

// SetRepoDirFor configures the git repository path for a specific project.
// When set, tasks for this owner/repo use this path instead of the default.
func (p *Pool) SetRepoDirFor(owner, repo, dir string) {
	if p.repoDirs == nil {
		p.repoDirs = make(map[string]string)
	}
	p.repoDirs[owner+"/"+repo] = dir
}

// repoDirFor returns the repo directory for the given owner/repo, falling
// back to the default repoDir if no per-project directory is configured.
func (p *Pool) repoDirFor(owner, repo string) string {
	if p.repoDirs != nil {
		if dir, ok := p.repoDirs[owner+"/"+repo]; ok {
			return dir
		}
	}
	return p.repoDir
}

// fetchLatestFor pulls the latest code from origin for the given project.
// Serialized via fetchMu to prevent concurrent git operations on the same
// clone directory (git uses index locks that cause failures on concurrent access).
func (p *Pool) fetchLatestFor(owner, repo string) {
	dir := p.repoDirFor(owner, repo)
	if dir == "" {
		return
	}
	p.fetchMu.Lock()
	defer p.fetchMu.Unlock()
	FetchLatest(dir, p.logger)
}

// RegistryRouting returns the routing table from the underlying provider registry.
// Used by the dispatcher for pre-flight health gate checks.
func (p *Pool) RegistryRouting() map[string][]string {
	if p.registry == nil {
		return nil
	}
	return p.registry.Routing()
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
//
// When task.Timeout is set, the context is wrapped with a deadline so the
// provider call is cancelled if it exceeds the estimated duration.
func (p *Pool) processTask(task Task) {
	p.active.Add(1)
	defer p.active.Add(-1)

	// cleanupCtx is never cancelled by the task timeout so that session
	// status updates and failure comments can still reach the store/forge.
	cleanupCtx := context.Background()

	ctx := cleanupCtx
	if task.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, task.Timeout)
		defer cancel()
	}

	logger := p.logger.With(
		zap.String("session_id", task.SessionID),
		zap.Int("issue", task.Issue.Number),
		zap.String("agent_type", string(task.AgentType)),
		zap.Duration("timeout", task.Timeout),
	)

	routingKey := task.ProviderKey
	if routingKey == "" {
		routingKey = string(task.AgentType)
	}

	prov, model, err := p.registry.Get(ctx, routingKey)
	if err != nil && routingKey != "default" {
		// Selected provider chain is unhealthy; fall back to the default chain.
		logger.Warn("selected provider chain unhealthy, falling back to default",
			zap.String("provider_key", routingKey),
			zap.String("error", err.Error()),
		)
		prov, model, err = p.registry.Get(ctx, "default")
	}
	if err != nil {
		logger.Error("no healthy provider", zap.String("error", err.Error()))
		p.failSession(cleanupCtx, task.SessionID, fmt.Sprintf("no healthy provider: %v", err))
		// Notify dispatcher even on provider failure.
		if cbPtr := p.onComplete.Load(); cbPtr != nil {
			(*cbPtr)(TaskResult{
				IssueNumber: task.Issue.Number,
				Owner:       task.Owner,
				Repo:        task.Repo,
				SessionID:   task.SessionID,
				AgentType:   task.AgentType,
				ProviderKey: routingKey,
				Success:     false,
				Error:       err.Error(),
			})
		}
		return
	}

	// Resolve per-project repo directory for workspace isolation.
	taskRepoDir := p.repoDirFor(task.Owner, task.Repo)

	// Fetch latest code before creating a runner (serialized across workers).
	p.fetchLatestFor(task.Owner, task.Repo)

	// Stagger CLI spawns to avoid overwhelming the system when multiple
	// workers start tasks simultaneously.
	p.spawnMu.Lock()
	if since := time.Since(p.lastSpawn); since < spawnStagger {
		time.Sleep(spawnStagger - since)
	}
	p.lastSpawn = time.Now()
	p.spawnMu.Unlock()

	// Use per-task tracker when set; fall back to pool's default tracker.
	taskTracker := task.Tracker
	if taskTracker == nil {
		taskTracker = p.tracker
	}
	runner := NewRunner(prov, model, taskTracker, p.store, p.costs, p.logger)
	runner.cleanupCtx = cleanupCtx
	runner.synapset = p.synapset
	runner.SetRepoDir(taskRepoDir)
	if p.repoWriter != nil {
		runner.SetRepoWriter(p.repoWriter)
	}
	if p.prManager != nil {
		runner.SetPRManager(p.prManager)
	}
	start := time.Now()
	runErr := runner.Run(ctx, task)
	duration := time.Since(start)

	// Failover: if the runner failed with a retryable timeout error, try the
	// next provider in the routing chain (max 1 retry to avoid long loops).
	if runErr != nil && provider.IsRetryable(runErr) {
		provName := p.registry.NameOf(prov)
		logger.Warn("provider timeout, attempting failover to next in chain",
			zap.String("timed_out_provider", provName),
			zap.String("error", runErr.Error()),
			zap.Duration("duration", duration.Truncate(time.Millisecond)),
		)

		nextProv, nextModel, nextErr := p.registry.GetAfter(ctx, routingKey, provName)
		if nextErr == nil {
			// Post [FAILOVER] comment on the issue for visibility.
			failoverMsg := fmt.Sprintf("[FAILOVER] Provider %s timed out after %s. Retrying with %s.",
				provName, duration.Truncate(time.Second), nextProv.Name())
			if _, commentErr := taskTracker.AddComment(cleanupCtx, task.Issue.Number, failoverMsg); commentErr != nil {
				logger.Warn("failed to post failover comment",
					zap.Int("issue", task.Issue.Number),
					zap.Error(commentErr),
				)
			}

			// Re-stagger to avoid overwhelming the fallback provider.
			p.spawnMu.Lock()
			if since := time.Since(p.lastSpawn); since < spawnStagger {
				time.Sleep(spawnStagger - since)
			}
			p.lastSpawn = time.Now()
			p.spawnMu.Unlock()

			fallbackRunner := NewRunner(nextProv, nextModel, taskTracker, p.store, p.costs, p.logger)
			fallbackRunner.cleanupCtx = cleanupCtx
			fallbackRunner.synapset = p.synapset
			fallbackRunner.SetRepoDir(taskRepoDir)
			if p.repoWriter != nil {
				fallbackRunner.SetRepoWriter(p.repoWriter)
			}
			if p.prManager != nil {
				fallbackRunner.SetPRManager(p.prManager)
			}

			logger.Info("failover: retrying task with next provider",
				zap.String("provider", nextProv.Name()),
				zap.String("model", nextModel),
			)

			start = time.Now()
			runErr = fallbackRunner.Run(ctx, task)
			duration = time.Since(start)
		} else {
			logger.Warn("failover: no next provider available",
				zap.String("error", nextErr.Error()),
			)
			// Keep the original timeout error.
		}
	}

	// Notify dispatcher of completion (success or failure).
	result := TaskResult{
		IssueNumber: task.Issue.Number,
		Owner:       task.Owner,
		Repo:        task.Repo,
		SessionID:   task.SessionID,
		AgentType:   task.AgentType,
		ProviderKey: routingKey,
		Success:     runErr == nil,
	}
	if runErr != nil {
		result.Error = runErr.Error()
		logger.Error("task failed",
			zap.String("error", runErr.Error()),
			zap.Duration("duration", duration.Truncate(time.Millisecond)),
			zap.Duration("estimated", task.Timeout),
		)
	} else {
		logger.Info("task done",
			zap.Duration("duration", duration.Truncate(time.Millisecond)),
			zap.Duration("estimated", task.Timeout),
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
			zap.String("session_id", sessionID),
			zap.String("error", err.Error()),
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
			zap.String("session_id", sessionID),
			zap.String("error", err.Error()),
		)
	}
}
