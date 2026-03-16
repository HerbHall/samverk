// Package dispatcher watches the issue tracker, routes tasks to agents, and manages dependencies.
package dispatcher

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/agent"
	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/metrics"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/internal/store"
)

// claimedIssue tracks in-memory heartbeat state for a single routed issue.
type claimedIssue struct {
	AgentID       string
	Owner         string // repository owner
	Repo          string // repository name
	ClaimedAt     time.Time
	LastHeartbeat time.Time
	FailureCount  int
}

// TrackerEntry binds a forge.IssueTracker to its owner/repo identity.
// This avoids modifying the IssueTracker interface while supporting
// multiple repositories.
type TrackerEntry struct {
	Owner   string
	Repo    string
	Tracker forge.IssueTracker
}

// Dispatcher is the core routing engine. It watches the issue tracker for
// changes, classifies incoming work, resolves dependencies, and assigns
// tasks to agent pools.
type Dispatcher struct {
	trackers       map[string]forge.IssueTracker // key: strings.ToLower(owner+"/"+repo)
	trackerEntries []TrackerEntry                // original entries for iteration
	policy         autonomy.AutonomyPolicy
	store          store.Store
	pool           *agent.Pool
	healthMonitor  *provider.HealthMonitor
	decomposer     Decomposer
	projects       ProjectResolver
	config         *Config
	claimed        map[string]*claimedIssue  // key: issueKey(owner, repo, number)
	issueFailures  map[string]int            // key: issueKey(owner, repo, number)
	circuitBreaker *CircuitBreaker
	metrics        *metrics.DispatcherMetrics
	wakeup         chan struct{}
	lastEventTime  time.Time // updated by watcher callbacks; read by stale detection
	mu             sync.Mutex
	logger         *zap.Logger
	stop           context.CancelFunc
}

// issueKey returns a case-insensitive composite key for claimed/failure maps.
func issueKey(owner, repo string, number int) string {
	return fmt.Sprintf("%s/%s#%d", strings.ToLower(owner), strings.ToLower(repo), number)
}

// trackerKey returns the normalized map key for a tracker entry.
func trackerKey(owner, repo string) string {
	return strings.ToLower(owner + "/" + repo)
}

// trackerFor resolves the IssueTracker for a given owner/repo pair.
// Returns the first registered tracker if owner/repo is empty (backward compat).
func (d *Dispatcher) trackerFor(owner, repo string) forge.IssueTracker {
	if owner == "" && repo == "" && len(d.trackerEntries) > 0 {
		return d.trackerEntries[0].Tracker
	}
	if t, ok := d.trackers[trackerKey(owner, repo)]; ok {
		return t
	}
	// Fallback: return the first tracker if only one is registered.
	if len(d.trackerEntries) == 1 {
		return d.trackerEntries[0].Tracker
	}
	return nil
}

