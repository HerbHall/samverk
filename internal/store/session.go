package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

// CreateSession inserts a new session record. If s.ID is empty, a random ID is generated.
func (s *SQLiteStore) CreateSession(ctx context.Context, sess *models.Session) error {
	if sess.ID == "" {
		sess.ID = generateID()
	}
	now := time.Now().UTC()
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = now
	}
	if sess.UpdatedAt.IsZero() {
		sess.UpdatedAt = now
	}

	var finishedAt sql.NullString
	if sess.FinishedAt != nil {
		finishedAt = sql.NullString{String: sess.FinishedAt.Format(time.RFC3339), Valid: true}
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, issue_number, agent_type, provider, model, status, started_at, finished_at, error, estimated_timeout_ms, partial_output, checkpoint_hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID,
		sess.IssueNumber,
		sess.AgentType,
		sess.Provider,
		sess.Model,
		string(sess.Status),
		sess.StartedAt.Format(time.RFC3339),
		finishedAt,
		sess.Error,
		sess.EstimatedTimeout.Milliseconds(),
		sess.PartialOutput,
		sess.CheckpointHash,
		sess.CreatedAt.Format(time.RFC3339),
		sess.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// GetSession retrieves a session by ID. Returns ErrNotFound if no row matches.
func (s *SQLiteStore) GetSession(ctx context.Context, id string) (*models.Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, issue_number, agent_type, provider, model, status, started_at, finished_at, error, estimated_timeout_ms, partial_output, checkpoint_hash, created_at, updated_at
		 FROM sessions WHERE id = ?`, id)

	sess, err := scanSession(row)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// UpdateSession updates an existing session record. The ID must already exist.
func (s *SQLiteStore) UpdateSession(ctx context.Context, sess *models.Session) error {
	sess.UpdatedAt = time.Now().UTC()

	var finishedAt sql.NullString
	if sess.FinishedAt != nil {
		finishedAt = sql.NullString{String: sess.FinishedAt.Format(time.RFC3339), Valid: true}
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET issue_number=?, agent_type=?, provider=?, model=?, status=?, started_at=?, finished_at=?, error=?, estimated_timeout_ms=?, partial_output=?, checkpoint_hash=?, updated_at=?
		 WHERE id=?`,
		sess.IssueNumber,
		sess.AgentType,
		sess.Provider,
		sess.Model,
		string(sess.Status),
		sess.StartedAt.Format(time.RFC3339),
		finishedAt,
		sess.Error,
		sess.EstimatedTimeout.Milliseconds(),
		sess.PartialOutput,
		sess.CheckpointHash,
		sess.UpdatedAt.Format(time.RFC3339),
		sess.ID,
	)
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ListSessions returns all sessions matching the given status.
// Pass an empty status to list all sessions.
func (s *SQLiteStore) ListSessions(ctx context.Context, status models.SessionStatus) ([]*models.Session, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if status != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, issue_number, agent_type, provider, model, status, started_at, finished_at, error, estimated_timeout_ms, partial_output, checkpoint_hash, created_at, updated_at
			 FROM sessions WHERE status = ? ORDER BY created_at DESC`, string(status))
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, issue_number, agent_type, provider, model, status, started_at, finished_at, error, estimated_timeout_ms, partial_output, checkpoint_hash, created_at, updated_at
			 FROM sessions ORDER BY created_at DESC`)
	}
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]*models.Session, 0)
	for rows.Next() {
		sess, scanErr := scanSessionRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, sess)
	}
	return result, rows.Err()
}

// LatestSuccessByProvider returns the most recent successful session completion
// timestamp per provider name. Providers with no successful sessions are omitted.
func (s *SQLiteStore) LatestSuccessByProvider(ctx context.Context) (map[string]time.Time, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT provider, MAX(finished_at) FROM sessions
		 WHERE status = 'completed' AND finished_at IS NOT NULL
		 GROUP BY provider`)
	if err != nil {
		return nil, fmt.Errorf("query latest success by provider: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]time.Time)
	for rows.Next() {
		var providerName, finishedAt string
		if scanErr := rows.Scan(&providerName, &finishedAt); scanErr != nil {
			return nil, fmt.Errorf("scan provider success row: %w", scanErr)
		}
		t, parseErr := time.Parse(time.RFC3339, finishedAt)
		if parseErr != nil {
			return nil, fmt.Errorf("parse finished_at for provider %q: %w", providerName, parseErr)
		}
		result[providerName] = t
	}
	return result, rows.Err()
}

// scanSession scans a single session from a *sql.Row.
func scanSession(row *sql.Row) (*models.Session, error) {
	var (
		sess               models.Session
		status             string
		startedAt          string
		finishedAt         sql.NullString
		estimatedTimeoutMs int64
		createdAt          string
		updatedAt          string
	)

	err := row.Scan(
		&sess.ID, &sess.IssueNumber, &sess.AgentType, &sess.Provider, &sess.Model,
		&status, &startedAt, &finishedAt, &sess.Error, &estimatedTimeoutMs, &sess.PartialOutput, &sess.CheckpointHash, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}

	sess.Status = models.SessionStatus(status)
	sess.EstimatedTimeout = time.Duration(estimatedTimeoutMs) * time.Millisecond
	if sess.StartedAt, err = time.Parse(time.RFC3339, startedAt); err != nil {
		return nil, fmt.Errorf("parse started_at: %w", err)
	}
	if finishedAt.Valid {
		t, parseErr := time.Parse(time.RFC3339, finishedAt.String)
		if parseErr != nil {
			return nil, fmt.Errorf("parse finished_at: %w", parseErr)
		}
		sess.FinishedAt = &t
	}
	if sess.CreatedAt, err = time.Parse(time.RFC3339, createdAt); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if sess.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &sess, nil
}

// scanSessionRows scans a single session from *sql.Rows.
func scanSessionRows(rows *sql.Rows) (*models.Session, error) {
	var (
		sess               models.Session
		status             string
		startedAt          string
		finishedAt         sql.NullString
		estimatedTimeoutMs int64
		createdAt          string
		updatedAt          string
	)

	err := rows.Scan(
		&sess.ID, &sess.IssueNumber, &sess.AgentType, &sess.Provider, &sess.Model,
		&status, &startedAt, &finishedAt, &sess.Error, &estimatedTimeoutMs, &sess.PartialOutput, &sess.CheckpointHash, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan session row: %w", err)
	}

	sess.Status = models.SessionStatus(status)
	sess.EstimatedTimeout = time.Duration(estimatedTimeoutMs) * time.Millisecond
	if sess.StartedAt, err = time.Parse(time.RFC3339, startedAt); err != nil {
		return nil, fmt.Errorf("parse started_at: %w", err)
	}
	if finishedAt.Valid {
		t, parseErr := time.Parse(time.RFC3339, finishedAt.String)
		if parseErr != nil {
			return nil, fmt.Errorf("parse finished_at: %w", parseErr)
		}
		sess.FinishedAt = &t
	}
	if sess.CreatedAt, err = time.Parse(time.RFC3339, createdAt); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if sess.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &sess, nil
}
