package store

import (
	"context"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

// SaveScalingEvent inserts a scaling event into the database.
// It is safe to call concurrently; each call uses a fresh ID.
func (s *SQLiteStore) SaveScalingEvent(ctx context.Context, e models.ScalingEvent) error {
	id := generateID()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO scaling_events (id, timestamp, action, from_workers, to_workers, reason, confidence)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id,
		e.Timestamp.UTC().Format(time.RFC3339),
		e.Action,
		e.FromWorkers,
		e.ToWorkers,
		e.Reason,
		e.Confidence,
	)
	return err
}

// ListScalingEvents returns up to limit scaling events ordered newest-first.
// If limit <= 0 it defaults to 100.
func (s *SQLiteStore) ListScalingEvents(ctx context.Context, limit int) ([]*models.ScalingEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, timestamp, action, from_workers, to_workers, reason, confidence
FROM scaling_events
ORDER BY timestamp DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var events []*models.ScalingEvent
	for rows.Next() {
		var e models.ScalingEvent
		var tsStr string
		if err := rows.Scan(&e.ID, &tsStr, &e.Action, &e.FromWorkers, &e.ToWorkers, &e.Reason, &e.Confidence); err != nil {
			return nil, err
		}
		e.Timestamp, err = time.Parse(time.RFC3339, tsStr)
		if err != nil {
			return nil, err
		}
		events = append(events, &e)
	}
	return events, rows.Err()
}
