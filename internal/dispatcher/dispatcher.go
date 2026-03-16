// Package dispatcher watches the issue tracker, routes tasks to agents, and manages dependencies.
package dispatcher

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"sync"
	"time"

	"github.com/herbhall/samverk/internal/agent"
	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/metrics"
	"github.com/herbhall/samverk/internal/store"
)

// claimedIssue tracks in-memory heartbeat state for a single routed issue.
type claimedIssue struct {
	AgentID       string
	ClaimedAt     time.Time
	LastHeartbeat time.Time
	FailureCount  int
}

// Dispatcher is the core routing engine. It watches the issue tracker for
// changes, classifies incoming work, resolves dependencies, and assigns
// tasks to agent pools.
type Dispatcher struct {
	tracker        forge.IssueTracker
	policy         autonomy.AutonomyPolicy
	store          store.Store
	pool           *agent.Pool
	decomposer     Decomposer
	projects       ProjectResolver
	config         *Config
	claimed        map[int]*claimedIssue
	issueFailures  map[int]int // in-memory fallback; persisted counter is authoritative when store is set
	circuitBreaker *CircuitBreaker
	metrics        *metrics.DispatcherMetrics
	wakeup         chan struct{}
	mu             sync.Mutex
	logger         *zap.Logger
	stop           context.CancelFunc
}

// New creates a Dispatcher with the given dependencies. The pool parameter
// is optional; when nil, routed issues are labeled but no agent tasks are spawned.
func New(tracker forge.IssueTracker, policy autonomy.AutonomyPolicy, st store.Store, pool *agent.Pool, cfg *Config, logger *zap.Logger) *Dispatcher {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Dispatcher{
		tracker:        tracker,
		policy:         policy,
		store:          st,
		pool:           pool,
		config:         cfg,
		claimed:        make(map[int]*claimedIssue),
		issueFailures:  make(map[int]int),
		circuitBreaker: NewCircuitBreaker(logger),
		metrics:        metrics.NewDispatcherMetrics(),
		wakeup:         make(chan struct{}, 1),
		logger:         logger,
	}
}

// SetDecomposer configures the optional issue decomposer. When set and the
// decomposition threshold is exceeded, oversized issues are split into
// child issues before routing.
func (d *Dispatcher) SetDecomposer(dec Decomposer) {
	d.decomposer = dec
}

// SetProjectResolver configures the cross-project dependency resolver.
// When set, the dispatcher can resolve depends_on references to issues
// in other registered projects (e.g., "owner/repo#42").
func (d *Dispatcher) SetProjectResolver(pr ProjectResolver) {
	d.projects = pr
}

// Snapshot returns a point-in-time snapshot of dispatcher metrics.
func (d *Dispatcher) Snapshot() metrics.DispatcherSnapshot {
	return d.metrics.Snapshot()
}

// handleTaskComplete is called by the agent pool when a task finishes.
// It removes the issue from the claimed map and updates labels.
// On failure, it records a FailureEvent and checks the persisted failure
// count for escalation — the counter survives restarts.
func (d *Dispatcher) handleTaskComplete(result agent.TaskResult) {
	d.metrics.IssueUnclaimed()
	d.mu.Lock()
	delete(d.claimed, result.IssueNumber)
	if result.Success {
		delete(d.issueFailures, result.IssueNumber)
	}
	d.mu.Unlock()

	ctx := context.Background()

	_ = d.tracker.RemoveLabel(ctx, result.IssueNumber, "status:claimed")
	_ = d.tracker.RemoveLabel(ctx, result.IssueNumber, "status:in-progress")

	if result.Success {
		d.clearFailure(ctx, result.IssueNumber)
		if d.circuitBreaker != nil {
			d.circuitBreaker.RecordSuccess(result.ProviderKey)
		}
		if err := d.tracker.AddLabel(ctx, result.IssueNumber, "status:needs-qc"); err != nil {
			d.logger.Error("add label", zap.Int("issue", result.IssueNumber), zap.String("label", "needs-qc"), zap.String("error", err.Error()))
		}
		d.logger.Info("task completed", zap.Int("issue", result.IssueNumber), zap.String("session", result.SessionID))
	} else {
		// Record failure event with classification.
		d.recordFailure(ctx, result.IssueNumber, result.SessionID,
			string(result.AgentType), result.ProviderKey, result.Error, 0)

		// Use correction engine to decide response instead of blind re-queue.
		fc := classifyFailure(result.Error)
		attempt := d.getPersistedFailureCount(ctx, result.IssueNumber)
		decision := decideCorrection(fc, result.IssueNumber, attempt, 0)
		d.applyCorrection(ctx, result, decision)
	}

	// Signal the run loop to immediately check for queued work
	// instead of waiting for the next tick.
	select {
	case d.wakeup <- struct{}{}:
	default: // already signaled, don't block
	}
}

