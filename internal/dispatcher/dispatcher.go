// Package dispatcher watches the issue tracker, routes tasks to agents, and manages dependencies.
package dispatcher

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/forge"
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
	tracker forge.IssueTracker
	policy  autonomy.AutonomyPolicy
	store   store.Store
	config  *Config
	claimed map[int]*claimedIssue
	mu      sync.Mutex
	logger  *log.Logger
	stop    context.CancelFunc
}

// New creates a Dispatcher with the given dependencies.
func New(tracker forge.IssueTracker, policy autonomy.AutonomyPolicy, st store.Store, cfg *Config) *Dispatcher {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Dispatcher{
		tracker: tracker,
		policy:  policy,
		store:   st,
		config:  cfg,
		claimed: make(map[int]*claimedIssue),
		logger:  log.Default(),
	}
}

// Run starts the watch loop and heartbeat ticker. It blocks until ctx is
// cancelled or an unrecoverable error occurs.
func (d *Dispatcher) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	d.stop = cancel

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
		case <-ticker.C:
			if err := d.checkTimeouts(ctx); err != nil {
				d.logger.Printf("heartbeat check error: %v", err)
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
	var err error
	switch ev.Type {
	case forge.EventIssueOpened:
		err = d.handleOpened(ctx, ev)
	case forge.EventIssueClosed:
		err = d.handleClosed(ctx, ev)
	case forge.EventIssueLabeled:
		err = d.handleLabeled(ctx, ev)
	case forge.EventIssueCommented:
		err = d.handleCommented(ctx, ev)
	case forge.EventIssueEdited:
		err = d.handleEdited(ctx, ev)
	default:
		d.logger.Printf("unknown event type: %s", ev.Type)
		return
	}
	if err != nil {
		d.logger.Printf("error handling %s for issue #%d: %v", ev.Type, ev.IssueNumber, err)
	}
}

// handleOpened processes a newly created issue: classify, check deps, route or block.
func (d *Dispatcher) handleOpened(ctx context.Context, ev forge.Event) error {
	issue := ev.Issue
	if issue == nil {
		var err error
		issue, err = d.tracker.GetIssue(ctx, ev.IssueNumber)
		if err != nil {
			return fmt.Errorf("get issue #%d: %w", ev.IssueNumber, err)
		}
	}

	agentType, err := d.classify(ctx, issue)
	if err != nil {
		return d.escalate(ctx, issue.Number, "invalid_frontmatter", err.Error())
	}

	// Check dependencies before routing.
	fm, fmErr := d.parseFrontmatter(issue)
	if fmErr != nil {
		return d.escalate(ctx, issue.Number, "frontmatter_parse_error", fmErr.Error())
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

	return d.route(ctx, issue, agentType)
}

// handleClosed processes a closed issue by unblocking dependents.
func (d *Dispatcher) handleClosed(ctx context.Context, ev forge.Event) error {
	// Remove from claimed map if tracked.
	d.mu.Lock()
	delete(d.claimed, ev.IssueNumber)
	d.mu.Unlock()

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
		comment := fmt.Sprintf(
			"ESCALATE [dispatcher] [%s]\ntrigger: dependency_cycle\ndetails: Cycle detected: %s\naction_needed: Break the dependency cycle.",
			time.Now().UTC().Format(time.RFC3339), cyclePath,
		)
		if err := d.tracker.AddLabel(ctx, num, "status:needs-human"); err != nil {
			d.logger.Printf("add needs-human to #%d: %v", num, err)
		}
		if _, err := d.tracker.AddComment(ctx, num, comment); err != nil {
			d.logger.Printf("add cycle comment to #%d: %v", num, err)
		}
	}
	return nil
}

// blockIssue transitions an issue to status:blocked with a comment listing blockers.
func (d *Dispatcher) blockIssue(ctx context.Context, issueNumber int, blockers []int) error {
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
