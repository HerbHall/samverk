package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"samverk.dev/samverk/internal/provider"
)

// SaveProviderHealthSnapshot persists the latest provider health data so the
// serve process can read it without a running health monitor. The snapshot is
// stored as a JSON blob in a single-row table (upsert pattern, id = 1).
func (s *SQLiteStore) SaveProviderHealthSnapshot(ctx context.Context, entries []provider.ProviderHealth) error {
	blob, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshal provider health snapshot: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO provider_health_snapshots (id, snapshot_at, data)
VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	snapshot_at = excluded.snapshot_at,
	data        = excluded.data`,
		time.Now().UTC().Format(time.RFC3339),
		string(blob),
	)
	if err != nil {
		return fmt.Errorf("save provider health snapshot: %w", err)
	}
	return nil
}

// LoadProviderHealthSnapshot returns the last persisted provider health entries
// and the time they were saved. Returns ErrNotFound if no snapshot exists yet.
func (s *SQLiteStore) LoadProviderHealthSnapshot(ctx context.Context) ([]provider.ProviderHealth, time.Time, error) {
	var snapshotAt, data string
	err := s.db.QueryRowContext(ctx, `
SELECT snapshot_at, data FROM provider_health_snapshots WHERE id = 1`).Scan(&snapshotAt, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("load provider health snapshot: %w", err)
	}

	t, err := time.Parse(time.RFC3339, snapshotAt)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("parse provider health snapshot time: %w", err)
	}

	var entries []provider.ProviderHealth
	if err = json.Unmarshal([]byte(data), &entries); err != nil {
		return nil, time.Time{}, fmt.Errorf("unmarshal provider health snapshot: %w", err)
	}

	return entries, t, nil
}
