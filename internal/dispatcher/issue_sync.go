package dispatcher

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/store"
)

const (
	openSyncInterval   = 60 * time.Second
	closedSyncInterval = 5 * time.Minute
)

// runIssueCacheSync syncs issues from all registered trackers to the local
// SQLite cache. Open issues sync every 60s, closed issues every 5 minutes.
// An initial full sync (open + closed) runs immediately on startup.
func (d *Dispatcher) runIssueCacheSync(ctx context.Context) {
	if d.store == nil {
		d.logger.Warn("issue cache sync disabled: no store configured")
		return
	}

	// Initial full sync on startup.
	d.syncAllIssues(ctx, true)

	openTicker := time.NewTicker(openSyncInterval)
	closedTicker := time.NewTicker(closedSyncInterval)
	defer openTicker.Stop()
	defer closedTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-openTicker.C:
			d.syncAllIssues(ctx, false)
		case <-closedTicker.C:
			d.syncAllIssues(ctx, true)
		}
	}
}

// syncAllIssues syncs issues from every registered tracker.
// If includeClosed is true, closed issues are also fetched and synced.
func (d *Dispatcher) syncAllIssues(ctx context.Context, includeClosed bool) {
	d.mu.RLock()
	entries := make([]TrackerEntry, len(d.trackerEntries))
	copy(entries, d.trackerEntries)
	d.mu.RUnlock()

	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		d.syncTrackerIssues(ctx, entry, includeClosed)
	}
}

// syncTrackerIssues fetches and caches issues for a single tracker.
func (d *Dispatcher) syncTrackerIssues(ctx context.Context, entry TrackerEntry, includeClosed bool) {
	project := entry.Repo // project name is the repo name

	// Sync open issues (always).
	openIssues, err := d.fetchAllIssues(ctx, entry.Tracker, forge.StateOpen)
	if err != nil {
		d.logger.Warn("issue cache: failed to fetch open issues",
			zap.String("project", project),
			zap.Error(err),
		)
		return
	}

	cached := forgeIssuesToCached(openIssues)
	if err := d.store.SyncIssues(ctx, project, cached); err != nil {
		d.logger.Warn("issue cache: failed to sync open issues",
			zap.String("project", project),
			zap.Error(err),
		)
		return
	}

	// Prune stale open entries (issues that were closed or deleted since last sync).
	openNumbers := make([]int, len(openIssues))
	for i, iss := range openIssues {
		openNumbers[i] = iss.Number
	}
	if err := d.store.DeleteStaleIssues(ctx, project, openNumbers); err != nil {
		d.logger.Warn("issue cache: failed to prune stale issues",
			zap.String("project", project),
			zap.Error(err),
		)
	}

	// Sync closed issues (only when requested).
	if includeClosed {
		closedIssues, fetchErr := d.fetchAllIssues(ctx, entry.Tracker, forge.StateClosed)
		if fetchErr != nil {
			d.logger.Warn("issue cache: failed to fetch closed issues",
				zap.String("project", project),
				zap.Error(fetchErr),
			)
			return
		}

		closedCached := forgeIssuesToCached(closedIssues)
		if syncErr := d.store.SyncIssues(ctx, project, closedCached); syncErr != nil {
			d.logger.Warn("issue cache: failed to sync closed issues",
				zap.String("project", project),
				zap.Error(syncErr),
			)
		}
	}

	d.logger.Debug("issue cache synced",
		zap.String("project", project),
		zap.Int("open", len(openIssues)),
		zap.Bool("closed_synced", includeClosed),
	)
}

// fetchAllIssues paginates through all issues of the given state.
func (d *Dispatcher) fetchAllIssues(ctx context.Context, tracker forge.IssueTracker, state forge.State) ([]*forge.Issue, error) {
	var all []*forge.Issue
	page := 1
	for {
		batch, err := tracker.ListIssues(ctx, &forge.ListOptions{
			State:   state,
			Page:    page,
			PerPage: 50,
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
