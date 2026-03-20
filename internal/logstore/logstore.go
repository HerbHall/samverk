package logstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// LogEntry represents a single structured log record persisted to SQLite.
type LogEntry struct {
	ID          int64     `json:"id"`
	Timestamp   time.Time `json:"ts"`
	Level       string    `json:"level"`
	Message     string    `json:"msg"`
	Component   string    `json:"component,omitempty"`
	SessionID   string    `json:"session_id,omitempty"`
	IssueNumber int       `json:"issue_number,omitempty"`
	Fields      string    `json:"fields,omitempty"`
}

// QueryFilter controls which log entries are returned by Query.
type QueryFilter struct {
	Level     string
	Component string
	SessionID string
	Issue     int
	Since     time.Time
	Until     time.Time
	Search    string // substring match on msg
	Limit     int    // default 100, max 1000
	Offset    int    // for pagination
}

// LogStore provides SQLite-backed structured log persistence.
type LogStore struct {
	db *sql.DB
}

// New opens (or creates) a SQLite database at dbPath for log storage,
// enables WAL mode and busy_timeout, and runs migrations.
func New(dbPath string) (*LogStore, error) {
	// Set busy_timeout via DSN so it applies to ALL pooled connections,
	// not just the first one (PRAGMA only affects the connection it runs on).
	dsn := dbPath + "?_pragma=busy_timeout%3d5000&_pragma=journal_mode%3dWAL"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open log database: %w", err)
	}

	s := &LogStore{db: db}
	if err = s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// migrate creates the logs table and indexes if they do not exist.
func (s *LogStore) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ts TEXT NOT NULL,
	level TEXT NOT NULL,
	msg TEXT NOT NULL,
	component TEXT NOT NULL DEFAULT '',
	session_id TEXT NOT NULL DEFAULT '',
	issue_number INTEGER NOT NULL DEFAULT 0,
	fields TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_logs_ts ON logs(ts);
CREATE INDEX IF NOT EXISTS idx_logs_level ON logs(level);
CREATE INDEX IF NOT EXISTS idx_logs_session ON logs(session_id);
CREATE INDEX IF NOT EXISTS idx_logs_issue ON logs(issue_number);
`
	_, err := s.db.ExecContext(context.Background(), ddl)
	return err
}

// Insert writes a single log entry to SQLite.
func (s *LogStore) Insert(ctx context.Context, entry *LogEntry) error {
	fields := entry.Fields
	if fields == "" {
		fields = "{}"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO logs (ts, level, msg, component, session_id, issue_number, fields)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.Timestamp.UTC().Format(time.RFC3339Nano),
		entry.Level,
		entry.Message,
		entry.Component,
		entry.SessionID,
		entry.IssueNumber,
		fields,
	)
	if err != nil {
		return fmt.Errorf("insert log: %w", err)
	}
	return nil
}

// Query returns log entries matching the given filter, ordered by ts DESC, id DESC.
func (s *LogStore) Query(ctx context.Context, f QueryFilter) ([]LogEntry, error) {
	var (
		clauses []string
		args    []any
	)

	if f.Level != "" {
		clauses = append(clauses, "level = ?")
		args = append(args, f.Level)
	}
	if f.Component != "" {
		clauses = append(clauses, "component = ?")
		args = append(args, f.Component)
	}
	if f.SessionID != "" {
		clauses = append(clauses, "session_id = ?")
		args = append(args, f.SessionID)
	}
	if f.Issue != 0 {
		clauses = append(clauses, "issue_number = ?")
		args = append(args, f.Issue)
	}
	if !f.Since.IsZero() {
		clauses = append(clauses, "ts >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}
	if !f.Until.IsZero() {
		clauses = append(clauses, "ts <= ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339Nano))
	}
	if f.Search != "" {
		clauses = append(clauses, "msg LIKE ?")
		args = append(args, "%"+f.Search+"%")
	}

	query := "SELECT id, ts, level, msg, component, session_id, issue_number, fields FROM logs"
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ") //nolint:gosec // G202: clauses are static strings, user input is parameterized via ?
	}
	query += " ORDER BY ts DESC, id DESC"

	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	query += " LIMIT ?"
	args = append(args, limit)

	if f.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, f.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []LogEntry
	for rows.Next() {
		var e LogEntry
		var tsStr string
		if err := rows.Scan(&e.ID, &tsStr, &e.Level, &e.Message, &e.Component, &e.SessionID, &e.IssueNumber, &e.Fields); err != nil {
			return nil, fmt.Errorf("scan log row: %w", err)
		}
		e.Timestamp, err = time.Parse(time.RFC3339Nano, tsStr)
		if err != nil {
			// Best-effort: keep the entry with zero time rather than fail.
			e.Timestamp = time.Time{}
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate log rows: %w", err)
	}

	return entries, nil
}

// Prune deletes log entries older than the given retention duration.
// Returns the number of rows deleted.
func (s *LogStore) Prune(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan).UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, "DELETE FROM logs WHERE ts < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune logs: %w", err)
	}
	return result.RowsAffected()
}

// Count returns the total number of log entries in the store.
func (s *LogStore) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM logs").Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count logs: %w", err)
	}
	return n, nil
}

// Close closes the underlying database connection.
func (s *LogStore) Close() error {
	return s.db.Close()
}
