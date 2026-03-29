package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CachedIssue is the data written to the issue_cache table.
// Callers convert from forge.Issue before passing to SyncIssues.
type CachedIssue struct {
	Number    int
	Title     string
	State     string // "open" or "closed"
	Labels    []string
	Assignees []string
	CreatedAt time.Time
	UpdatedAt time.Time
	ClosedAt  *time.Time
}

// IssueCounts holds open/closed totals for a single project.
type IssueCounts struct {
	Open   int `json:"open"`
	Closed int `json:"closed"`
	Total  int `json:"total"`
}

// SyncIssues upserts a batch of issues into the cache for the given project.
// Uses a transaction for atomicity.
func (s *SQLiteStore) SyncIssues(ctx context.Context, project string, issues []CachedIssue) error {
	if len(issues) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sync issues: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO issue_cache (number, project, title, state, labels, assignees, created_at, updated_at, closed_at, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project, number) DO UPDATE SET
			title     = excluded.title,
			state     = excluded.state,
			labels    = excluded.labels,
			assignees = excluded.assignees,
			updated_at = excluded.updated_at,
			closed_at  = excluded.closed_at,
			synced_at  = excluded.synced_at
	`)
	if err != nil {
		return fmt.Errorf("sync issues: prepare: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now().UTC().Format(time.RFC3339)

	for i := range issues {
		iss := &issues[i]

		labelsJSON, _ := json.Marshal(iss.Labels)
		assigneesJSON, _ := json.Marshal(iss.Assignees)

		var closedAt *string
		if iss.ClosedAt != nil {
			s := iss.ClosedAt.UTC().Format(time.RFC3339)
			closedAt = &s
		}

		if _, err := stmt.ExecContext(ctx,
			iss.Number,
			project,
			iss.Title,
			iss.State,
			string(labelsJSON),
			string(assigneesJSON),
			iss.CreatedAt.UTC().Format(time.RFC3339),
			iss.UpdatedAt.UTC().Format(time.RFC3339),
			closedAt,
			now,
		); err != nil {
			return fmt.Errorf("sync issues: upsert #%d: %w", iss.Number, err)
		}
	}

	return tx.Commit()
}

// DeleteStaleIssues removes cached open issues that are no longer present on
// the forge. currentNumbers is the full set of open issue numbers from the
// latest poll. Any cached open issue NOT in this set is deleted (it was closed
// or deleted on the forge and will be re-synced on the next closed-issue sync).
func (s *SQLiteStore) DeleteStaleIssues(ctx context.Context, project string, currentNumbers []int) error {
	if len(currentNumbers) == 0 {
		// If zero open issues, delete all cached open issues for this project.
		_, err := s.db.ExecContext(ctx,
			`DELETE FROM issue_cache WHERE project = ? AND state = 'open'`,
			project,
		)
		return err
	}

	// Build placeholders for the IN clause.
	placeholders := make([]string, len(currentNumbers))
	args := make([]any, 0, len(currentNumbers)+1)
	args = append(args, project)
	for i, n := range currentNumbers {
		placeholders[i] = "?"
		args = append(args, n)
	}

	query := fmt.Sprintf( //nolint:gosec // G201: placeholders are literal "?" strings, not user input
		`DELETE FROM issue_cache WHERE project = ? AND state = 'open' AND number NOT IN (%s)`,
		strings.Join(placeholders, ","),
	)

	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

// GetIssueCounts returns the open and closed issue counts for a project from cache.
func (s *SQLiteStore) GetIssueCounts(ctx context.Context, project string) (open, closed int, err error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT state, COUNT(*) FROM issue_cache WHERE project = ? GROUP BY state`,
		project,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("get issue counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return 0, 0, fmt.Errorf("get issue counts: scan: %w", err)
		}
		switch state {
		case "open":
			open = count
		case "closed":
			closed = count
		}
	}

	return open, closed, rows.Err()
}

// GetAllIssueCounts returns counts for every project in the cache.
func (s *SQLiteStore) GetAllIssueCounts(ctx context.Context) (map[string]IssueCounts, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT project, state, COUNT(*) FROM issue_cache GROUP BY project, state`,
	)
	if err != nil {
		return nil, fmt.Errorf("get all issue counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]IssueCounts)
	for rows.Next() {
		var project, state string
		var count int
		if err := rows.Scan(&project, &state, &count); err != nil {
			return nil, fmt.Errorf("get all issue counts: scan: %w", err)
		}
		c := result[project]
		switch state {
		case "open":
			c.Open = count
		case "closed":
			c.Closed = count
		}
		c.Total = c.Open + c.Closed
		result[project] = c
	}

	return result, rows.Err()
}

// GetIssueLabelCounts returns a map of label -> count for open issues in a project.
func (s *SQLiteStore) GetIssueLabelCounts(ctx context.Context, project string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT labels FROM issue_cache WHERE project = ? AND state = 'open'`,
		project,
	)
	if err != nil {
		return nil, fmt.Errorf("get issue label counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[string]int)
	for rows.Next() {
		var labelsJSON string
		if err := rows.Scan(&labelsJSON); err != nil {
			return nil, fmt.Errorf("get issue label counts: scan: %w", err)
		}
		var labels []string
		if err := json.Unmarshal([]byte(labelsJSON), &labels); err != nil {
			continue // skip malformed rows
		}
		for _, l := range labels {
			counts[l]++
		}
	}

	return counts, rows.Err()
}

// GetCacheSyncTime returns the most recent synced_at time for a project.
func (s *SQLiteStore) GetCacheSyncTime(ctx context.Context, project string) (time.Time, error) {
	var ts string
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(synced_at) FROM issue_cache WHERE project = ?`,
		project,
	).Scan(&ts)
	if err != nil || ts == "" {
		if err == sql.ErrNoRows || ts == "" {
			return time.Time{}, ErrNotFound
		}
		return time.Time{}, fmt.Errorf("get cache sync time: %w", err)
	}

	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse cache sync time: %w", err)
	}
	return t, nil
}
