package store

import (
	"context"
	"database/sql"
	"fmt"

	"samverk.dev/samverk/pkg/models"
)

// GetKPIReport computes the rolling KPI report from the failure_events table.
// Rates are nil when sample_size < 5 (insufficient data).
func (s *SQLiteStore) GetKPIReport(ctx context.Context) (*models.KPIReport, error) {
	report := &models.KPIReport{
		WindowDays:         30,
		RootCauseBreakdown: []models.RootCauseBreakdownEntry{},
	}

	// Sample size: total events in the 30-day window.
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM failure_events WHERE created_at >= datetime('now', '-30 days')`,
	).Scan(&report.SampleSize); err != nil {
		return nil, fmt.Errorf("kpi sample size: %w", err)
	}

	// Root cause breakdown is always computed (even with small samples).
	if err := s.queryRootCauseBreakdown(ctx, report); err != nil {
		return nil, err
	}

	// Return nulls for all rates when data is insufficient.
	if report.SampleSize < 5 {
		return report, nil
	}

	if err := s.computeFirstTimeFixRate(ctx, report); err != nil {
		return nil, err
	}
	if err := s.computeFixSuccessRate(ctx, report); err != nil {
		return nil, err
	}
	if err := s.computeRecurrenceRate(ctx, report); err != nil {
		return nil, err
	}
	if err := s.computeWorkaroundRate(ctx, report); err != nil {
		return nil, err
	}
	if err := s.computeMeanTimeToFix(ctx, report); err != nil {
		return nil, err
	}
	if err := s.computePlanningGapRate(ctx, report); err != nil {
		return nil, err
	}
	if err := s.computeUnknownRootCauseRate(ctx, report); err != nil {
		return nil, err
	}
	if err := s.computePreventionCoverage(ctx, report); err != nil {
		return nil, err
	}

	return report, nil
}

func floatPtr(v float64) *float64 { return &v }

func (s *SQLiteStore) computeFirstTimeFixRate(ctx context.Context, report *models.KPIReport) error {
	var val sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `
SELECT
  CAST(SUM(CASE WHEN NOT EXISTS (
    SELECT 1 FROM failure_events f2
    WHERE f2.component = f1.component
      AND f2.root_cause_category = f1.root_cause_category
      AND f2.component != ''
      AND f2.root_cause_category != ''
      AND f2.created_at BETWEEN f1.resolved_at AND datetime(f1.resolved_at, '+30 days')
  ) THEN 1 ELSE 0 END) AS REAL) / COUNT(*) * 100
FROM failure_events f1
WHERE f1.resolved_at IS NOT NULL
  AND f1.resolved_at >= datetime('now', '-30 days')
`).Scan(&val)
	if err != nil {
		return fmt.Errorf("kpi ftfr: %w", err)
	}
	if val.Valid {
		report.FirstTimeFixRate = floatPtr(val.Float64)
	}
	return nil
}

func (s *SQLiteStore) computeFixSuccessRate(ctx context.Context, report *models.KPIReport) error {
	var val sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `
SELECT
  CAST(SUM(CASE WHEN status = 'resolved' THEN 1 ELSE 0 END) AS REAL)
    / COUNT(*) * 100
FROM failure_events
WHERE created_at >= datetime('now', '-7 days')
`).Scan(&val)
	if err != nil {
		return fmt.Errorf("kpi fix_success_rate: %w", err)
	}
	if val.Valid {
		report.FixSuccessRate = floatPtr(val.Float64)
	}
	return nil
}

func (s *SQLiteStore) computeRecurrenceRate(ctx context.Context, report *models.KPIReport) error {
	var val sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `
WITH recurrences AS (
  SELECT DISTINCT f2.id
  FROM failure_events f1
  JOIN failure_events f2
    ON f2.component = f1.component
   AND f2.root_cause_category = f1.root_cause_category
   AND f2.component != ''
   AND f2.root_cause_category != ''
   AND f2.id != f1.id
   AND f2.created_at > f1.resolved_at
   AND f2.created_at <= datetime(f1.resolved_at, '+30 days')
  WHERE f1.resolved_at >= datetime('now', '-30 days')
)
SELECT
  CAST(COUNT(DISTINCT r.id) AS REAL)
    / NULLIF((SELECT COUNT(*) FROM failure_events WHERE created_at >= datetime('now', '-30 days')), 0)
    * 100
FROM recurrences r
`).Scan(&val)
	if err != nil {
		return fmt.Errorf("kpi recurrence_rate: %w", err)
	}
	if val.Valid {
		report.RecurrenceRate = floatPtr(val.Float64)
	} else {
		report.RecurrenceRate = floatPtr(0)
	}
	return nil
}

func (s *SQLiteStore) computeWorkaroundRate(ctx context.Context, report *models.KPIReport) error {
	var val sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `
SELECT
  CAST(SUM(CASE WHEN fix_classification = 'workaround' THEN 1 ELSE 0 END) AS REAL)
    / COUNT(*) * 100
FROM failure_events
WHERE created_at >= datetime('now', '-30 days')
`).Scan(&val)
	if err != nil {
		return fmt.Errorf("kpi workaround_rate: %w", err)
	}
	if val.Valid {
		report.WorkaroundRate = floatPtr(val.Float64)
	}
	return nil
}

func (s *SQLiteStore) computeMeanTimeToFix(ctx context.Context, report *models.KPIReport) error {
	var val sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `
SELECT AVG(
  CAST((julianday(resolved_at) - julianday(created_at)) * 24 AS REAL)
)
FROM failure_events
WHERE resolved_at IS NOT NULL
  AND created_at >= datetime('now', '-7 days')
`).Scan(&val)
	if err != nil {
		return fmt.Errorf("kpi mttf: %w", err)
	}
	if val.Valid {
		report.MeanTimeToFixHours = floatPtr(val.Float64)
	}
	return nil
}

func (s *SQLiteStore) computePlanningGapRate(ctx context.Context, report *models.KPIReport) error {
	var val sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `
SELECT
  CAST(SUM(CASE WHEN root_cause_category = 'planning_gap' THEN 1 ELSE 0 END) AS REAL)
    / COUNT(*) * 100
FROM failure_events
WHERE created_at >= datetime('now', '-30 days')
`).Scan(&val)
	if err != nil {
		return fmt.Errorf("kpi planning_gap_rate: %w", err)
	}
	if val.Valid {
		report.PlanningGapRate = floatPtr(val.Float64)
	}
	return nil
}

func (s *SQLiteStore) computeUnknownRootCauseRate(ctx context.Context, report *models.KPIReport) error {
	var val sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `
SELECT
  CAST(SUM(CASE WHEN root_cause_category = 'unknown' THEN 1 ELSE 0 END) AS REAL)
    / COUNT(*) * 100
FROM failure_events
WHERE created_at >= datetime('now', '-30 days')
`).Scan(&val)
	if err != nil {
		return fmt.Errorf("kpi unknown_root_cause_rate: %w", err)
	}
	if val.Valid {
		report.UnknownRootCauseRate = floatPtr(val.Float64)
	}
	return nil
}

func (s *SQLiteStore) computePreventionCoverage(ctx context.Context, report *models.KPIReport) error {
	var val sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `
SELECT
  CAST(SUM(CASE WHEN prevention_measure != '' AND prevention_measure != 'none' THEN 1 ELSE 0 END) AS REAL)
    / COUNT(*) * 100
FROM failure_events
WHERE created_at >= datetime('now', '-30 days')
`).Scan(&val)
	if err != nil {
		return fmt.Errorf("kpi prevention_coverage: %w", err)
	}
	if val.Valid {
		report.PreventionCoverage = floatPtr(val.Float64)
	}
	return nil
}

func (s *SQLiteStore) queryRootCauseBreakdown(ctx context.Context, report *models.KPIReport) error {
	rows, err := s.db.QueryContext(ctx, `
SELECT root_cause_category,
       COUNT(*) AS cnt,
       CAST(COUNT(*) AS REAL) / SUM(COUNT(*)) OVER () * 100 AS pct
FROM failure_events
WHERE created_at >= datetime('now', '-30 days')
  AND root_cause_category != ''
GROUP BY root_cause_category
ORDER BY cnt DESC
`)
	if err != nil {
		return fmt.Errorf("kpi root_cause_breakdown: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var entry models.RootCauseBreakdownEntry
		if scanErr := rows.Scan(&entry.Category, &entry.Count, &entry.Pct); scanErr != nil {
			return fmt.Errorf("kpi root_cause_breakdown scan: %w", scanErr)
		}
		report.RootCauseBreakdown = append(report.RootCauseBreakdown, entry)
	}
	return rows.Err()
}

// ReadinessGapRate returns the percentage of failure events with root_cause_category = 'readiness_gap'
// in the given number of days. Returns 0 if no events exist.
func (s *SQLiteStore) ReadinessGapRate(ctx context.Context, days int) (float64, error) {
	var val sql.NullFloat64
	dayStr := fmt.Sprintf("-%d days", days)
	err := s.db.QueryRowContext(ctx, `
SELECT
  CAST(SUM(CASE WHEN root_cause_category = 'readiness_gap' THEN 1 ELSE 0 END) AS REAL)
    / NULLIF(COUNT(*), 0) * 100
FROM failure_events
WHERE created_at >= datetime('now', ?)
`, dayStr).Scan(&val)
	if err != nil {
		return 0, fmt.Errorf("readiness gap rate: %w", err)
	}
	if val.Valid {
		return val.Float64, nil
	}
	return 0, nil
}

// PlanningEscalationRate returns the percentage of failure events with root_cause_category = 'planning_gap'
// in the given number of days. Returns 0 if no events exist.
func (s *SQLiteStore) PlanningEscalationRate(ctx context.Context, days int) (float64, error) {
	var val sql.NullFloat64
	dayStr := fmt.Sprintf("-%d days", days)
	err := s.db.QueryRowContext(ctx, `
SELECT
  CAST(SUM(CASE WHEN root_cause_category = 'planning_gap' THEN 1 ELSE 0 END) AS REAL)
    / NULLIF(COUNT(*), 0) * 100
FROM failure_events
WHERE created_at >= datetime('now', ?)
`, dayStr).Scan(&val)
	if err != nil {
		return 0, fmt.Errorf("planning escalation rate: %w", err)
	}
	if val.Valid {
		return val.Float64, nil
	}
	return 0, nil
}

// ImpactConflictRate returns the percentage of failure events with root_cause_category = 'impact_conflict'
// in the given number of days. Returns 0 if no events exist.
func (s *SQLiteStore) ImpactConflictRate(ctx context.Context, days int) (float64, error) {
	var val sql.NullFloat64
	dayStr := fmt.Sprintf("-%d days", days)
	err := s.db.QueryRowContext(ctx, `
SELECT
  CAST(SUM(CASE WHEN root_cause_category = 'impact_conflict' THEN 1 ELSE 0 END) AS REAL)
    / NULLIF(COUNT(*), 0) * 100
FROM failure_events
WHERE created_at >= datetime('now', ?)
`, dayStr).Scan(&val)
	if err != nil {
		return 0, fmt.Errorf("impact conflict rate: %w", err)
	}
	if val.Valid {
		return val.Float64, nil
	}
	return 0, nil
}
