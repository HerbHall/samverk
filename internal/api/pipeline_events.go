package api

import (
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/pkg/models"
)

// pipelineEventDTO is the JSON shape returned by GET /api/v1/pipeline/events.
type pipelineEventDTO struct {
	ID          int64     `json:"id"`
	IssueNumber int       `json:"issue_number"`
	Project     string    `json:"project"`
	FromStage   string    `json:"from_stage"`
	ToStage     string    `json:"to_stage"`
	TriggeredBy string    `json:"triggered_by"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// throughputDTO is the JSON shape returned by GET /api/v1/pipeline/throughput.
type throughputDTO struct {
	IssuesCompletedPerHour24h float64   `json:"issues_completed_per_hour_24h"`
	AvgCycleTimeHours         float64   `json:"avg_cycle_time_hours"`
	GeneratedAt               time.Time `json:"generated_at"`
}

// handlePipelineEvents returns pipeline stage transition events.
//
// Query parameters:
//   - issue: filter by issue number (optional; 0 = all issues)
//   - since: RFC3339 lower bound for occurred_at (default: 24 h ago)
//   - limit: max rows to return (default: 50)
func (a *API) handlePipelineEvents(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not configured")
		return
	}

	issueNumber := 0
	if v := r.URL.Query().Get("issue"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			issueNumber = n
		}
	}

	since := time.Now().Add(-24 * time.Hour)
	if v := r.URL.Query().Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			since = t
		}
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	events, err := a.store.GetPipelineEvents(r.Context(), issueNumber, since, limit)
	if err != nil {
		a.logger.Error("get pipeline events", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	dtos := make([]pipelineEventDTO, len(events))
	for i := range events {
		dtos[i] = pipelineEventDTO{
			ID:          events[i].ID,
			IssueNumber: events[i].IssueNumber,
			Project:     events[i].Project,
			FromStage:   events[i].FromStage,
			ToStage:     events[i].ToStage,
			TriggeredBy: events[i].TriggeredBy,
			OccurredAt:  events[i].OccurredAt,
		}
	}

	writeJSON(w, http.StatusOK, dtos)
}

// handlePipelineThroughput returns aggregated pipeline throughput for the last 24 h.
//   - issues_completed_per_hour_24h: distinct issues reaching status:needs-qc / 24
//   - avg_cycle_time_hours: mean time from first event to needs-qc per issue
func (a *API) handlePipelineThroughput(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not configured")
		return
	}

	since := time.Now().Add(-24 * time.Hour)
	events, err := a.store.GetPipelineEvents(r.Context(), 0, since, 0)
	if err != nil {
		a.logger.Error("get pipeline events for throughput", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	completedIssues := make(map[int]bool)
	for i := range events {
		if events[i].ToStage == models.LabelStatusNeedsQc {
			completedIssues[events[i].IssueNumber] = true
		}
	}
	completedCount := len(completedIssues)

	type issueTimes struct {
		firstAt     time.Time
		completedAt time.Time
		completed   bool
	}
	issueData := make(map[int]*issueTimes)
	for i := range events {
		ev := &events[i]
		it, ok := issueData[ev.IssueNumber]
		if !ok {
			it = &issueTimes{firstAt: ev.OccurredAt}
			issueData[ev.IssueNumber] = it
		}
		if ev.OccurredAt.Before(it.firstAt) {
			it.firstAt = ev.OccurredAt
		}
		if ev.ToStage == models.LabelStatusNeedsQc && !it.completed {
			it.completedAt = ev.OccurredAt
			it.completed = true
		}
	}

	var totalCycleHours float64
	var cycleCount int
	for _, it := range issueData {
		if it.completed {
			totalCycleHours += it.completedAt.Sub(it.firstAt).Hours()
			cycleCount++
		}
	}
	var avgCycleHours float64
	if cycleCount > 0 {
		avgCycleHours = totalCycleHours / float64(cycleCount)
	}

	writeJSON(w, http.StatusOK, throughputDTO{
		IssuesCompletedPerHour24h: float64(completedCount) / 24.0,
		AvgCycleTimeHours:         avgCycleHours,
		GeneratedAt:               time.Now().UTC(),
	})
}
