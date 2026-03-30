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

	"github.com/herbhall/samverk/internal/audit"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/pkg/models"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// PipelineEvent records a stage transition or general event for an issue.
type PipelineEvent struct {
	ID          int64
	IssueNumber int
	Project     string
	FromStage   string
	ToStage     string
	TriggeredBy string
	OccurredAt  time.Time
	// Extended fields (Wave 3): broader event types beyond stage transitions.
	EventType string // "stage_change", "label_added", "label_removed", etc.
	Key       string // label name, field name, or other event-specific key
	Value     string // new value
	OldValue  string // previous value
}

// Store defines the persistence interface for Samverk.
type Store interface {
	// Sessions
	CreateSession(ctx context.Context, s *models.Session) error
	GetSession(ctx context.Context, id string) (*models.Session, error)
	UpdateSession(ctx context.Context, s *models.Session) error
	ListSessions(ctx context.Context, status models.SessionStatus) ([]*models.Session, error)
	// GetLatestCheckpoint returns the partial_output from the most recent
	// failed session for the given issue that has a non-empty checkpoint.
	// Returns empty string (not ErrNotFound) when no checkpoint exists.
	GetLatestCheckpoint(ctx context.Context, issueNumber int) (string, error)

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
	RecentFailuresForIssue(ctx context.Context, issueNumber, limit int) ([]string, error)
	GetFailureSummary(ctx context.Context, since time.Time) (*models.FailureSummary, error)
	GetKPIReport(ctx context.Context) (*models.KPIReport, error)

	// Planning stage KPI rates (read by API and continuous improvement engine)
	ReadinessGapRate(ctx context.Context, days int) (float64, error)
	PlanningEscalationRate(ctx context.Context, days int) (float64, error)
	ImpactConflictRate(ctx context.Context, days int) (float64, error)

	// Persisted issue failure counter (survives restarts, unlike in-memory map)
	GetIssueFailureCount(ctx context.Context, issueNumber int) (int, error)
	IncrementIssueFailureCount(ctx context.Context, issueNumber int) (int, error)
	ClearIssueFailureCount(ctx context.Context, issueNumber int) error
	ResetAllFailureCounts(ctx context.Context) (int64, error)

	// Correction events (written by correction engine, read by API and diagnostics)
	SaveCorrectionEvent(ctx context.Context, e *models.CorrectionEvent) error
	ListCorrectionEvents(ctx context.Context, issueNumber int) ([]*models.CorrectionEvent, error)

	// Provider audits (written by audit CLI, read by API and diagnostics)
	SaveAuditResults(ctx context.Context, results []audit.AuditResult) error
	GetLastAudit(ctx context.Context) ([]audit.AuditResult, error)

	// Check-in state (persists the last get_digest call time for accurate "away" duration)
	GetLastCheckIn(ctx context.Context) (time.Time, error)
	SetLastCheckIn(ctx context.Context, t time.Time) error

	// Provider success timestamps (read by MCP get_provider_health for enriched status)
	LatestSuccessByProvider(ctx context.Context) (map[string]time.Time, error)

	// Provider health snapshots (written by dispatch, read by serve for cross-process health)
	SaveProviderHealthSnapshot(ctx context.Context, entries []provider.ProviderHealth) error
	LoadProviderHealthSnapshot(ctx context.Context) ([]provider.ProviderHealth, time.Time, error)

	// Pipeline events (written by dispatcher on each stage transition, read by API)
	RecordPipelineEvent(ctx context.Context, e PipelineEvent) error
	RecordEvent(ctx context.Context, e PipelineEvent) error
	GetPipelineEvents(ctx context.Context, issueNumber int, since time.Time, limit int) ([]PipelineEvent, error)

	// Triage decisions (written by triage agent, read by API and digest)
	SaveTriageDecision(ctx context.Context, d *TriageDecision) error
	ListTriageDecisions(ctx context.Context, since time.Time, limit int) ([]TriageDecision, error)
	GetTriageDecisionForIssue(ctx context.Context, issueNumber int) (*TriageDecision, error)
	UpdateTriageReview(ctx context.Context, id int64, reviewedBy, outcome string) error

	// Issue cache (synced from forge, read by API and MCP for accurate counts)
	SyncIssues(ctx context.Context, project string, issues []CachedIssue) error
	DeleteStaleIssues(ctx context.Context, project string, currentNumbers []int) error
	GetIssueCounts(ctx context.Context, project string) (open, closed int, err error)
	GetAllIssueCounts(ctx context.Context) (map[string]IssueCounts, error)
	GetIssueLabelCounts(ctx context.Context, project string) (map[string]int, error)
	GetCacheSyncTime(ctx context.Context, project string) (time.Time, error)
	GetCachedIssue(ctx context.Context, project string, number int) (*CachedIssue, error)
	ListCachedIssues(ctx context.Context, project, state string, labels []string) ([]CachedIssue, error)

	Close() error
}

