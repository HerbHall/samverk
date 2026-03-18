package store

import (
	"context"
	"time"

	"github.com/herbhall/samverk/internal/audit"
)

// SaveAuditResults persists a batch of provider audit results.
func (s *SQLiteStore) SaveAuditResults(ctx context.Context, results []audit.AuditResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO provider_audits (id, provider_name, provider_type, model, healthy, latency_ms, error, audited_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for i := range results {
		r := &results[i]
		healthy := 0
		if r.Healthy {
			healthy = 1
		}
		_, err = stmt.ExecContext(ctx,
			generateID(),
			r.ProviderName,
			r.ProviderType,
			r.Model,
			healthy,
			r.Latency.Milliseconds(),
			r.Error,
			r.AuditedAt.UTC().Format(time.RFC3339),
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetLastAudit returns the most recent audit results (all rows sharing the
// latest audited_at timestamp).
func (s *SQLiteStore) GetLastAudit(ctx context.Context) ([]audit.AuditResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT provider_name, provider_type, model, healthy, latency_ms, error, audited_at
		 FROM provider_audits
		 WHERE audited_at = (SELECT MAX(audited_at) FROM provider_audits)
		 ORDER BY provider_name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []audit.AuditResult
	for rows.Next() {
		var (
			r         audit.AuditResult
			healthy   int
			latencyMs int64
			tsStr     string
		)
		if scanErr := rows.Scan(&r.ProviderName, &r.ProviderType, &r.Model,
			&healthy, &latencyMs, &r.Error, &tsStr); scanErr != nil {
			return nil, scanErr
		}
		r.Healthy = healthy != 0
		r.Latency = time.Duration(latencyMs) * time.Millisecond
		r.AuditedAt, _ = time.Parse(time.RFC3339, tsStr)
		results = append(results, r)
	}
	return results, rows.Err()
}
