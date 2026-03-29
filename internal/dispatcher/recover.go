package dispatcher

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// RecoverOrphanedIssues scans all tracked repos for open issues with
// status:claimed or status:in-progress labels that are NOT in the live
// d.claimed map. On a fresh start the claimed map is empty, so any such
// issues are orphans from a previous dispatcher process that exited
// without cleanup (deploy, crash, SIGKILL).
//
// Each orphan is re-queued with a RECOVER comment for audit trail.
// This method runs synchronously on startup, before BackfillEditComments
// and before the dispatch loop, so that orphans are visible to pollQueued
// on the very first tick.
func (d *Dispatcher) RecoverOrphanedIssues(ctx context.Context) {
	d.logger.Info("recover: scanning tracked repos for orphaned issues")
	recovered := 0

	for i := range d.trackerEntries {
		entry := d.trackerEntries[i]
		issues, err := entry.Tracker.ListIssues(ctx, &forge.ListOptions{State: forge.StateOpen})
		if err != nil {
			d.logger.Warn("recover: list issues failed",
				zap.String("owner", entry.Owner),
				zap.String("repo", entry.Repo),
				zap.Error(err),
			)
			continue
		}

		for j := range issues {
			issue := issues[j]
			labels := make(map[string]bool, len(issue.Labels))
			for _, l := range issue.Labels {
				labels[l] = true
			}

			if !labels[models.LabelStatusClaimed] && !labels[models.LabelStatusInProgress] {
				continue
			}

			// Check if this issue is in d.claimed (won't be on fresh start).
			key := issueKey(entry.Owner, entry.Repo, issue.Number)
			d.mu.RLock()
			_, claimed := d.claimed[key]
			d.mu.RUnlock()
			if claimed {
				continue // actively being worked on by this process
			}

			// Orphan detected — re-queue.
			_ = entry.Tracker.RemoveLabel(ctx, issue.Number, models.LabelStatusClaimed)
			_ = entry.Tracker.RemoveLabel(ctx, issue.Number, models.LabelStatusInProgress)
			_ = entry.Tracker.AddLabel(ctx, issue.Number, models.LabelStatusQueued)

			comment := fmt.Sprintf(
				"RECOVER [dispatcher] [%s]\nOrphaned issue detected on startup (previous dispatcher process ended without cleanup). Re-queued for dispatch.",
				time.Now().UTC().Format(time.RFC3339),
			)
			_, _ = entry.Tracker.AddComment(ctx, issue.Number, comment)

			recovered++
			d.logger.Info("recover: re-queued orphaned issue",
				zap.String("owner", entry.Owner),
				zap.String("repo", entry.Repo),
				zap.Int("issue", issue.Number),
			)
		}
	}

	d.logger.Info("recover: complete", zap.Int("recovered", recovered))
}