// SQLiteStore implements Store using an SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// New opens (or creates) an SQLite database at dbPath, runs migrations,
// and enables WAL mode and foreign keys.
func New(dbPath string) (*SQLiteStore, error) {
	// Set pragmas via DSN so they apply to ALL pooled connections,
	// not just the first one. PRAGMA statements only affect the connection
	// they execute on, which causes SQLITE_BUSY on other pool connections.
	dsn := dbPath + "?_pragma=busy_timeout%3d5000&_pragma=journal_mode%3dWAL&_pragma=foreign_keys%3dON"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
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
	max_turns_hit  BOOLEAN DEFAULT FALSE,
	turns_used     INTEGER DEFAULT 0,
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

CREATE TABLE IF NOT EXISTS corrections (
	id             TEXT PRIMARY KEY,
	issue_number   INTEGER NOT NULL,
	failure_class  TEXT NOT NULL,
	action         TEXT NOT NULL,
	scope          TEXT NOT NULL,
	reason         TEXT NOT NULL DEFAULT '',
	new_provider   TEXT NOT NULL DEFAULT '',
	outcome        TEXT NOT NULL DEFAULT 'pending',
	created_at     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_corrections_issue ON corrections(issue_number);
CREATE INDEX IF NOT EXISTS idx_corrections_created ON corrections(created_at DESC);

CREATE TABLE IF NOT EXISTS provider_audits (
	id            TEXT PRIMARY KEY,
	provider_name TEXT NOT NULL,
	provider_type TEXT NOT NULL,
	model         TEXT NOT NULL DEFAULT '',
	healthy       INTEGER NOT NULL DEFAULT 0,
	latency_ms    INTEGER NOT NULL DEFAULT 0,
	error         TEXT NOT NULL DEFAULT '',
	audited_at    TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_provider_audits_at ON provider_audits(audited_at DESC);

CREATE TABLE IF NOT EXISTS check_in_state (
	id            INTEGER PRIMARY KEY CHECK (id = 1),
	checked_in_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS provider_health_snapshots (
	id          INTEGER PRIMARY KEY CHECK (id = 1),
	snapshot_at TEXT NOT NULL,
	data        TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS pipeline_events (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	issue_number INTEGER NOT NULL,
	project      TEXT NOT NULL,
	from_stage   TEXT NOT NULL DEFAULT '',
	to_stage     TEXT NOT NULL,
	triggered_by TEXT NOT NULL DEFAULT '',
	occurred_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_pipeline_issue ON pipeline_events(issue_number);
CREATE INDEX IF NOT EXISTS idx_pipeline_occurred ON pipeline_events(occurred_at DESC);

CREATE TABLE IF NOT EXISTS triage_decisions (
	id                  INTEGER PRIMARY KEY AUTOINCREMENT,
	issue_number        INTEGER NOT NULL,
	project             TEXT NOT NULL,
	verdict             TEXT NOT NULL,
	reasoning           TEXT NOT NULL,
	criteria_met        TEXT,
	docs_referenced     TEXT,
	sub_issues_created  TEXT,
	created_at          TEXT NOT NULL,
	reviewed_by         TEXT,
	review_outcome      TEXT
);

CREATE INDEX IF NOT EXISTS idx_triage_issue ON triage_decisions(issue_number);
CREATE INDEX IF NOT EXISTS idx_triage_verdict ON triage_decisions(verdict);
CREATE INDEX IF NOT EXISTS idx_triage_created ON triage_decisions(created_at DESC);

CREATE TABLE IF NOT EXISTS issue_cache (
	number       INTEGER NOT NULL,
	project      TEXT NOT NULL,
	title        TEXT NOT NULL,
	body         TEXT NOT NULL DEFAULT '',
	state        TEXT NOT NULL,
	labels       TEXT NOT NULL DEFAULT '[]',
	assignees    TEXT NOT NULL DEFAULT '[]',
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL,
	closed_at    TEXT,
	synced_at    TEXT NOT NULL,
	PRIMARY KEY (project, number)
);

CREATE INDEX IF NOT EXISTS idx_issue_cache_state ON issue_cache(project, state);
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
		// Session max turns tracking (issue #348).
		`ALTER TABLE sessions ADD COLUMN max_turns_hit BOOLEAN DEFAULT FALSE`,
		`ALTER TABLE sessions ADD COLUMN turns_used INTEGER DEFAULT 0`,
		// RCA fields for failure_events (issue #117).
		`ALTER TABLE failure_events ADD COLUMN root_cause_category TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE failure_events ADD COLUMN fix_classification TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE failure_events ADD COLUMN prevention_measure TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE failure_events ADD COLUMN recurrence_risk TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE failure_events ADD COLUMN detection_method TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE failure_events ADD COLUMN time_to_detect_min INTEGER`,
		`ALTER TABLE failure_events ADD COLUMN linked_issues TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE failure_events ADD COLUMN component TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE failure_events ADD COLUMN resolved_at TEXT`,
		`ALTER TABLE failure_events ADD COLUMN status TEXT NOT NULL DEFAULT ''`,
		// Issue cache body column for frontmatter parsing without API calls.
		`ALTER TABLE issue_cache ADD COLUMN body TEXT NOT NULL DEFAULT ''`,
		// Extended pipeline_events columns for general event recording (Wave 3).
		`ALTER TABLE pipeline_events ADD COLUMN event_type TEXT NOT NULL DEFAULT 'stage_change'`,
		`ALTER TABLE pipeline_events ADD COLUMN key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE pipeline_events ADD COLUMN value TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE pipeline_events ADD COLUMN old_value TEXT NOT NULL DEFAULT ''`,
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

// GetLastCheckIn returns the time of the most recent get_digest call.
// Returns ErrNotFound if no check-in has been recorded yet.
func (s *SQLiteStore) GetLastCheckIn(ctx context.Context) (time.Time, error) {
	var ts string
	err := s.db.QueryRowContext(ctx, `SELECT checked_in_at FROM check_in_state WHERE id = 1`).Scan(&ts)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("get last check-in: %w", err)
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse last check-in time: %w", err)
	}
	return t, nil
}

// SetLastCheckIn records the current check-in time, replacing any previous value.
func (s *SQLiteStore) SetLastCheckIn(ctx context.Context, t time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO check_in_state (id, checked_in_at) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET checked_in_at = excluded.checked_in_at`,
		t.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("set last check-in: %w", err)
	}
	return nil
}
