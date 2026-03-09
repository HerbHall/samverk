package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

// GetScalingControl returns the current scaling control record.
// Returns a zeroed ScalingControl (no override, not paused) if no record exists.
func (s *SQLiteStore) GetScalingControl(ctx context.Context) (*models.ScalingControl, error) {
	var c models.ScalingControl
	var setAtStr string
	var paused, manualWorkers int

	err := s.db.QueryRowContext(ctx,
		`SELECT paused, manual_workers, set_at, note FROM scaling_control WHERE id = 1`,
	).Scan(&paused, &manualWorkers, &setAtStr, &c.Note)

	if err == sql.ErrNoRows {
		return &models.ScalingControl{}, nil
	}
	if err != nil {
		return nil, err
	}

	c.Paused = paused != 0
	c.ManualWorkers = manualWorkers
	c.SetAt, _ = time.Parse(time.RFC3339, setAtStr)
	return &c, nil
}

// UpsertScalingControl inserts or replaces the scaling control record (single row, id=1).
func (s *SQLiteStore) UpsertScalingControl(ctx context.Context, c models.ScalingControl) error {
	paused := 0
	if c.Paused {
		paused = 1
	}
	setAt := c.SetAt
	if setAt.IsZero() {
		setAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO scaling_control (id, paused, manual_workers, set_at, note)
VALUES (1, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    paused = excluded.paused,
    manual_workers = excluded.manual_workers,
    set_at = excluded.set_at,
    note = excluded.note`,
		paused,
		c.ManualWorkers,
		setAt.UTC().Format(time.RFC3339),
		c.Note,
	)
	return err
}
