package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/herbhall/samverk/pkg/models"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// Store defines the persistence interface for Samverk.
type Store interface {
	// Sessions
	CreateSession(ctx context.Context, s *models.Session) error
	GetSession(ctx context.Context, id string) (*models.Session, error)
	UpdateSession(ctx context.Context, s *models.Session) error
	ListSessions(ctx context.Context, status models.SessionStatus) ([]*models.Session, error)

	// Cost tracking
	RecordCost(ctx context.Context, r *models.CostRecord) error
	ComputeCostSince(ctx context.Context, since time.Time) (*models.CostSummary, error)
	GetBudgetStatus(ctx context.Context, dailyBudgetUSD float64) (spent float64, remaining float64, err error)

	// Scaling events (written by dispatch, read by serve and MCP)
	SaveScalingEvent(ctx context.Context, e models.ScalingEvent) error
	ListScalingEvents(ctx context.Context, limit int) ([]*models.ScalingEvent, error)

	// Scaling control (written by serve/CLI, read by dispatch autoscaler)
	GetScalingControl(ctx context.Context) (*models.ScalingControl, error)
	UpsertScalingControl(ctx context.Context, c models.ScalingControl) error

	// Task profiles (updated after each completed session, read by policy engine and metrics API)
	UpdateTaskProfile(ctx context.Context, agentType, provider string) error
	ListTaskProfiles(ctx context.Context) ([]*models.TaskProfile, error)
	GetTaskProfile(ctx context.Context, agentType, provider string) (*models.TaskProfile, error)

	Close() error
}

// SQLiteStore implements Store using an SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// New opens (or creates) an SQLite database at dbPath, runs migrations,
// and enables WAL mode and foreign keys.
func New(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for concurrent read performance.
	if _, err = db.ExecContext(context.Background(), "PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	// Enable foreign key enforcement.
	if _, err = db.ExecContext(context.Background(), "PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err = s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// migrate creates the required tables and indexes if they do not exist.
func (s *SQLiteStore) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS sessions (
	id          TEXT PRIMARY KEY,
	issue_number INTEGER NOT NULL,
	agent_type  TEXT NOT NULL,
	provider    TEXT NOT NULL,
	model       TEXT NOT NULL,
	status      TEXT NOT NULL,
	started_at  TEXT NOT NULL,
	finished_at TEXT,
	error       TEXT,
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS cost_records (
	id            TEXT PRIMARY KEY,
	session_id    TEXT NOT NULL REFERENCES sessions(id),
	provider      TEXT NOT NULL,
	model         TEXT NOT NULL,
	input_tokens  INTEGER NOT NULL,
	output_tokens INTEGER NOT NULL,
	cost_usd      REAL NOT NULL,
	created_at    TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cost_session ON cost_records(session_id);
CREATE INDEX IF NOT EXISTS idx_cost_created ON cost_records(created_at);

CREATE TABLE IF NOT EXISTS scaling_events (
	id           TEXT PRIMARY KEY,
	timestamp    TEXT NOT NULL,
	action       TEXT NOT NULL,
	from_workers INTEGER NOT NULL,
	to_workers   INTEGER NOT NULL,
	reason       TEXT NOT NULL,
	confidence   REAL NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_scaling_ts ON scaling_events(timestamp DESC);

CREATE TABLE IF NOT EXISTS scaling_control (
	id             INTEGER PRIMARY KEY CHECK (id = 1),
	paused         INTEGER NOT NULL DEFAULT 0,
	manual_workers INTEGER NOT NULL DEFAULT 0,
	set_at         TEXT NOT NULL,
	note           TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS task_profiles (
	agent_type      TEXT NOT NULL,
	provider        TEXT NOT NULL,
	avg_duration_ms INTEGER NOT NULL DEFAULT 0,
	p50_duration_ms INTEGER NOT NULL DEFAULT 0,
	p90_duration_ms INTEGER NOT NULL DEFAULT 0,
	sample_count    INTEGER NOT NULL DEFAULT 0,
	avg_tokens      INTEGER NOT NULL DEFAULT 0,
	updated_at      TEXT NOT NULL,
	PRIMARY KEY (agent_type, provider)
);
`
	_, err := s.db.ExecContext(context.Background(), ddl)
	return err
}

// generateID produces a random hex-encoded identifier.
func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp if crypto/rand fails (shouldn't happen).
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
