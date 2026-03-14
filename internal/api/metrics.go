package api

import (
	"net/http"
	"sync"
	"time"
)

// metricsResponse is the JSON body returned by GET /api/v1/metrics.
type metricsResponse struct {
	Pool          *poolMetricsDTO       `json:"pool"`
	Dispatcher    *dispatcherMetricsDTO `json:"dispatcher"`
	System        *systemMetricsDTO     `json:"system"`
	Pressure      pressureDTO           `json:"pressure"`
	ScalingEvents []scalingEventDTO     `json:"scaling_events"`
	ScalingConfig *scalingConfigDTO     `json:"scaling_config"`
	TaskProfiles  []taskProfileDTO      `json:"task_profiles"`
	Capacity      *capacityDTO          `json:"capacity"`
}

// ProviderDTO describes a single registered AI provider.
type ProviderDTO struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Model   string `json:"model"`
	Healthy bool   `json:"healthy"`
}

// capacityDTO describes the available AI providers and routing configuration.
type capacityDTO struct {
	Providers     []ProviderDTO       `json:"providers"`
	RoutingChains map[string][]string `json:"routing_chains"`
}

// historyEntry is a single timestamped snapshot stored in the history ring buffer.
type historyEntry struct {
	Timestamp  string                `json:"timestamp"`
	Pool       *poolMetricsDTO       `json:"pool"`
	Dispatcher *dispatcherMetricsDTO `json:"dispatcher"`
	System     *systemMetricsDTO     `json:"system"`
	Pressure   pressureDTO           `json:"pressure"`
}

// historyResponse is the JSON body returned by GET /api/v1/metrics/history.
type historyResponse struct {
	Duration string         `json:"duration"`
	Entries  []historyEntry `json:"entries"`
}

// historyMaxEntries is the maximum number of entries kept in the ring buffer.
// At a 30-second poll interval this covers ~30 minutes of history.
const historyMaxEntries = 60

