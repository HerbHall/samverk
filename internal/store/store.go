package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
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
	ComputeCostForIssue(ctx context.Context, issueNumber int) (*models.CostSummary, error)
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

	// Metric snapshots (written by dispatch, read by serve for cross-process metrics)
	SaveMetricSnapshot(ctx context.Context, m MetricSnapshot) error
	LatestMetricSnapshot(ctx context.Context) (*MetricSnapshot, error)

	// Failure events (written by dispatcher and runner, read by API, MCP, and CLI)
	SaveFailureEvent(ctx context.Context, e *models.FailureEvent) error
	ListFailureEvents(ctx context.Context, since time.Time, limit int) ([]*models.FailureEvent, error)
	CountFailuresByIssue(ctx context.Context, issueNumber int) (int, error)
	GetFailureSummary(ctx context.Context, since time.Time) (*models.FailureSummary, error)

	// Persisted issue failure counter (survives restarts, unlike in-memory map)
	GetIssueFailureCount(ctx context.Context, issueNumber int) (int, error)
	IncrementIssueFailureCount(ctx context.Context, issueNumber int) (int, error)
	ClearIssueFailureCount(ctx context.Context, issueNumber int) error

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
	id             TEXT PRIMARY KEY,
	issue_number   INTEGER NOT NULL,
	agent_type     TEXT NOT NULL,
	provider       TEXT NOT NULL,
	model          TEXT NOT NULL,
	status         TEXT NOT NULL,
	started_at     TEXT NOT NULL,
	finished_at    TEXT,
	error          TEXT,
	estimated_timeout_ms INTEGER NOT NULL DEFAULT 0,
	partial_output  TEXT NOT NULL DEFAULT '',
	checkpoint_hash TEXT NOT NULL DEFAULT '',
	created_at      TEXT NOT NULL,
	updated_at      TEXT NOT NULL
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
CREATE INDEX IF NOT EXISTS idx_sessions_issue ON sessions(issue_number);

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

CREATE TABLE IF NOT EXISTS metric_snapshots (
	id                     INTEGER PRIMARY KEY CHECK (id = 1),
	collected_at           TEXT NOT NULL,
	total_workers          INTEGER NOT NULL DEFAULT 0,
	active_workers         INTEGER NOT NULL DEFAULT 0,
	idle_workers           INTEGER NOT NULL DEFAULT 0,
	queue_depth            INTEGER NOT NULL DEFAULT 0,
	tasks_completed        INTEGER NOT NULL DEFAULT 0,
	tasks_failed           INTEGER NOT NULL DEFAULT 0,
	avg_task_duration_ms   REAL NOT NULL DEFAULT 0,
	p95_task_duration_ms   REAL NOT NULL DEFAULT 0,
	disp_collected_at      TEXT NOT NULL,
	claimed_count          INTEGER NOT NULL DEFAULT 0,
	total_routed           INTEGER NOT NULL DEFAULT 0,
	total_requeued         INTEGER NOT NULL DEFAULT 0,
	total_events_processed INTEGER NOT NULL DEFAULT 0,
	avg_poll_latency_ms    REAL NOT NULL DEFAULT 0,
	last_poll_at           TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS failure_events (
	id             TEXT PRIMARY KEY,
	issue_number   INTEGER NOT NULL,
	session_id     TEXT NOT NULL DEFAULT '',
	failure_class  TEXT NOT NULL,
	error_message  TEXT NOT NULL DEFAULT '',
	agent_type     TEXT NOT NULL DEFAULT '',
	provider       TEXT NOT NULL DEFAULT '',
	attempt_number INTEGER NOT NULL DEFAULT 1,
	duration_ms    INTEGER NOT NULL DEFAULT 0,
	timestamp      TEXT NOT NULL,
	created_at     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_failure_issue ON failure_events(issue_number);
CREATE INDEX IF NOT EXISTS idx_failure_class ON failure_events(failure_class);
CREATE INDEX IF NOT EXISTS idx_failure_ts ON failure_events(timestamp DESC);

CREATE TABLE IF NOT EXISTS issue_failure_counts (
	issue_number INTEGER PRIMARY KEY,
	count        INTEGER NOT NULL DEFAULT 0,
	updated_at   TEXT NOT NULL
);
`
	if _, err := s.db.ExecContext(context.Background(), ddl); err != nil {
		return err
	}

	// Incremental migrations for existing databases.
	// SQLite silently ignores duplicate ADD COLUMN when using IF NOT EXISTS
	// is not available, so we detect and skip manually.
	migrations := []string{
		`ALTER TABLE sessions ADD COLUMN partial_output TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN checkpoint_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN estimated_timeout_ms INTEGER NOT NULL DEFAULT 0`,
	}
	for _, m := range migrations {
		if _, err := s.db.ExecContext(context.Background(), m); err != nil {
			// "duplicate column name" means the column already exists — skip.
			if !isDuplicateColumn(err) {
				return fmt.Errorf("migration: %w", err)
			}
		}
	}
	return nil
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

// isDuplicateColumn returns true if the error indicates an ALTER TABLE ADD COLUMN
// failed because the column already exists. SQLite returns "duplicate column name: X".
func isDuplicateColumn(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column name")
}
