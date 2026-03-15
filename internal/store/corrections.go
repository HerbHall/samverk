package store

import (
	"context"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

// SaveCorrectionEvent inserts a correction event into the corrections table.
func (s *SQLiteStore) SaveCorrectionEvent(ctx context.Context, e *models.CorrectionEvent) error {
	if e.ID == "" {
		e.ID = generateID()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO corrections (id, issue_number, failure_class, action, scope, reason, new_provider, outcome, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.IssueNumber, string(e.FailureClass), e.Action,
		e.Scope, e.Reason, e.NewProvider, e.Outcome, now,
	)
	return err
}

// ListCorrectionEvents returns correction events for a given issue.
func (s *SQLiteStore) ListCorrectionEvents(ctx context.Context, issueNumber int) ([]*models.CorrectionEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, issue_number, failure_class, action, scope, reason, new_provider, outcome, created_at
		 FROM corrections WHERE issue_number = ? ORDER BY created_at DESC`,
		issueNumber,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var events []*models.CorrectionEvent
	for rows.Next() {
		e := &models.CorrectionEvent{}
		var fc, createdAt string
		if scanErr := rows.Scan(&e.ID, &e.IssueNumber, &fc, &e.Action, &e.Scope,
			&e.Reason, &e.NewProvider, &e.Outcome, &createdAt); scanErr != nil {
			return nil, scanErr
		}
		e.FailureClass = models.FailureClass(fc)
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		events = append(events, e)
	}
	return events, rows.Err()
}
