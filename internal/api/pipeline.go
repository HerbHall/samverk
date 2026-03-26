package api

import (
	"net/http"
	"time"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
)

// pipelineIssueRef is a minimal issue reference within a pipeline stage.
type pipelineIssueRef struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

// pipelineStage groups issues by their lifecycle stage.
type pipelineStage struct {
	Name   string             `json:"name"`
	Count  int                `json:"count"`
	Issues []pipelineIssueRef `json:"issues"`
}

// pipelineTotals contains aggregate counts across all stages.
type pipelineTotals struct {
	Open          int `json:"open"`
	Closed        int `json:"closed"`
	Throughput24h int `json:"throughput_24h"`
}

// pipelineResponse is the JSON response for GET /api/v1/pipeline/stages.
type pipelineResponse struct {
	Stages      []pipelineStage `json:"stages"`
	Totals      pipelineTotals  `json:"totals"`
	GeneratedAt string          `json:"generated_at"`
}

// classifyStage maps a forge.Issue to one of the nine pipeline stage names.
// Closed issues → "done". Open issues are classified by their status:* label.
// Open issues with no recognised status label → "open" (unrouted).
func classifyStage(iss *forge.Issue) string {
	if iss.State == forge.StateClosed {
		return store.StageDone
	}
	for _, label := range iss.Labels {
		switch label {
		case models.LabelStatusQueued:
			return store.StageQueued
		case models.LabelStatusClaimed:
			return store.StageClaimed
		case models.LabelStatusInProgress:
			return store.StageInProgress
		case models.LabelStatusNeedsQc:
			return store.StageNeedsQC
		case models.LabelStatusNeedsHuman:
			return store.StageNeedsHuman
		case models.LabelStatusBlocked:
			return store.StageBlocked
		case "status:failed":
			return store.StageFailed
		}
	}
	// Open with no status label — unrouted.
	return store.StageOpen
}

// handlePipelineStages handles GET /api/v1/pipeline/stages.
// It fetches all issues (open and closed) from the forge, classifies each by
// its status:* label and state, then returns the pipeline breakdown.
func (a *API) handlePipelineStages(w http.ResponseWriter, r *http.Request) {
	if a.tracker == nil {
		writeError(w, http.StatusServiceUnavailable, "issue tracker not available")
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()
	cutoff := now.Add(-24 * time.Hour)

	// Fetch open issues.
	openIssues, err := a.tracker.ListIssues(ctx, &forge.ListOptions{
		State:   forge.StateOpen,
		PerPage: maxLimit,
		Page:    1,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list open issues")
		return
	}

	// Fetch closed issues (for "done" stage and throughput_24h).
	closedIssues, err := a.tracker.ListIssues(ctx, &forge.ListOptions{
		State:   forge.StateClosed,
		PerPage: maxLimit,
		Page:    1,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list closed issues")
		return
	}

	// Ordered stage names used in the response.
	stageOrder := store.StageOrder

	// Accumulate issues per stage.
	stageMap := make(map[string][]pipelineIssueRef, len(stageOrder))
	for _, name := range stageOrder {
		stageMap[name] = []pipelineIssueRef{}
	}

	throughput24h := 0

	for _, iss := range openIssues {
		stage := classifyStage(iss)
		stageMap[stage] = append(stageMap[stage], pipelineIssueRef{
			Number: iss.Number,
			Title:  iss.Title,
		})
	}

	for _, iss := range closedIssues {
		stageMap[store.StageDone] = append(stageMap[store.StageDone], pipelineIssueRef{
			Number: iss.Number,
			Title:  iss.Title,
		})
		// Count issues closed within the last 24h.
		if iss.ClosedAt != nil && iss.ClosedAt.After(cutoff) {
			throughput24h++
		} else if iss.ClosedAt == nil && iss.UpdatedAt.After(cutoff) {
			// Fallback: forge did not populate ClosedAt — use UpdatedAt.
			throughput24h++
		}
	}

	// Build ordered stage list.
	stages := make([]pipelineStage, 0, len(stageOrder))
	for _, name := range stageOrder {
		refs := stageMap[name]
		stages = append(stages, pipelineStage{
			Name:   name,
			Count:  len(refs),
			Issues: refs,
		})
	}

	totals := pipelineTotals{
		Open:          len(openIssues),
		Closed:        len(closedIssues),
		Throughput24h: throughput24h,
	}

	writeJSON(w, http.StatusOK, pipelineResponse{
		Stages:      stages,
		Totals:      totals,
		GeneratedAt: now.Format("2006-01-02T15:04:05Z"),
	})
}