// New creates a Dispatcher with the given dependencies. The pool parameter
// is optional; when nil, routed issues are labeled but no agent tasks are spawned.
// Accepts a slice of TrackerEntry for multi-repo support. A single-element
// slice is equivalent to the previous single-tracker behavior.
func New(trackers []TrackerEntry, policy autonomy.AutonomyPolicy, st store.Store, pool *agent.Pool, cfg *Config, logger *zap.Logger) *Dispatcher {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	tm := make(map[string]forge.IssueTracker, len(trackers))
	for i := range trackers {
		tm[trackerKey(trackers[i].Owner, trackers[i].Repo)] = trackers[i].Tracker
	}
	return &Dispatcher{
		trackers:       tm,
		trackerEntries: trackers,
		policy:         policy,
		store:          st,
		pool:           pool,
		config:         cfg,
		claimed:        make(map[string]*claimedIssue),
		issueFailures:  make(map[string]int),
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

// SetHealthMonitor configures the provider health monitor for pre-flight
// health gating. When set, route() checks that at least one provider in
// the routing chain is healthy before claiming an issue.
func (d *Dispatcher) SetHealthMonitor(hm *provider.HealthMonitor) {
	d.healthMonitor = hm
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
	key := issueKey(result.Owner, result.Repo, result.IssueNumber)
	d.mu.Lock()
	delete(d.claimed, key)
	if result.Success {
		delete(d.issueFailures, key)
	}
	d.mu.Unlock()

	ctx := context.Background()
	tracker := d.trackerFor(result.Owner, result.Repo)
	if tracker == nil {
		d.logger.Error("no tracker for task result", zap.String("owner", result.Owner), zap.String("repo", result.Repo))
		return
	}

	_ = tracker.RemoveLabel(ctx, result.IssueNumber, "status:claimed")
	_ = tracker.RemoveLabel(ctx, result.IssueNumber, "status:in-progress")

	if result.Success {
		d.clearFailure(ctx, result.IssueNumber)
		if d.circuitBreaker != nil {
			d.circuitBreaker.RecordSuccess(result.ProviderKey)
		}

		// Post-completion quality gate: check output quality from session.
		// Don't escalate yet -- just log. Escalation to a higher-tier
		// provider will be added in #492 (multi-machine routing).
		d.checkCompletionQuality(ctx, result)

		if err := tracker.AddLabel(ctx, result.IssueNumber, "status:needs-qc"); err != nil {
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

// watcherRestartBackoff defines the exponential backoff sequence for watcher
// restarts: 1s, 2s, 4s, 8s, 16s, capped at maxWatcherBackoff.
const (
	initialWatcherBackoff = 1 * time.Second
	maxWatcherBackoff     = 5 * time.Minute
	// maxConsecutiveWatcherFailures is the number of rapid watcher failures
	// (within watcherFailureWindow) before Run() gives up.
	maxConsecutiveWatcherFailures = 5
	watcherFailureWindow          = 10 * time.Minute
	// stalePollMultiplier defines how many poll intervals with no events
	// triggers a stale-watcher warning log.
	stalePollMultiplier = 10
)

// watcherState tracks restart state for a single tracker watcher goroutine.
type watcherState struct {
	entry          TrackerEntry
	failures       []time.Time // timestamps of recent failures
	currentBackoff time.Duration
}

// Run starts the watch loop and heartbeat ticker. It blocks until ctx is
// cancelled or an unrecoverable error occurs. One watch goroutine is started
// per registered tracker. Watchers that fail are restarted with exponential
// backoff; Run() exits only after maxConsecutiveWatcherFailures within a
// short window.
func (d *Dispatcher) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	d.stop = cancel

	// Register completion callback with agent pool.
	if d.pool != nil {
		d.pool.SetOnComplete(d.handleTaskComplete)
	}

	type watcherError struct {
		idx int
		err error
	}
	errCh := make(chan watcherError, len(d.trackerEntries))

	// Build watcher state for each tracker.
	watchers := make([]watcherState, len(d.trackerEntries))
	for i := range d.trackerEntries {
		watchers[i] = watcherState{
			entry:          d.trackerEntries[i],
			currentBackoff: initialWatcherBackoff,
		}
	}

	// startWatcher launches a goroutine for the given tracker index.
	startWatcher := func(idx int) {
		entry := watchers[idx].entry
		go func() {
			err := entry.Tracker.Watch(ctx, func(ev forge.Event) {
				d.handleEvent(ctx, ev)
				d.mu.Lock()
				d.lastEventTime = time.Now()
				d.mu.Unlock()
			})
			errCh <- watcherError{idx: idx, err: err}
		}()
	}

	// Start one event watcher per registered tracker.
	d.mu.Lock()
	d.lastEventTime = time.Now()
	d.mu.Unlock()
	for i := range d.trackerEntries {
		startWatcher(i)
	}

	ticker := time.NewTicker(d.config.HeartbeatCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case we := <-errCh:
			// Context cancellation is a clean shutdown, not a failure.
			if ctx.Err() != nil {
				return ctx.Err()
			}

			ws := &watchers[we.idx]
			now := time.Now()

			// Prune failures outside the window.
			cutoff := now.Add(-watcherFailureWindow)
			recent := make([]time.Time, 0, len(ws.failures))
			for _, ft := range ws.failures {
				if ft.After(cutoff) {
					recent = append(recent, ft)
				}
			}
			recent = append(recent, now)
			ws.failures = recent

			if len(ws.failures) >= maxConsecutiveWatcherFailures {
				return fmt.Errorf("watcher %s/%s failed %d times in %v: %w",
					ws.entry.Owner, ws.entry.Repo,
					len(ws.failures), watcherFailureWindow, we.err)
			}

			d.logger.Warn("watcher failed, restarting with backoff",
				zap.String("owner", ws.entry.Owner),
				zap.String("repo", ws.entry.Repo),
				zap.Duration("backoff", ws.currentBackoff),
				zap.Int("recent_failures", len(ws.failures)),
				zap.Error(we.err),
			)

			backoff := ws.currentBackoff
			ws.currentBackoff *= 2
			if ws.currentBackoff > maxWatcherBackoff {
				ws.currentBackoff = maxWatcherBackoff
			}

			// Restart after backoff delay in a goroutine.
			go func(idx int, delay time.Duration) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
					startWatcher(idx)
				}
			}(we.idx, backoff)

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

			// Stale watcher detection: warn if no events for a long time.
			d.mu.Lock()
			lastEvt := d.lastEventTime
			d.mu.Unlock()
			staleThreshold := time.Duration(stalePollMultiplier) * d.config.HeartbeatCheckInterval
			if time.Since(lastEvt) > staleThreshold {
				d.logger.Warn("no events received recently, watchers may be stale",
					zap.Duration("since_last_event", time.Since(lastEvt)),
					zap.Duration("threshold", staleThreshold),
				)
			}
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

	tracker := d.trackerFor(ev.Owner, ev.Repo)
	if tracker == nil {
		return fmt.Errorf("no tracker for %s/%s", ev.Owner, ev.Repo)
	}

	issue := ev.Issue
	if issue == nil {
		var err error
		issue, err = tracker.GetIssue(ctx, ev.IssueNumber)
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

	agentType, fm, err := d.classify(ctx, ev.Owner, ev.Repo, issue)
	if err != nil {
		d.recordFailure(ctx, issue.Number, "", "", "", err.Error(), 0)
		return d.escalate(ctx, ev.Owner, ev.Repo, issue.Number, "invalid_frontmatter", err.Error())
	}

	if fm != nil && len(fm.DependsOn) > 0 {
		// Check for cycles first.
		cycle, cycleErr := d.detectCycle(ctx, ev.Owner, ev.Repo, issue.Number)
		if cycleErr != nil {
			return fmt.Errorf("cycle detection for #%d: %w", issue.Number, cycleErr)
		}
		if len(cycle) > 0 {
			return d.escalateCycle(ctx, ev.Owner, ev.Repo, cycle)
		}

		blocked, blockers, depErr := d.checkDependencies(ctx, ev.Owner, ev.Repo, fm)
		if depErr != nil {
			return fmt.Errorf("check deps for #%d: %w", issue.Number, depErr)
		}
		if blocked {
			return d.blockIssue(ctx, ev.Owner, ev.Repo, issue.Number, blockers)
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

	decomposed, decErr := d.decomposeAndCreateChildren(ctx, ev.Owner, ev.Repo, issue, fm, agentType, timeout)
	if decErr != nil {
		d.recordFailure(ctx, issue.Number, "", string(agentType), "", decErr.Error(), 0)
		return fmt.Errorf("decompose #%d: %w", issue.Number, decErr)
	}
	if decomposed {
		return nil // parent is now blocked on children; don't route it
	}

	return d.route(ctx, ev.Owner, ev.Repo, issue, agentType, fm)
}

// handleClosed processes a closed issue by unblocking dependents.
func (d *Dispatcher) handleClosed(ctx context.Context, ev forge.Event) error {
	// Remove from claimed map and clear both in-memory and persisted failure counts.
	key := issueKey(ev.Owner, ev.Repo, ev.IssueNumber)
	d.mu.Lock()
	delete(d.claimed, key)
	delete(d.issueFailures, key)
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

	key := issueKey(ev.Owner, ev.Repo, ev.IssueNumber)
	claimed, ok := d.claimed[key]
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
func (d *Dispatcher) escalate(ctx context.Context, owner, repo string, issueNumber int, trigger, details string) error {
	tracker := d.trackerFor(owner, repo)
	if tracker == nil {
		return fmt.Errorf("no tracker for %s/%s", owner, repo)
	}
	comment := fmt.Sprintf(
		"ESCALATE [dispatcher] [%s]\ntrigger: %s\ndetails: %s\naction_needed: Manual review required.",
		time.Now().UTC().Format(time.RFC3339), trigger, details,
	)
	if err := tracker.AddLabel(ctx, issueNumber, "status:needs-human"); err != nil {
		return fmt.Errorf("add needs-human label: %w", err)
	}
	if _, err := tracker.AddComment(ctx, issueNumber, comment); err != nil {
		return fmt.Errorf("add escalation comment: %w", err)
	}
	return nil
}

// escalateCycle labels all issues in a dependency cycle as needs-human.
// Cycle detection is per-repo, so all issues share the triggering repo's identity.
func (d *Dispatcher) escalateCycle(ctx context.Context, owner, repo string, cycle []int) error {
	tracker := d.trackerFor(owner, repo)
	if tracker == nil {
		return fmt.Errorf("no tracker for %s/%s", owner, repo)
	}
	cyclePath := fmt.Sprintf("%v", cycle)
	for _, num := range cycle {
		errMsg := fmt.Sprintf("dependency_cycle: %s", cyclePath)
		d.recordFailure(ctx, num, "", "", "", errMsg, 0)

		comment := fmt.Sprintf(
			"ESCALATE [dispatcher] [%s]\ntrigger: dependency_cycle\ndetails: Cycle detected: %s\naction_needed: Break the dependency cycle.",
			time.Now().UTC().Format(time.RFC3339), cyclePath,
		)
		if err := tracker.AddLabel(ctx, num, "status:needs-human"); err != nil {
			d.logger.Error("add label", zap.Int("issue", num), zap.String("label", "needs-human"), zap.String("error", err.Error()))
		}
		if _, err := tracker.AddComment(ctx, num, comment); err != nil {
			d.logger.Error("add comment", zap.Int("issue", num), zap.String("context", "cycle"), zap.String("error", err.Error()))
		}
	}
	return nil
}

// blockIssue transitions an issue to status:blocked with a comment listing blockers.
func (d *Dispatcher) blockIssue(ctx context.Context, owner, repo string, issueNumber int, blockers []string) error {
	tracker := d.trackerFor(owner, repo)
	if tracker == nil {
		return fmt.Errorf("no tracker for %s/%s", owner, repo)
	}
	comment := fmt.Sprintf(
		"BLOCKED [dispatcher] [%s]\nWaiting on: %v\nWill auto-unblock when all dependencies reach status:done.",
		time.Now().UTC().Format(time.RFC3339), blockers,
	)
	if err := tracker.AddLabel(ctx, issueNumber, "status:blocked"); err != nil {
		return fmt.Errorf("add blocked label: %w", err)
	}
	if _, err := tracker.AddComment(ctx, issueNumber, comment); err != nil {
		return fmt.Errorf("add block comment: %w", err)
	}
	return nil
}
