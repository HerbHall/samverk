package api

import (
	"net/http"
	"time"

	"github.com/herbhall/samverk/internal/store"
)

// issueSummaryResponse is the JSON response for GET /api/v1/issues/summary.
type issueSummaryResponse struct {
	Projects    map[string]store.IssueCounts `json:"projects"`
	Aggregate   store.IssueCounts            `json:"aggregate"`
	LabelCounts map[string]int               `json:"label_counts"`
	SyncedAt    string                       `json:"synced_at"`
}

// handleIssueSummary returns accurate issue counts from the local SQLite cache.
func (a *API) handleIssueSummary(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}

	ctx := r.Context()

	counts, err := a.store.GetAllIssueCounts(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get issue counts")
		return
	}

	// Aggregate across all projects.
	var agg store.IssueCounts
	for _, c := range counts {
		agg.Open += c.Open
		agg.Closed += c.Closed
	}
	agg.Total = agg.Open + agg.Closed

	// Aggregate label counts across all projects.
	allLabels := make(map[string]int)
	for project := range counts {
		labels, labelErr := a.store.GetIssueLabelCounts(ctx, project)
		if labelErr != nil {
			continue
		}
		for label, count := range labels {
			allLabels[label] += count
		}
	}

	// Find the most recent sync time across all projects.
	var latestSync time.Time
	for project := range counts {
		syncTime, syncErr := a.store.GetCacheSyncTime(ctx, project)
		if syncErr != nil {
			continue
		}
		if syncTime.After(latestSync) {
			latestSync = syncTime
		}
	}

	syncedAt := ""
	if !latestSync.IsZero() {
		syncedAt = latestSync.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, issueSummaryResponse{
		Projects:    counts,
		Aggregate:   agg,
		LabelCounts: allLabels,
		SyncedAt:    syncedAt,
	})
}
