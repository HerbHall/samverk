package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/pkg/models"
)

// TestPipelineStages_Classification verifies that each status:* label maps to the
// correct pipeline stage, and that closed issues always land in "done".
func TestPipelineStages_Classification(t *testing.T) {
	now := time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		issue     *forge.Issue
		wantStage string
	}{
		{
			name:      "closed issue goes to done",
			issue:     &forge.Issue{Number: 1, Title: "done issue", State: forge.StateClosed, CreatedAt: now, UpdatedAt: now},
			wantStage: "done",
		},
		{
			name:      "open with no status label goes to open",
			issue:     &forge.Issue{Number: 2, Title: "unrouted", State: forge.StateOpen, Labels: []string{models.LabelAgentCodeGen}, CreatedAt: now, UpdatedAt: now},
			wantStage: "open",
		},
		{
			name:      models.LabelStatusQueued,
			issue:     &forge.Issue{Number: 3, Title: "queued issue", State: forge.StateOpen, Labels: []string{models.LabelStatusQueued}, CreatedAt: now, UpdatedAt: now},
			wantStage: "queued",
		},
		{
			name:      models.LabelStatusClaimed,
			issue:     &forge.Issue{Number: 4, Title: "claimed issue", State: forge.StateOpen, Labels: []string{models.LabelStatusClaimed}, CreatedAt: now, UpdatedAt: now},
			wantStage: "claimed",
		},
		{
			name:      models.LabelStatusInProgress,
			issue:     &forge.Issue{Number: 5, Title: "in progress", State: forge.StateOpen, Labels: []string{models.LabelStatusInProgress}, CreatedAt: now, UpdatedAt: now},
			wantStage: "in_progress",
		},
		{
			name:      models.LabelStatusNeedsQc,
			issue:     &forge.Issue{Number: 6, Title: "needs qc", State: forge.StateOpen, Labels: []string{models.LabelStatusNeedsQc}, CreatedAt: now, UpdatedAt: now},
			wantStage: "needs_qc",
		},
		{
			name:      models.LabelStatusNeedsHuman,
			issue:     &forge.Issue{Number: 7, Title: "needs human", State: forge.StateOpen, Labels: []string{models.LabelStatusNeedsHuman}, CreatedAt: now, UpdatedAt: now},
			wantStage: "needs_human",
		},
		{
			name:      models.LabelStatusBlocked,
			issue:     &forge.Issue{Number: 8, Title: "blocked", State: forge.StateOpen, Labels: []string{models.LabelStatusBlocked}, CreatedAt: now, UpdatedAt: now},
			wantStage: "blocked",
		},
		{
			name:      "status:failed",
			issue:     &forge.Issue{Number: 9, Title: "failed", State: forge.StateOpen, Labels: []string{"status:failed"}, CreatedAt: now, UpdatedAt: now},
			wantStage: "failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var openIssues, closedIssues []*forge.Issue
			if tt.issue.State == forge.StateOpen {
				openIssues = append(openIssues, tt.issue)
			} else {
				closedIssues = append(closedIssues, tt.issue)
			}
			tracker := &pipelineMockTracker{
				openIssues:   openIssues,
				closedIssues: closedIssues,
			}
			ts := newTestAPI(t, tracker, nil, nil)

			resp := doGet(t, ts.URL+"/api/v1/pipeline/stages")
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}

			var body struct {
				Stages []struct {
					Name   string `json:"name"`
					Issues []struct {
						Number int `json:"number"`
					} `json:"issues"`
				} `json:"stages"`
			}
			decodeJSON(t, resp, &body)

			found := false
			for _, stage := range body.Stages {
				if stage.Name != tt.wantStage {
					continue
				}
				for _, iss := range stage.Issues {
					if iss.Number == tt.issue.Number {
						found = true
						break
					}
				}
			}
			if !found {
				t.Errorf("issue #%d not found in stage %q", tt.issue.Number, tt.wantStage)
			}
		})
	}
}

// TestPipelineStages_Response verifies the overall response shape, stage ordering,
// totals, and throughput_24h calculation.
func TestPipelineStages_Response(t *testing.T) {
	now := time.Now().UTC()
	recent := now.Add(-1 * time.Hour) // within the 24h window

	openIssues := []*forge.Issue{
		{Number: 1, Title: "open issue", State: forge.StateOpen, CreatedAt: now, UpdatedAt: now},
		{Number: 2, Title: "queued issue", State: forge.StateOpen, Labels: []string{models.LabelStatusQueued}, CreatedAt: now, UpdatedAt: now},
	}
	closedIssues := []*forge.Issue{
		{Number: 3, Title: "done issue", State: forge.StateClosed, CreatedAt: now, UpdatedAt: recent, ClosedAt: &recent},
	}

	tracker := &pipelineMockTracker{
		openIssues:   openIssues,
		closedIssues: closedIssues,
	}
	ts := newTestAPI(t, tracker, nil, nil)

	resp := doGet(t, ts.URL+"/api/v1/pipeline/stages")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Stages []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		} `json:"stages"`
		Totals struct {
			Open          int `json:"open"`
			Closed        int `json:"closed"`
			Throughput24h int `json:"throughput_24h"`
		} `json:"totals"`
		GeneratedAt string `json:"generated_at"`
	}
	decodeJSON(t, resp, &body)

	if len(body.Stages) != 12 {
		t.Errorf("stage count = %d, want 12", len(body.Stages))
	}

	wantOrder := []string{"readiness_review", "planning", "impact_review", "open", "queued", "claimed", "in_progress", "needs_qc", "needs_human", "blocked", "done", "failed"}
	for i, stage := range body.Stages {
		if stage.Name != wantOrder[i] {
			t.Errorf("stages[%d].name = %q, want %q", i, stage.Name, wantOrder[i])
		}
	}

	if body.Totals.Open != 2 {
		t.Errorf("totals.open = %d, want 2", body.Totals.Open)
	}
	if body.Totals.Closed != 1 {
		t.Errorf("totals.closed = %d, want 1", body.Totals.Closed)
	}
	if body.Totals.Throughput24h != 1 {
		t.Errorf("totals.throughput_24h = %d, want 1", body.Totals.Throughput24h)
	}
	if body.GeneratedAt == "" {
		t.Error("generated_at must not be empty")
	}
}

// TestPipelineStages_NoTracker verifies 503 when no issue tracker is configured.
func TestPipelineStages_NoTracker(t *testing.T) {
	ts := newTestAPI(t, nil, nil, nil)

	resp := doGet(t, ts.URL+"/api/v1/pipeline/stages")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}

	var body map[string]string
	decodeJSON(t, resp, &body)
	if body["error"] != "issue tracker not available" {
		t.Errorf("error = %q, want %q", body["error"], "issue tracker not available")
	}
}

// pipelineMockTracker routes ListIssues calls by State so open and closed
// issues can be configured independently.
type pipelineMockTracker struct {
	mockTracker                  // inherit all no-op methods
	openIssues   []*forge.Issue
	closedIssues []*forge.Issue
}

func (m *pipelineMockTracker) ListIssues(_ context.Context, opts *forge.ListOptions) ([]*forge.Issue, error) {
	if opts.State == forge.StateClosed {
		return m.closedIssues, nil
	}
	return m.openIssues, nil
}
