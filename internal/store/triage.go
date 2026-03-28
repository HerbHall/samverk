package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TriageDecision records the outcome of an autonomous triage evaluation.
type TriageDecision struct {
	ID               int64
	IssueNumber      int
	Project          string
	Verdict          string // "auto_unblock", "decompose", "confirmed_human"
	Reasoning        string
	CriteriaMet      string // JSON array of criteria results
	DocsReferenced   string // JSON array of docs consulted
	SubIssuesCreated string // JSON array of sub-issue numbers
	CreatedAt        time.Time
	ReviewedBy       string // NULL until human reviews
	ReviewOutcome    string // NULL, "correct", "incorrect", "adjusted"
}

// SaveTriageDecision inserts a triage decision record.
func (s *SQLiteStore) SaveTriageDecision(ctx context.Context, d *TriageDecision) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO triage_decisions (issue_number, project, verdict, reasoning, criteria_met, docs_referenced, sub_issues_created, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.IssueNumber, d.Project, d.Verdict, d.Reasoning,
		d.CriteriaMet, d.DocsReferenced, d.SubIssuesCreated,
		d.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("save triage decision: %w", err)
	}
	return nil
}

// ListTriageDecisions returns triage decisions since the given time, ordered by created_at descending.
// Use limit=0 for no row limit.
func (s *SQLiteStore) ListTriageDecisions(ctx context.Context, since time.Time, limit int) ([]TriageDecision, error) {
	sinceStr := since.UTC().Format(time.RFC3339)

	query := `SELECT id, issue_number, project, verdict, reasoning, criteria_met, docs_referenced, sub_issues_created, created_at, reviewed_by, review_outcome
	           FROM triage_decisions WHERE created_at >= ? ORDER BY created_at DESC`
	args := []any{sinceStr}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list triage decisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var decisions []TriageDecision
	for rows.Next() {
		var d TriageDecision
		var createdStr string
		var reviewedBy, reviewOutcome sql.NullString
		if scanErr := rows.Scan(
			&d.ID, &d.IssueNumber, &d.Project, &d.Verdict, &d.Reasoning,
			&d.CriteriaMet, &d.DocsReferenced, &d.SubIssuesCreated,
			&createdStr, &reviewedBy, &reviewOutcome,
		); scanErr != nil {
			return nil, fmt.Errorf("scan triage decision: %w", scanErr)
		}
		d.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		d.ReviewedBy = reviewedBy.String
		d.ReviewOutcome = reviewOutcome.String
		decisions = append(decisions, d)
	}
	return decisions, rows.Err()
}

// GetTriageDecisionForIssue returns the most recent triage decision for a given issue.
// Returns ErrNotFound if no decision exists.
func (s *SQLiteStore) GetTriageDecisionForIssue(ctx context.Context, issueNumber int) (*TriageDecision, error) {
	var d TriageDecision
	var createdStr string
	var reviewedBy, reviewOutcome sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT id, issue_number, project, verdict, reasoning, criteria_met, docs_referenced, sub_issues_created, created_at, reviewed_by, review_outcome
		 FROM triage_decisions WHERE issue_number = ? ORDER BY created_at DESC LIMIT 1`,
		issueNumber,
	).Scan(
		&d.ID, &d.IssueNumber, &d.Project, &d.Verdict, &d.Reasoning,
		&d.CriteriaMet, &d.DocsReferenced, &d.SubIssuesCreated,
		&createdStr, &reviewedBy, &reviewOutcome,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get triage decision for issue: %w", err)
	}
	d.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	d.ReviewedBy = reviewedBy.String
	d.ReviewOutcome = reviewOutcome.String
	return &d, nil
}

// UpdateTriageReview records a human review of a triage decision.
func (s *SQLiteStore) UpdateTriageReview(ctx context.Context, id int64, reviewedBy, outcome string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE triage_decisions SET reviewed_by = ?, review_outcome = ? WHERE id = ?`,
		reviewedBy, outcome, id,
	)
	if err != nil {
		return fmt.Errorf("update triage review: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
