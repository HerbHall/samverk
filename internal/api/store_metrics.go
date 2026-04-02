package api

import (
	"context"
	"time"

	"samverk.dev/samverk/internal/metrics"
	"samverk.dev/samverk/internal/store"
)

// storeMetricsReader reads the latest metric snapshot from SQLite.
// It is the shared core for both pool and dispatcher metric sources.
type storeMetricsReader struct {
	store store.Store
}

func newStoreMetricsReader(s store.Store) *storeMetricsReader {
	return &storeMetricsReader{store: s}
}

func (r *storeMetricsReader) latest() (*store.MetricSnapshot, error) {
	return r.store.LatestMetricSnapshot(context.Background())
}

// hasData returns true if a metric snapshot exists and is recent (within 10 minutes).
func (r *storeMetricsReader) hasData() bool {
	snap, err := r.latest()
	if err != nil {
		return false
	}
	return time.Since(snap.CollectedAt) < 10*time.Minute
}

// StorePoolMetrics implements poolMetricsSource by reading from SQLite.
type StorePoolMetrics struct {
	reader *storeMetricsReader
}

// Snapshot returns a PoolSnapshot read from the latest persisted metric snapshot.
func (m *StorePoolMetrics) Snapshot() metrics.PoolSnapshot {
	snap, err := m.reader.latest()
	if err != nil {
		return metrics.PoolSnapshot{CollectedAt: time.Now()}
	}
	return metrics.PoolSnapshot{
		CollectedAt:     snap.CollectedAt,
		TotalWorkers:    snap.TotalWorkers,
		ActiveWorkers:   snap.ActiveWorkers,
		IdleWorkers:     snap.IdleWorkers,
		QueueDepth:      snap.QueueDepth,
		TasksCompleted:  snap.TasksCompleted,
		TasksFailed:     snap.TasksFailed,
		AvgTaskDuration: time.Duration(snap.AvgTaskDurationMs * float64(time.Millisecond)),
		P95TaskDuration: time.Duration(snap.P95TaskDurationMs * float64(time.Millisecond)),
	}
}

// StoreDispatcherMetrics implements dispatcherMetricsSource by reading from SQLite.
type StoreDispatcherMetrics struct {
	reader *storeMetricsReader
}

// Snapshot returns a DispatcherSnapshot read from the latest persisted metric snapshot.
func (m *StoreDispatcherMetrics) Snapshot() metrics.DispatcherSnapshot {
	snap, err := m.reader.latest()
	if err != nil {
		return metrics.DispatcherSnapshot{CollectedAt: time.Now()}
	}
	return metrics.DispatcherSnapshot{
		CollectedAt:          snap.DispCollectedAt,
		ClaimedCount:         snap.ClaimedCount,
		TotalRouted:          snap.TotalRouted,
		TotalRequeued:        snap.TotalRequeued,
		TotalEventsProcessed: snap.TotalEventsProcessed,
		AvgPollLatency:       time.Duration(snap.AvgPollLatencyMs * float64(time.Millisecond)),
		LastPollAt:           snap.LastPollAt,
	}
}

// NewStoreBackedMetrics creates pool and dispatcher metric sources that read
// from SQLite. This bridges the cross-process gap: the dispatch process writes
// snapshots, and the serve process reads them via these types.
// Returns nil, nil if the store has no recent metric data.
func NewStoreBackedMetrics(s store.Store) (pool *StorePoolMetrics, disp *StoreDispatcherMetrics) {
	reader := newStoreMetricsReader(s)
	return &StorePoolMetrics{reader: reader}, &StoreDispatcherMetrics{reader: reader}
}

// StoreMetricsHaveData returns true if the store contains a recent metric snapshot.
func StoreMetricsHaveData(s store.Store) bool {
	reader := newStoreMetricsReader(s)
	return reader.hasData()
}

// compile-time interface checks
var (
	_ poolMetricsSource       = (*StorePoolMetrics)(nil)
	_ dispatcherMetricsSource = (*StoreDispatcherMetrics)(nil)
)
