package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

// SaveFailureEvent inserts a failure event into the failure_events table.
func (s *SQLiteStore) SaveFailureEvent(ctx context.Context, e *models.FailureEvent) error {
	if e.ID == "" {
		e.ID = generateID()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO failure_events (id, issue_number, session_id, failure_class, error_message, agent_type, provider, attempt_number, duration_ms, timestamp, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.IssueNumber, e.SessionID, string(e.FailureClass), e.ErrorMessage,
		e.AgentType, e.Provider, e.AttemptNumber,
		e.Duration.Milliseconds(), e.Timestamp.UTC().Format(time.RFC3339), now,
	)
	return err
}

// ListFailureEvents returns failure events since the given time, ordered by
// timestamp descending. Use limit=0 for no limit.
func (s *SQLiteStore) ListFailureEvents(ctx context.Context, since time.Time, limit int) ([]*models.FailureEvent, error) {
	query := `SELECT id, issue_number, session_id, failure_class, error_message, agent_type, provider, attempt_number, duration_ms, timestamp
	          FROM failure_events WHERE timestamp >= ? ORDER BY timestamp DESC`
	args := []interface{}{since.UTC().Format(time.RFC3339)}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var events []*models.FailureEvent
	for rows.Next() {
		e := &models.FailureEvent{}
		var (
			fc         string
			durationMs int64
			tsStr      string
		)
		if scanErr := rows.Scan(&e.ID, &e.IssueNumber, &e.SessionID, &fc, &e.ErrorMessage,
			&e.AgentType, &e.Provider, &e.AttemptNumber, &durationMs, &tsStr); scanErr != nil {
			return nil, scanErr
		}
		e.FailureClass = models.FailureClass(fc)
		e.Duration = time.Duration(durationMs) * time.Millisecond
		e.Timestamp, _ = time.Parse(time.RFC3339, tsStr)
		events = append(events, e)
	}
	return events, rows.Err()
}

// CountFailuresByIssue returns the total number of failure events for a given issue.
func (s *SQLiteStore) CountFailuresByIssue(ctx context.Context, issueNumber int) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM failure_events WHERE issue_number = ?`, issueNumber,
	).Scan(&count)
	return count, err
}

// RecentFailuresForIssue returns the most recent failure error messages for a
// given issue, ordered newest first. Used to enrich agent prompts with prior
// failure context so retries can avoid the same mistakes.
func (s *SQLiteStore) RecentFailuresForIssue(ctx context.Context, issueNumber, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 3
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT error_message FROM failure_events WHERE issue_number = ? ORDER BY timestamp DESC LIMIT ?`,
		issueNumber, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var messages []string
	for rows.Next() {
		var msg string
		if scanErr := rows.Scan(&msg); scanErr != nil {
			return nil, scanErr
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

// GetFailureSummary builds an aggregated failure summary for the given period.
func (s *SQLiteStore) GetFailureSummary(ctx context.Context, since time.Time) (*models.FailureSummary, error) {
	sinceStr := since.UTC().Format(time.RFC3339)
	summary := &models.FailureSummary{
		Since:          since,
		ByClass:        make(map[models.FailureClass]int),
		ProviderHealth: make(map[string]int),
	}

	// Total failures.
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM failure_events WHERE timestamp >= ?`, sinceStr,
	).Scan(&summary.TotalFailures); err != nil {
		return nil, err
	}

	// By class.
	if err := s.queryByClass(ctx, sinceStr, summary); err != nil {
		return nil, err
	}

	// Top issues by failure count.
	if err := s.queryTopIssues(ctx, sinceStr, summary); err != nil {
		return nil, err
	}

	// Provider health (failure count per provider).
	if err := s.queryProviderHealth(ctx, sinceStr, summary); err != nil {
		return nil, err
	}

	return summary, nil
}

func (s *SQLiteStore) queryByClass(ctx context.Context, sinceStr string, summary *models.FailureSummary) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT failure_class, COUNT(*) FROM failure_events WHERE timestamp >= ? GROUP BY failure_class ORDER BY COUNT(*) DESC`,
		sinceStr,
	)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var fc string
		var count int
		if scanErr := rows.Scan(&fc, &count); scanErr != nil {
			return scanErr
		}
		summary.ByClass[models.FailureClass(fc)] = count
	}
	return rows.Err()
}

func (s *SQLiteStore) queryTopIssues(ctx context.Context, sinceStr string, summary *models.FailureSummary) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT issue_number, COUNT(*) as cnt FROM failure_events WHERE timestamp >= ? GROUP BY issue_number ORDER BY cnt DESC LIMIT 10`,
		sinceStr,
	)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var ifc models.IssueFailureCount
		if scanErr := rows.Scan(&ifc.IssueNumber, &ifc.Count); scanErr != nil {
			return scanErr
		}
		summary.TopIssues = append(summary.TopIssues, ifc)
		if ifc.Count >= 5 {
			summary.LoopingIssues = append(summary.LoopingIssues, ifc)
		}
	}
	return rows.Err()
}

func (s *SQLiteStore) queryProviderHealth(ctx context.Context, sinceStr string, summary *models.FailureSummary) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT provider, COUNT(*) FROM failure_events WHERE timestamp >= ? AND provider != '' GROUP BY provider ORDER BY COUNT(*) DESC`,
		sinceStr,
	)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var prov string
		var count int
		if scanErr := rows.Scan(&prov, &count); scanErr != nil {
			return scanErr
		}
		summary.ProviderHealth[prov] = count
	}
	return rows.Err()
}

// GetIssueFailureCount returns the persisted failure count for an issue.
// Returns 0 if no record exists.
func (s *SQLiteStore) GetIssueFailureCount(ctx context.Context, issueNumber int) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT count FROM issue_failure_counts WHERE issue_number = ?`, issueNumber,
	).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return count, err
}

// IncrementIssueFailureCount atomically increments the failure count for an
// issue and returns the new value. Creates the record if it doesn't exist.
func (s *SQLiteStore) IncrementIssueFailureCount(ctx context.Context, issueNumber int) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO issue_failure_counts (issue_number, count, updated_at) VALUES (?, 1, ?)
		 ON CONFLICT(issue_number) DO UPDATE SET count = count + 1, updated_at = ?`,
		issueNumber, now, now,
	)
	if err != nil {
		return 0, err
	}
	return s.GetIssueFailureCount(ctx, issueNumber)
}

// ClearIssueFailureCount removes the failure count record for an issue.
func (s *SQLiteStore) ClearIssueFailureCount(ctx context.Context, issueNumber int) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM issue_failure_counts WHERE issue_number = ?`, issueNumber,
	)
	return err
}

// ResetAllFailureCounts removes all persisted failure counts, allowing the
// dispatcher to retry previously-escalated issues. Returns the number of
// records cleared.
func (s *SQLiteStore) ResetAllFailureCounts(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM issue_failure_counts`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
