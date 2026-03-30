package dispatcher

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/store"
)

const (
	// defaultFullReconcileInterval is the default interval for full reconciliation.
	// Override via Config.IssueReconcileInterval. Between reconciliations, the
	// cache is kept current by event-driven updates from the Watch loop.
	defaultFullReconcileInterval = 15 * time.Minute
)

// runIssueCacheSync syncs issues from all registered trackers to the local
// SQLite cache. The initial full sync runs on startup. After that, the cache
// is kept current by event-driven updates from the Watch loop (via
// updateCacheFromEvent). Periodic full reconciliation catches any events that
// were missed or handles closed issues not covered by events.
func (d *Dispatcher) runIssueCacheSync(ctx context.Context) {
	if d.store == nil {
		d.logger.Warn("issue cache sync disabled: no store configured")
		return
	}

	// Track last sync time per tracker for reconciliation.
	var lastSyncMu sync.Mutex
	lastSyncTimes := make(map[string]time.Time)

	// Initial full sync on startup (open + closed).
	d.syncAllIssuesFull(ctx, lastSyncTimes, &lastSyncMu)

	reconcileInterval := d.config.IssueReconcileInterval
	if reconcileInterval == 0 {
		reconcileInterval = defaultFullReconcileInterval
	}

	reconcileTicker := time.NewTicker(reconcileInterval)
	defer reconcileTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-reconcileTicker.C:
			d.syncAllIssuesFull(ctx, lastSyncTimes, &lastSyncMu)
		}
	}
}

// updateCacheFromEvent writes event issue data to the SQLite cache.
// Called from handleEvent on each Watch event so the cache stays current
// without a separate incremental sync goroutine.
func (d *Dispatcher) updateCacheFromEvent(ctx context.Context, ev forge.Event) {
	if d.store == nil || ev.Issue == nil {
		return
	}

	project := ev.Repo
	if project == "" && ev.Owner != "" {
		project = ev.Repo
	}
	// Determine project from tracker entries if not set on event.
	if project == "" {
		d.mu.RLock()
		for _, entry := range d.trackerEntries {
			project = entry.Repo
			break
		}
		d.mu.RUnlock()
	}
	if project == "" {
		return
	}

	cached := forgeIssuesToCached([]*forge.Issue{ev.Issue})
	if err := d.store.SyncIssues(ctx, project, cached); err != nil {
		d.logger.Debug("issue cache: event update failed",
			zap.Int("issue", ev.IssueNumber),
			zap.String("event", string(ev.Type)),
			zap.Error(err))
	}
}

// syncAllIssuesFull does a complete sync of all open (and closed) issues
// from every registered tracker. Updates lastSyncTimes on success.
func (d *Dispatcher) syncAllIssuesFull(ctx context.Context, lastSyncTimes map[string]time.Time, mu *sync.Mutex) {
	d.mu.RLock()
	entries := make([]TrackerEntry, len(d.trackerEntries))
	copy(entries, d.trackerEntries)
	d.mu.RUnlock()

	now := time.Now().UTC()

	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		project := entry.Repo

		// Fetch all open issues.
		openIssues, err := d.fetchAllIssues(ctx, entry.Tracker, forge.StateOpen, nil)
		if err != nil {
			d.logger.Warn("issue cache: full sync failed (open)",
				zap.String("project", project), zap.Error(err))
			continue
		}

		cached := forgeIssuesToCached(openIssues)
		if syncErr := d.store.SyncIssues(ctx, project, cached); syncErr != nil {
			d.logger.Warn("issue cache: full sync store failed",
				zap.String("project", project), zap.Error(syncErr))
			continue
		}

		// Prune stale entries.
		openNumbers := make([]int, len(openIssues))
		for i, iss := range openIssues {
			openNumbers[i] = iss.Number
		}
		if pruneErr := d.store.DeleteStaleIssues(ctx, project, openNumbers); pruneErr != nil {
			d.logger.Warn("issue cache: prune failed",
				zap.String("project", project), zap.Error(pruneErr))
		}

		// Fetch closed issues too (full reconciliation).
		closedIssues, closedErr := d.fetchAllIssues(ctx, entry.Tracker, forge.StateClosed, nil)
		if closedErr != nil {
			d.logger.Warn("issue cache: full sync failed (closed)",
				zap.String("project", project), zap.Error(closedErr))
		} else {
			closedCached := forgeIssuesToCached(closedIssues)
			if syncErr := d.store.SyncIssues(ctx, project, closedCached); syncErr != nil {
				d.logger.Warn("issue cache: full sync store failed (closed)",
					zap.String("project", project), zap.Error(syncErr))
			}
		}

		mu.Lock()
		lastSyncTimes[project] = now
		mu.Unlock()

		d.logger.Debug("issue cache: full reconciliation",
			zap.String("project", project),
			zap.Int("open", len(openIssues)),
			zap.Int("closed", len(closedIssues)),
		)
	}
}

// fetchAllIssues paginates through all issues of the given state.
// If since is non-nil, only issues updated after that time are returned.
// If state is empty, issues of all states are returned (for incremental sync).
func (d *Dispatcher) fetchAllIssues(ctx context.Context, tracker forge.IssueTracker, state forge.State, since *time.Time) ([]*forge.Issue, error) {
	var all []*forge.Issue
	page := 1
	for {
		batch, err := tracker.ListIssues(ctx, &forge.ListOptions{
			State:   state,
			Page:    page,
			PerPage: 50,
			Since:   since,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 50 {
			break
		}
		page++
	}
	return all, nil
}

// forgeIssuesToCached converts forge issues to the store's CachedIssue type.
func forgeIssuesToCached(issues []*forge.Issue) []store.CachedIssue {
	result := make([]store.CachedIssue, len(issues))
	for i, iss := range issues {
		labels := iss.Labels
		if labels == nil {
			labels = []string{}
		}
		assignees := iss.Assignees
		if assignees == nil {
			assignees = []string{}
		}
		result[i] = store.CachedIssue{
			Number:    iss.Number,
			Title:     iss.Title,
			Body:      iss.Body,
			State:     string(iss.State),
			Labels:    labels,
			Assignees: assignees,
			CreatedAt: iss.CreatedAt,
			UpdatedAt: iss.UpdatedAt,
			ClosedAt:  iss.ClosedAt,
		}
	}
	return result
}