// Run starts the watch loop and heartbeat ticker. It blocks until ctx is
// cancelled or an unrecoverable error occurs.
func (d *Dispatcher) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	d.stop = cancel

	// Register completion callback with agent pool.
	if d.pool != nil {
		d.pool.SetOnComplete(d.handleTaskComplete)
	}

	errCh := make(chan error, 1)

	// Start the event watcher in a goroutine.
	go func() {
		errCh <- d.tracker.Watch(ctx, func(ev forge.Event) {
			d.handleEvent(ctx, ev)
		})
	}()

	ticker := time.NewTicker(d.config.HeartbeatCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return fmt.Errorf("watch stopped: %w", err)
		case <-d.wakeup:
			d.logger.Debug("wakeup: task completed, checking for queued work")
			pollStart := time.Now()
			if err := d.checkTimeouts(ctx); err != nil {
				d.logger.Error("heartbeat check", zap.String("error", err.Error()))
			}
			if err := d.recheckCrossProjectDeps(ctx); err != nil {
				d.logger.Error("cross-project dep recheck", zap.String("error", err.Error()))
			}
			d.metrics.PollCompleted(time.Since(pollStart))
		case <-ticker.C:
			pollStart := time.Now()
			if err := d.checkTimeouts(ctx); err != nil {
				d.logger.Error("heartbeat check", zap.String("error", err.Error()))
			}
			// Periodically re-evaluate cross-project dependencies for blocked issues.
			if err := d.recheckCrossProjectDeps(ctx); err != nil {
				d.logger.Error("cross-project dep recheck", zap.String("error", err.Error()))
			}
			d.metrics.PollCompleted(time.Since(pollStart))
		}
	}
}

// Stop cancels the dispatcher's context, terminating Run.
func (d *Dispatcher) Stop() {
	if d.stop != nil {
		d.stop()
	}
}

// handleEvent dispatches a forge event to the correct handler.
func (d *Dispatcher) handleEvent(ctx context.Context, ev forge.Event) {
	d.metrics.EventProcessed()
	var err error
	switch ev.Type {
	case forge.EventIssueOpened:
		err = d.handleOpened(ctx, ev)
	case forge.EventIssueClosed:
		err = d.handleClosed(ctx, ev)
	case forge.EventIssueLabeled:
		err = d.handleLabeled(ctx, ev)
	case forge.EventIssueAssigned:
		err = d.handleAssigned(ctx, ev)
	case forge.EventIssueCommented:
		err = d.handleCommented(ctx, ev)
	case forge.EventIssueEdited:
		err = d.handleEdited(ctx, ev)
	default:
		d.logger.Warn("unknown event type", zap.String("type", string(ev.Type)))
		return
	}
	if err != nil {
		d.logger.Error("event handler", zap.String("event", string(ev.Type)), zap.Int("issue", ev.IssueNumber), zap.String("error", err.Error()))
	}
}

// handleOpened processes a newly created issue: classify, check deps, route or block.
// Pull requests are silently skipped — they are not routable work items.
func (d *Dispatcher) handleOpened(ctx context.Context, ev forge.Event) error {
	if ev.IsPullRequest {
		d.logger.Debug("skipping pull request", zap.Int("issue", ev.IssueNumber))
		return nil
	}

	issue := ev.Issue
	if issue == nil {
		var err error
		issue, err = d.tracker.GetIssue(ctx, ev.IssueNumber)
		if err != nil {
			return fmt.Errorf("get issue #%d: %w", ev.IssueNumber, err)
		}
	}

	// Skip issues that are already in a terminal or human-managed state.
	labels := make(map[string]bool, len(issue.Labels))
	for _, l := range issue.Labels {
		labels[l] = true
	}
	if labels["status:needs-human"] || labels["status:human-pending"] || labels["status:blocked"] || labels["status:claimed"] || labels["status:in-progress"] {
		d.logger.Debug("skipping issue with terminal status", zap.Int("issue", issue.Number))
		return nil
	}

	agentType, fm, err := d.classify(ctx, issue)
	if err != nil {
		d.recordFailure(ctx, issue.Number, "", "", "", err.Error(), 0)
		return d.escalate(ctx, issue.Number, "invalid_frontmatter", err.Error())
	}

	if fm != nil && len(fm.DependsOn) > 0 {
		// Check for cycles first.
		cycle, cycleErr := d.detectCycle(ctx, issue.Number)
		if cycleErr != nil {
			return fmt.Errorf("cycle detection for #%d: %w", issue.Number, cycleErr)
		}
		if len(cycle) > 0 {
			return d.escalateCycle(ctx, cycle)
		}

		blocked, blockers, depErr := d.checkDependencies(ctx, fm)
		if depErr != nil {
			return fmt.Errorf("check deps for #%d: %w", issue.Number, depErr)
		}
		if blocked {
			return d.blockIssue(ctx, issue.Number, blockers)
		}
	}

	// Pre-flight decomposition: if the estimated timeout exceeds the
	// configured threshold and this is not already a child issue, break
	// it into smaller sub-tasks before routing.
	providerKey, _ := selectProviderKey(issue, agentType)
	var timeout time.Duration
	if d.store != nil {
		timeout = CalibratedTimeout(ctx, d.store, d.logger, issue, fm, agentType, providerKey)
	} else {
		timeout = EstimateTimeout(issue, fm, agentType, providerKey)
	}

	decomposed, decErr := d.decomposeAndCreateChildren(ctx, issue, fm, agentType, timeout)
	if decErr != nil {
		d.recordFailure(ctx, issue.Number, "", string(agentType), "", decErr.Error(), 0)
		return fmt.Errorf("decompose #%d: %w", issue.Number, decErr)
	}
	if decomposed {
		return nil // parent is now blocked on children; don't route it
	}

	return d.route(ctx, issue, agentType, fm)
}

