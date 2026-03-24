package store

import (
	"context"
	"fmt"
	"time"
)

// PipelineStage is a typed string for pipeline stage names.
type PipelineStage = string

// Pipeline stage constants used by the API and event system.
const (
	StageOpen       PipelineStage = "open"
	StageQueued     PipelineStage = "queued"
	StageClaimed    PipelineStage = "claimed"
	StageInProgress PipelineStage = "in_progress"
	StageNeedsQC    PipelineStage = "needs_qc"
	StageNeedsHuman PipelineStage = "needs_human"
	StageBlocked    PipelineStage = "blocked"
	StageDone       PipelineStage = "done"
	StageFailed     PipelineStage = "failed"
)

// RecordPipelineEvent inserts a pipeline stage transition event.
func (s *SQLiteStore) RecordPipelineEvent(ctx context.Context, e PipelineEvent) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO pipeline_events (issue_number, project, from_stage, to_stage, triggered_by, occurred_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		e.IssueNumber, e.Project, e.FromStage, e.ToStage, e.TriggeredBy,
		e.OccurredAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("record pipeline event: %w", err)
	}
	return nil
}

// GetPipelineEvents returns pipeline events for an issue since the given time,
// ordered by occurred_at ascending. Use issueNumber=0 to query all issues.
// Use limit=0 for no row limit.
func (s *SQLiteStore) GetPipelineEvents(ctx context.Context, issueNumber int, since time.Time, limit int) ([]PipelineEvent, error) {
	sinceStr := since.UTC().Format(time.RFC3339)

	var query string
	var args []any

	if issueNumber > 0 {
		query = `SELECT id, issue_number, project, from_stage, to_stage, triggered_by, occurred_at
		         FROM pipeline_events WHERE issue_number = ? AND occurred_at >= ? ORDER BY occurred_at ASC`
		args = []any{issueNumber, sinceStr}
	} else {
		query = `SELECT id, issue_number, project, from_stage, to_stage, triggered_by, occurred_at
		         FROM pipeline_events WHERE occurred_at >= ? ORDER BY occurred_at ASC`
		args = []any{sinceStr}
	}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get pipeline events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []PipelineEvent
	for rows.Next() {
		var ev PipelineEvent
		var occurredStr string
		if scanErr := rows.Scan(
			&ev.ID, &ev.IssueNumber, &ev.Project,
			&ev.FromStage, &ev.ToStage, &ev.TriggeredBy, &occurredStr,
		); scanErr != nil {
			return nil, fmt.Errorf("scan pipeline event: %w", scanErr)
		}
		ev.OccurredAt, _ = time.Parse(time.RFC3339, occurredStr)
		events = append(events, ev)
	}
	return events, rows.Err()
}