// taskProfileDTO is the JSON-serializable form of models.TaskProfile.
// Duration fields are expressed in milliseconds.
type taskProfileDTO struct {
	AgentType     string  `json:"agent_type"`
	Provider      string  `json:"provider"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	P50DurationMs float64 `json:"p50_duration_ms"`
	P90DurationMs float64 `json:"p90_duration_ms"`
	SampleCount   int     `json:"sample_count"`
	AvgTokens     int     `json:"avg_tokens"`
	UpdatedAt     string  `json:"updated_at"`
}

// scalingEventDTO is the JSON-serializable form of a scaling.ScalingEvent.
type scalingEventDTO struct {
	Timestamp   string  `json:"timestamp"`
	Action      string  `json:"action"`
	FromWorkers int     `json:"from_workers"`
	ToWorkers   int     `json:"to_workers"`
	Reason      string  `json:"reason"`
	Confidence  float64 `json:"confidence"`
}

// scalingConfigDTO exposes the active scaling policy configuration.
type scalingConfigDTO struct {
	Enabled       bool `json:"enabled"`
	MinWorkers    int  `json:"min_workers"`
	MaxWorkers    int  `json:"max_workers"`
	CurrentTarget int  `json:"current_target"`
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

// historyMu guards history.
var historyMu sync.Mutex

// appendHistory adds a snapshot entry to the API's ring buffer (capacity historyMaxEntries).
func (a *API) appendHistory(entry historyEntry) {
	historyMu.Lock()
	defer historyMu.Unlock()
	if len(a.history) >= historyMaxEntries {
		// Shift: drop oldest entry.
		a.history = append(a.history[1:], entry)
	} else {
		a.history = append(a.history, entry)
	}
}

// historyBefore returns entries recorded no earlier than cutoff, oldest first.
func (a *API) historyBefore(cutoff time.Time) []historyEntry {
	historyMu.Lock()
	defer historyMu.Unlock()
	var out []historyEntry
	for _, e := range a.history {
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}
		if !ts.Before(cutoff) {
			out = append(out, e)
		}
	}
	return out
}

// Pressure returns the current pressure level string. It is used by the server's
// /healthz handler to surface resource pressure without importing the api package.
func (a *API) Pressure() string {
	var pool *poolMetricsDTO
	var sys *systemMetricsDTO
	if a.pool != nil {
		snap := a.pool.Snapshot()
		pool = &poolMetricsDTO{
			IdleWorkers: snap.IdleWorkers,
			QueueDepth:  snap.QueueDepth,
		}
	}
	if a.system != nil {
		snap := a.system.Collect()
		sys = &systemMetricsDTO{
			HeapAllocBytes: snap.HeapAllocBytes,
			SysBytesTotal:  snap.SysBytesTotal,
		}
	}
	return computePressure(pool, sys).Level
}

// handleMetrics serves GET /api/v1/metrics.
// Returns 200 with a JSON body. Any source that is nil contributes a null field.
// Each successful call appends a snapshot to the history ring buffer.
func (a *API) handleMetrics(w http.ResponseWriter, r *http.Request) {
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

	resp.Pressure = computePressure(resp.Pool, resp.System)

	if a.scalingEnabled && a.store != nil {
		events, evErr := a.store.ListScalingEvents(r.Context(), 20)
		if evErr == nil {
			dtos := make([]scalingEventDTO, 0, len(events))
			for _, e := range events {
				dtos = append(dtos, scalingEventDTO{
					Timestamp:   e.Timestamp.UTC().Format(time.RFC3339),
					Action:      e.Action,
					FromWorkers: e.FromWorkers,
					ToWorkers:   e.ToWorkers,
					Reason:      e.Reason,
					Confidence:  e.Confidence,
				})
			}
			resp.ScalingEvents = dtos
		}

		currentTarget := 0
		if a.pool != nil {
			currentTarget = a.pool.Snapshot().TotalWorkers
		}
		resp.ScalingConfig = &scalingConfigDTO{
			Enabled:       true,
			MinWorkers:    a.scalingMin,
			MaxWorkers:    a.scalingMax,
			CurrentTarget: currentTarget,
		}
	}

	if a.store != nil {
		profiles, profErr := a.store.ListTaskProfiles(r.Context())
		if profErr == nil && len(profiles) > 0 {
			dtos := make([]taskProfileDTO, 0, len(profiles))
			for _, p := range profiles {
				dtos = append(dtos, taskProfileDTO{
					AgentType:     p.AgentType,
					Provider:      p.Provider,
					AvgDurationMs: durationToMs(p.AvgDuration),
					P50DurationMs: durationToMs(p.P50Duration),
					P90DurationMs: durationToMs(p.P90Duration),
					SampleCount:   p.SampleCount,
					AvgTokens:     p.AvgTokens,
					UpdatedAt:     p.UpdatedAt.UTC().Format(time.RFC3339),
				})
			}
			resp.TaskProfiles = dtos
		}
	}

	resp.Capacity = a.capacity

	// Record snapshot in history ring buffer for /api/v1/metrics/history.
	a.appendHistory(historyEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Pool:       resp.Pool,
		Dispatcher: resp.Dispatcher,
		System:     resp.System,
		Pressure:   resp.Pressure,
	})

	writeJSON(w, http.StatusOK, resp)
}

// handleMetricsHistory serves GET /api/v1/metrics/history?duration=<duration>.
// duration defaults to 1h if omitted or unparseable.
// Returns entries from the in-memory ring buffer recorded within the requested window.
func (a *API) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	d := time.Hour
	if raw := r.URL.Query().Get("duration"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			d = parsed
		}
	}

	cutoff := time.Now().UTC().Add(-d)
	entries := a.historyBefore(cutoff)
	if entries == nil {
		entries = []historyEntry{}
	}

	writeJSON(w, http.StatusOK, historyResponse{
		Duration: d.String(),
		Entries:  entries,
	})
}

// durationToMs converts a time.Duration to milliseconds as a float64.
func durationToMs(d time.Duration) float64 {
	return float64(d.Nanoseconds()) / 1e6
}