// handleClosed processes a closed issue by unblocking dependents.
func (d *Dispatcher) handleClosed(ctx context.Context, ev forge.Event) error {
	// Remove from claimed map and clear both in-memory and persisted failure counts.
	d.mu.Lock()
	delete(d.claimed, ev.IssueNumber)
	delete(d.issueFailures, ev.IssueNumber)
	d.mu.Unlock()

	d.clearFailure(ctx, ev.IssueNumber)

	return d.unblockDependents(ctx, ev.IssueNumber)
}

// handleLabeled re-evaluates routing when a status label changes.
func (d *Dispatcher) handleLabeled(_ context.Context, _ forge.Event) error {
	// Phase 1: no re-evaluation on label changes beyond what handleOpened covers.
	return nil
}

// handleCommented checks for heartbeat comments from agents.
func (d *Dispatcher) handleCommented(_ context.Context, ev forge.Event) error {
	if ev.Comment == nil {
		return nil
	}
	hb := parseHeartbeat(ev.Comment.Body)
	if hb == nil {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	claimed, ok := d.claimed[ev.IssueNumber]
	if !ok {
		return nil
	}
	claimed.LastHeartbeat = hb.Timestamp
	return nil
}

// handleAssigned logs assignment events. If the assignee matches a known agent
// type pattern, it could trigger routing in a future iteration.
func (d *Dispatcher) handleAssigned(_ context.Context, ev forge.Event) error {
	assignee := ev.Assignee
	if assignee == "" && ev.Issue != nil && len(ev.Issue.Assignees) > 0 {
		assignee = ev.Issue.Assignees[len(ev.Issue.Assignees)-1]
	}
	d.logger.Info("issue assigned", zap.Int("issue", ev.IssueNumber), zap.String("assignee", assignee))
	return nil
}

// handleEdited re-parses frontmatter when an issue body changes.
func (d *Dispatcher) handleEdited(_ context.Context, _ forge.Event) error {
	// Phase 1: no re-parsing on edits.
	return nil
}

// escalate labels an issue as needs-human and posts a comment.
func (d *Dispatcher) escalate(ctx context.Context, issueNumber int, trigger, details string) error {
	comment := fmt.Sprintf(
		"ESCALATE [dispatcher] [%s]\ntrigger: %s\ndetails: %s\naction_needed: Manual review required.",
		time.Now().UTC().Format(time.RFC3339), trigger, details,
	)
	if err := d.tracker.AddLabel(ctx, issueNumber, "status:needs-human"); err != nil {
		return fmt.Errorf("add needs-human label: %w", err)
	}
	if _, err := d.tracker.AddComment(ctx, issueNumber, comment); err != nil {
		return fmt.Errorf("add escalation comment: %w", err)
	}
	return nil
}

// escalateCycle labels all issues in a dependency cycle as needs-human.
func (d *Dispatcher) escalateCycle(ctx context.Context, cycle []int) error {
	cyclePath := fmt.Sprintf("%v", cycle)
	for _, num := range cycle {
		errMsg := fmt.Sprintf("dependency_cycle: %s", cyclePath)
		d.recordFailure(ctx, num, "", "", "", errMsg, 0)

		comment := fmt.Sprintf(
			"ESCALATE [dispatcher] [%s]\ntrigger: dependency_cycle\ndetails: Cycle detected: %s\naction_needed: Break the dependency cycle.",
			time.Now().UTC().Format(time.RFC3339), cyclePath,
		)
		if err := d.tracker.AddLabel(ctx, num, "status:needs-human"); err != nil {
			d.logger.Error("add label", zap.Int("issue", num), zap.String("label", "needs-human"), zap.String("error", err.Error()))
		}
		if _, err := d.tracker.AddComment(ctx, num, comment); err != nil {
			d.logger.Error("add comment", zap.Int("issue", num), zap.String("context", "cycle"), zap.String("error", err.Error()))
		}
	}
	return nil
}

// blockIssue transitions an issue to status:blocked with a comment listing blockers.
func (d *Dispatcher) blockIssue(ctx context.Context, issueNumber int, blockers []string) error {
	comment := fmt.Sprintf(
		"BLOCKED [dispatcher] [%s]\nWaiting on: %v\nWill auto-unblock when all dependencies reach status:done.",
		time.Now().UTC().Format(time.RFC3339), blockers,
	)
	if err := d.tracker.AddLabel(ctx, issueNumber, "status:blocked"); err != nil {
		return fmt.Errorf("add blocked label: %w", err)
	}
	if _, err := d.tracker.AddComment(ctx, issueNumber, comment); err != nil {
		return fmt.Errorf("add block comment: %w", err)
	}
	return nil
}
