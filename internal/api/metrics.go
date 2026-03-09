package api

import (
	"net/http"
	"time"
)

// metricsResponse is the JSON body returned by GET /api/v1/metrics.
type metricsResponse struct {
	Pool       *poolMetricsDTO       `json:"pool"`
	Dispatcher *dispatcherMetricsDTO `json:"dispatcher"`
	System     *systemMetricsDTO     `json:"system"`
}

// poolMetricsDTO is the JSON-serializable form of metrics.PoolSnapshot.
// Duration fields are expressed in milliseconds for dashboard readability.
type poolMetricsDTO struct {
	CollectedAt       string  `json:"collected_at"`
	TotalWorkers      int     `json:"total_workers"`
	ActiveWorkers     int     `json:"active_workers"`
	IdleWorkers       int     `json:"idle_workers"`
	QueueDepth        int     `json:"queue_depth"`
	TasksCompleted    int64   `json:"tasks_completed"`
	TasksFailed       int64   `json:"tasks_failed"`
	AvgTaskDurationMs float64 `json:"avg_task_duration_ms"`
	P95TaskDurationMs float64 `json:"p95_task_duration_ms"`
}

// dispatcherMetricsDTO is the JSON-serializable form of metrics.DispatcherSnapshot.
type dispatcherMetricsDTO struct {
	CollectedAt          string  `json:"collected_at"`
	ClaimedCount         int     `json:"claimed_count"`
	TotalRouted          int64   `json:"total_routed"`
	TotalRequeued        int64   `json:"total_requeued"`
	TotalEventsProcessed int64   `json:"total_events_processed"`
	AvgPollLatencyMs     float64 `json:"avg_poll_latency_ms"`
	LastPollAt           string  `json:"last_poll_at"`
}

// systemMetricsDTO is the JSON-serializable form of metrics.SystemSnapshot.
type systemMetricsDTO struct {
	CollectedAt    string `json:"collected_at"`
	Goroutines     int    `json:"goroutines"`
	HeapAllocBytes uint64 `json:"heap_alloc_bytes"`
	SysBytesTotal  uint64 `json:"sys_bytes_total"`
	GCCycles       uint32 `json:"gc_cycles"`
	NextGCBytes    uint64 `json:"next_gc_bytes"`
}

// handleMetrics serves GET /api/v1/metrics.
// Returns 200 with a JSON body. Any source that is nil contributes a null field.
func (a *API) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	resp := metricsResponse{}

	if a.pool != nil {
		snap := a.pool.Snapshot()
		resp.Pool = &poolMetricsDTO{
			CollectedAt:       snap.CollectedAt.UTC().Format("2006-01-02T15:04:05Z"),
			TotalWorkers:      snap.TotalWorkers,
			ActiveWorkers:     snap.ActiveWorkers,
			IdleWorkers:       snap.IdleWorkers,
			QueueDepth:        snap.QueueDepth,
			TasksCompleted:    snap.TasksCompleted,
			TasksFailed:       snap.TasksFailed,
			AvgTaskDurationMs: durationToMs(snap.AvgTaskDuration),
			P95TaskDurationMs: durationToMs(snap.P95TaskDuration),
		}
	}

	if a.dispatcher != nil {
		snap := a.dispatcher.Snapshot()
		resp.Dispatcher = &dispatcherMetricsDTO{
			CollectedAt:          snap.CollectedAt.UTC().Format("2006-01-02T15:04:05Z"),
			ClaimedCount:         snap.ClaimedCount,
			TotalRouted:          snap.TotalRouted,
			TotalRequeued:        snap.TotalRequeued,
			TotalEventsProcessed: snap.TotalEventsProcessed,
			AvgPollLatencyMs:     durationToMs(snap.AvgPollLatency),
			LastPollAt:           snap.LastPollAt.UTC().Format("2006-01-02T15:04:05Z"),
		}
	}

	if a.system != nil {
		snap := a.system.Collect()
		resp.System = &systemMetricsDTO{
			CollectedAt:    snap.CollectedAt.UTC().Format("2006-01-02T15:04:05Z"),
			Goroutines:     snap.Goroutines,
			HeapAllocBytes: snap.HeapAllocBytes,
			SysBytesTotal:  snap.SysBytesTotal,
			GCCycles:       snap.GCCycles,
			NextGCBytes:    snap.NextGCBytes,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// durationToMs converts a time.Duration to milliseconds as a float64.
func durationToMs(d time.Duration) float64 {
	return float64(d.Nanoseconds()) / 1e6
}
