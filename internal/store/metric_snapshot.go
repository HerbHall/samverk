package store

import (
	"context"
	"time"
)

// MetricSnapshot holds the latest pool and dispatcher metrics persisted
// by the dispatch process for cross-process consumption by the serve process.
type MetricSnapshot struct {
	// Pool metrics
	CollectedAt       time.Time
	TotalWorkers      int
	ActiveWorkers     int
	IdleWorkers       int
	QueueDepth        int
	TasksCompleted    int64
	TasksFailed       int64
	AvgTaskDurationMs float64
	P95TaskDurationMs float64

	// Dispatcher metrics
	DispCollectedAt          time.Time
	ClaimedCount             int
	TotalRouted              int64
	TotalRequeued            int64
	TotalEventsProcessed     int64
	AvgPollLatencyMs         float64
	LastPollAt               time.Time
}

// SaveMetricSnapshot upserts the latest metric snapshot (single-row table).
func (s *SQLiteStore) SaveMetricSnapshot(ctx context.Context, m MetricSnapshot) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO metric_snapshots (id,
	collected_at, total_workers, active_workers, idle_workers, queue_depth,
	tasks_completed, tasks_failed, avg_task_duration_ms, p95_task_duration_ms,
	disp_collected_at, claimed_count, total_routed, total_requeued,
	total_events_processed, avg_poll_latency_ms, last_poll_at)
VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	collected_at=excluded.collected_at,
	total_workers=excluded.total_workers,
	active_workers=excluded.active_workers,
	idle_workers=excluded.idle_workers,
	queue_depth=excluded.queue_depth,
	tasks_completed=excluded.tasks_completed,
	tasks_failed=excluded.tasks_failed,
	avg_task_duration_ms=excluded.avg_task_duration_ms,
	p95_task_duration_ms=excluded.p95_task_duration_ms,
	disp_collected_at=excluded.disp_collected_at,
	claimed_count=excluded.claimed_count,
	total_routed=excluded.total_routed,
	total_requeued=excluded.total_requeued,
	total_events_processed=excluded.total_events_processed,
	avg_poll_latency_ms=excluded.avg_poll_latency_ms,
	last_poll_at=excluded.last_poll_at`,
		m.CollectedAt.UTC().Format(time.RFC3339),
		m.TotalWorkers, m.ActiveWorkers, m.IdleWorkers, m.QueueDepth,
		m.TasksCompleted, m.TasksFailed, m.AvgTaskDurationMs, m.P95TaskDurationMs,
		m.DispCollectedAt.UTC().Format(time.RFC3339),
		m.ClaimedCount, m.TotalRouted, m.TotalRequeued,
		m.TotalEventsProcessed, m.AvgPollLatencyMs,
		m.LastPollAt.UTC().Format(time.RFC3339),
	)
	return err
}

// LatestMetricSnapshot returns the most recent metric snapshot, or nil if none exists.
func (s *SQLiteStore) LatestMetricSnapshot(ctx context.Context) (*MetricSnapshot, error) {
	var m MetricSnapshot
	var collectedAt, dispCollectedAt, lastPollAt string
	err := s.db.QueryRowContext(ctx, `
SELECT collected_at, total_workers, active_workers, idle_workers, queue_depth,
	tasks_completed, tasks_failed, avg_task_duration_ms, p95_task_duration_ms,
	disp_collected_at, claimed_count, total_routed, total_requeued,
	total_events_processed, avg_poll_latency_ms, last_poll_at
FROM metric_snapshots WHERE id = 1`).Scan(
		&collectedAt,
		&m.TotalWorkers, &m.ActiveWorkers, &m.IdleWorkers, &m.QueueDepth,
		&m.TasksCompleted, &m.TasksFailed, &m.AvgTaskDurationMs, &m.P95TaskDurationMs,
		&dispCollectedAt,
		&m.ClaimedCount, &m.TotalRouted, &m.TotalRequeued,
		&m.TotalEventsProcessed, &m.AvgPollLatencyMs,
		&lastPollAt,
	)
	if err != nil {
		return nil, err
	}

	var parseErr error
	m.CollectedAt, parseErr = time.Parse(time.RFC3339, collectedAt)
	if parseErr != nil {
		return nil, parseErr
	}
	m.DispCollectedAt, parseErr = time.Parse(time.RFC3339, dispCollectedAt)
	if parseErr != nil {
		return nil, parseErr
	}
	m.LastPollAt, parseErr = time.Parse(time.RFC3339, lastPollAt)
	if parseErr != nil {
		return nil, parseErr
	}

	return &m, nil
}
