package profile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"samverk.dev/samverk/internal/store"
)

// ProfileStore defines the read API for user profiles.
type ProfileStore interface {
	// Get returns the latest profile version.
	Get(ctx context.Context) (*Profile, error)
	// GetField extracts a single field by dot-separated path (e.g., "git.branch_pattern").
	GetField(ctx context.Context, path string) (any, error)
	// Version returns the current profile version number.
	Version(ctx context.Context) (int, error)
	// Import stores a new profile version (append-only).
	Import(ctx context.Context, p *Profile) error
	// Close releases resources.
	Close() error
}

// SQLiteProfileStore implements ProfileStore using SQLite.
type SQLiteProfileStore struct {
	db *sql.DB
}

// Compile-time interface check.
var _ ProfileStore = (*SQLiteProfileStore)(nil)

// New creates a ProfileStore using the given database connection.
// It runs migrations to ensure the profiles table exists.
func New(db *sql.DB) (ps *SQLiteProfileStore, err error) {
	ps = &SQLiteProfileStore{db: db}
	if err = ps.migrate(); err != nil {
		return nil, fmt.Errorf("profile migrate: %w", err)
	}
	return ps, nil
}

// Close is a no-op because the db connection is shared and owned by the caller.
func (s *SQLiteProfileStore) Close() error {
	return nil
}

// migrate creates the profiles table and index if they do not exist.
func (s *SQLiteProfileStore) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS profiles (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	version    INTEGER NOT NULL,
	data       TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_profiles_version ON profiles(version);
`
	_, err := s.db.ExecContext(context.Background(), ddl)
	return err
}

// Get returns the latest profile version.
func (s *SQLiteProfileStore) Get(ctx context.Context) (p *Profile, err error) {
	var data string
	err = s.db.QueryRowContext(ctx,
		"SELECT data FROM profiles ORDER BY version DESC LIMIT 1",
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query latest profile: %w", err)
	}

	p = &Profile{}
	if err = json.Unmarshal([]byte(data), p); err != nil {
		return nil, fmt.Errorf("unmarshal profile: %w", err)
	}
	return p, nil
}

// GetField extracts a single field by dot-separated path.
// It navigates the JSON representation of the profile using the path segments.
func (s *SQLiteProfileStore) GetField(ctx context.Context, path string) (val any, err error) {
	p, err := s.Get(ctx)
	if err != nil {
		return nil, err
	}

	// Round-trip through JSON to get a generic map for path navigation.
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal profile for field lookup: %w", err)
	}

	var m map[string]any
	if err = json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("unmarshal profile to map: %w", err)
	}

	parts := strings.Split(path, ".")
	var current any = m
	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("field %q: %w", path, store.ErrNotFound)
		}
		val, ok = obj[part]
		if !ok {
			return nil, fmt.Errorf("field %q: %w", path, store.ErrNotFound)
		}
		current = val
	}
	return current, nil
}

// Version returns the current profile version number.
// Returns 0 if no profiles exist.
func (s *SQLiteProfileStore) Version(ctx context.Context) (v int, err error) {
	var version sql.NullInt64
	err = s.db.QueryRowContext(ctx,
		"SELECT MAX(version) FROM profiles",
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("query max version: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return int(version.Int64), nil
}

// Import stores a new profile version (append-only).
func (s *SQLiteProfileStore) Import(ctx context.Context, p *Profile) error {
	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Get next version number.
	currentVersion, err := s.Version(ctx)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}
	nextVersion := currentVersion + 1

	_, err = s.db.ExecContext(ctx,
		"INSERT INTO profiles (version, data, created_at, updated_at) VALUES (?, ?, ?, ?)",
		nextVersion, string(data), now, now,
	)
	if err != nil {
		return fmt.Errorf("insert profile: %w", err)
	}
	return nil
}
