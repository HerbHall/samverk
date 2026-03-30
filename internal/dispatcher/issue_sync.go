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
	// defaultIncrementalSyncInterval is the default interval for incremental issue sync.
	// Override via Config.IssueSyncInterval.
	defaultIncrementalSyncInterval = 60 * time.Second

	// defaultFullReconcileInterval is the default interval for full reconciliation.
	// Override via Config.IssueReconcileInterval.
	defaultFullReconcileInterval = 15 * time.Minute
)

// runIssueCacheSync syncs issues from all registered trackers to the local
// SQLite cache. Uses incremental sync (?since=lastSyncAt) every 60s for
// efficiency, with a full reconciliation every 15 minutes to catch any
// missed updates. An initial full sync runs immediately on startup.
func (d *Dispatcher) runIssueCacheSync(ctx context.Context) {
	if d.store == nil {
		d.logger.Warn("issue cache sync disabled: no store configured")
		return
	}

	// Track last sync time per tracker for incremental queries.
	var lastSyncMu sync.Mutex
	lastSyncTimes := make(map[string]time.Time) // key: tracker repo name

	// Initial full sync on startup (open + closed).
	d.syncAllIssuesFull(ctx, lastSyncTimes, &lastSyncMu)

	syncInterval := d.config.IssueSyncInterval
	if syncInterval == 0 {
		syncInterval = defaultIncrementalSyncInterval
	}
	reconcileInterval := d.config.IssueReconcileInterval
	if reconcileInterval == 0 {
		reconcileInterval = defaultFullReconcileInterval
	}

	incrementalTicker := time.NewTicker(syncInterval)
	reconcileTicker := time.NewTicker(reconcileInterval)
	defer incrementalTicker.Stop()
	defer reconcileTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-incrementalTicker.C:
			d.syncAllIssuesIncremental(ctx, lastSyncTimes, &lastSyncMu)
		case <-reconcileTicker.C:
			d.syncAllIssuesFull(ctx, lastSyncTimes, &lastSyncMu)
		}
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

// syncAllIssuesIncremental fetches only issues updated since the last sync.
// Falls back to full sync if no lastSyncTime is available.
func (d *Dispatcher) syncAllIssuesIncremental(ctx context.Context, lastSyncTimes map[string]time.Time, mu *sync.Mutex) {
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

		mu.Lock()
		lastSync, hasLastSync := lastSyncTimes[project]
		mu.Unlock()

		if !hasLastSync {
			// No previous sync -- do a full sync instead.
			d.syncAllIssuesFull(ctx, lastSyncTimes, mu)
			return
		}

		// Fetch only issues updated since lastSync.
		// Use a small buffer to handle clock skew between samverk and forge.
		since := lastSync.Add(-5 * time.Second)
		updated, err := d.fetchAllIssues(ctx, entry.Tracker, "", &since)
		if err != nil {
			d.logger.Warn("issue cache: incremental sync failed",
				zap.String("project", project), zap.Error(err))
			continue
		}

		if len(updated) > 0 {
			cached := forgeIssuesToCached(updated)
			if syncErr := d.store.SyncIssues(ctx, project, cached); syncErr != nil {
				d.logger.Warn("issue cache: incremental sync store failed",
					zap.String("project", project), zap.Error(syncErr))
				continue
			}
		}

		mu.Lock()
		lastSyncTimes[project] = now
		mu.Unlock()

		d.logger.Debug("issue cache: incremental sync",
			zap.String("project", project),
			zap.Int("updated", len(updated)),
			zap.Duration("since_last", now.Sub(lastSync)),
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
