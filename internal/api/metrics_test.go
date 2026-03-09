package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/api"
	"github.com/herbhall/samverk/internal/metrics"
)

// makeMetricsServer creates an httptest.Server for an already-configured API.
func makeMetricsServer(t *testing.T, a *api.API) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	a.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestHandleMetrics_AllNil(t *testing.T) {
	// Do not call SetMetrics -- all three sources are nil interfaces by default.
	a := api.New(nil, nil, nil, zap.NewNop())
	ts := makeMetricsServer(t, a)

	resp := doGet(t, ts.URL+"/api/v1/metrics")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, field := range []string{"pool", "dispatcher", "system"} {
		val := string(body[field])
		if val != "null" {
			t.Errorf("%s = %s, want null", field, val)
		}
	}
}

func TestHandleMetrics_Pool(t *testing.T) {
	pm := metrics.NewPoolMetrics(4)
	// Simulate a completed task so avg duration is non-zero.
	pm.WorkerStarted()
	pm.WorkerFinished(50_000_000, metrics.TaskResultSuccess) // 50ms

	a := api.New(nil, nil, nil, zap.NewNop())
	a.SetMetrics(pm, nil, nil) // nil literals are untyped: safe nil interfaces
	ts := makeMetricsServer(t, a)

	resp := doGet(t, ts.URL+"/api/v1/metrics")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Pool *struct {
			CollectedAt       string  `json:"collected_at"`
			TotalWorkers      int     `json:"total_workers"`
			ActiveWorkers     int     `json:"active_workers"`
			IdleWorkers       int     `json:"idle_workers"`
			TasksCompleted    int64   `json:"tasks_completed"`
			TasksFailed       int64   `json:"tasks_failed"`
			AvgTaskDurationMs float64 `json:"avg_task_duration_ms"`
		} `json:"pool"`
		Dispatcher *struct{} `json:"dispatcher"`
		System     *struct{} `json:"system"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Pool == nil {
		t.Fatal("pool = nil, want non-nil")
	}
	if body.Pool.CollectedAt == "" {
		t.Error("pool.collected_at is empty")
	}
	if body.Pool.TotalWorkers != 4 {
		t.Errorf("total_workers = %d, want 4", body.Pool.TotalWorkers)
	}
	if body.Pool.ActiveWorkers != 0 {
		t.Errorf("active_workers = %d, want 0", body.Pool.ActiveWorkers)
	}
	if body.Pool.IdleWorkers != 4 {
		t.Errorf("idle_workers = %d, want 4", body.Pool.IdleWorkers)
	}
	if body.Pool.TasksCompleted != 1 {
		t.Errorf("tasks_completed = %d, want 1", body.Pool.TasksCompleted)
	}
	if body.Pool.TasksFailed != 0 {
		t.Errorf("tasks_failed = %d, want 0", body.Pool.TasksFailed)
	}
	// 50ms -> 50.0ms
	if body.Pool.AvgTaskDurationMs < 49 || body.Pool.AvgTaskDurationMs > 51 {
		t.Errorf("avg_task_duration_ms = %v, want ~50", body.Pool.AvgTaskDurationMs)
	}
	if body.Dispatcher != nil {
		t.Error("dispatcher = non-nil, want null")
	}
	if body.System != nil {
		t.Error("system = non-nil, want null")
	}
}

func TestHandleMetrics_Dispatcher(t *testing.T) {
	dm := metrics.NewDispatcherMetrics()
	dm.IssueRouted()
	dm.IssueRouted()
	dm.IssueRequeued()
	dm.EventProcessed()
	dm.PollCompleted(10_000_000) // 10ms

	a := api.New(nil, nil, nil, zap.NewNop())
	a.SetMetrics(nil, dm, nil) // nil literals are untyped: safe nil interfaces
	ts := makeMetricsServer(t, a)

	resp := doGet(t, ts.URL+"/api/v1/metrics")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Pool       *struct{} `json:"pool"`
		Dispatcher *struct {
			CollectedAt          string  `json:"collected_at"`
			TotalRouted          int64   `json:"total_routed"`
			TotalRequeued        int64   `json:"total_requeued"`
			TotalEventsProcessed int64   `json:"total_events_processed"`
			AvgPollLatencyMs     float64 `json:"avg_poll_latency_ms"`
			LastPollAt           string  `json:"last_poll_at"`
		} `json:"dispatcher"`
		System *struct{} `json:"system"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Pool != nil {
		t.Error("pool = non-nil, want null")
	}
	if body.Dispatcher == nil {
		t.Fatal("dispatcher = nil, want non-nil")
	}
	if body.Dispatcher.TotalRouted != 2 {
		t.Errorf("total_routed = %d, want 2", body.Dispatcher.TotalRouted)
	}
	if body.Dispatcher.TotalRequeued != 1 {
		t.Errorf("total_requeued = %d, want 1", body.Dispatcher.TotalRequeued)
	}
	if body.Dispatcher.TotalEventsProcessed != 1 {
		t.Errorf("total_events_processed = %d, want 1", body.Dispatcher.TotalEventsProcessed)
	}
	// 10ms -> 10.0ms
	if body.Dispatcher.AvgPollLatencyMs < 9 || body.Dispatcher.AvgPollLatencyMs > 11 {
		t.Errorf("avg_poll_latency_ms = %v, want ~10", body.Dispatcher.AvgPollLatencyMs)
	}
	if body.System != nil {
		t.Error("system = non-nil, want null")
	}
}

func TestHandleMetrics_System(t *testing.T) {
	sc := metrics.NewSystemCollector()

	a := api.New(nil, nil, nil, zap.NewNop())
	a.SetMetrics(nil, nil, sc) // nil literals are untyped: safe nil interfaces
	ts := makeMetricsServer(t, a)

	resp := doGet(t, ts.URL+"/api/v1/metrics")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Pool       *struct{} `json:"pool"`
		Dispatcher *struct{} `json:"dispatcher"`
		System     *struct {
			CollectedAt    string `json:"collected_at"`
			Goroutines     int    `json:"goroutines"`
			HeapAllocBytes uint64 `json:"heap_alloc_bytes"`
			SysBytesTotal  uint64 `json:"sys_bytes_total"`
		} `json:"system"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Pool != nil {
		t.Error("pool = non-nil, want null")
	}
	if body.Dispatcher != nil {
		t.Error("dispatcher = non-nil, want null")
	}
	if body.System == nil {
		t.Fatal("system = nil, want non-nil")
	}
	if body.System.CollectedAt == "" {
		t.Error("system.collected_at is empty")
	}
	if body.System.Goroutines <= 0 {
		t.Errorf("goroutines = %d, want > 0", body.System.Goroutines)
	}
	if body.System.HeapAllocBytes == 0 {
		t.Error("heap_alloc_bytes = 0, want > 0")
	}
	if body.System.SysBytesTotal == 0 {
		t.Error("sys_bytes_total = 0, want > 0")
	}
}

func TestHandleMetrics_AllSources(t *testing.T) {
	pm := metrics.NewPoolMetrics(2)
	dm := metrics.NewDispatcherMetrics()
	sc := metrics.NewSystemCollector()

	a := api.New(nil, nil, nil, zap.NewNop())
	a.SetMetrics(pm, dm, sc)
	ts := makeMetricsServer(t, a)

	resp := doGet(t, ts.URL+"/api/v1/metrics")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Pool       *json.RawMessage `json:"pool"`
		Dispatcher *json.RawMessage `json:"dispatcher"`
		System     *json.RawMessage `json:"system"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Pool == nil {
		t.Error("pool = null, want non-null")
	}
	if body.Dispatcher == nil {
		t.Error("dispatcher = null, want non-null")
	}
	if body.System == nil {
		t.Error("system = null, want non-null")
	}
}

func TestHandleMetrics_PressureField(t *testing.T) {
	// With no pool or system source, pressure must be "low" with no reasons.
	a := api.New(nil, nil, nil, zap.NewNop())
	ts := makeMetricsServer(t, a)

	resp := doGet(t, ts.URL+"/api/v1/metrics")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Pressure struct {
			Level   string   `json:"level"`
			Reasons []string `json:"reasons"`
		} `json:"pressure"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Pressure.Level != "low" {
		t.Errorf("pressure.level = %q, want low", body.Pressure.Level)
	}
	if len(body.Pressure.Reasons) != 0 {
		t.Errorf("pressure.reasons = %v, want nil/empty", body.Pressure.Reasons)
	}
}

func TestHandleMetricsHistory_Empty(t *testing.T) {
	// No prior calls to /api/v1/metrics → history is empty.
	a := api.New(nil, nil, nil, zap.NewNop())
	ts := makeMetricsServer(t, a)

	resp := doGet(t, ts.URL+"/api/v1/metrics/history?duration=1h")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Duration string            `json:"duration"`
		Entries  []json.RawMessage `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Duration == "" {
		t.Error("duration is empty")
	}
	if len(body.Entries) != 0 {
		t.Errorf("entries = %d, want 0", len(body.Entries))
	}
}

func TestHandleMetricsHistory_PopulatedByMetricsCall(t *testing.T) {
	// Each GET /api/v1/metrics appends to the ring buffer.
	pm := metrics.NewPoolMetrics(2)
	a := api.New(nil, nil, nil, zap.NewNop())
	a.SetMetrics(pm, nil, nil)
	ts := makeMetricsServer(t, a)

	// Trigger two snapshots.
	for range 2 {
		resp := doGet(t, ts.URL+"/api/v1/metrics")
		_ = resp.Body.Close()
	}

	resp := doGet(t, ts.URL+"/api/v1/metrics/history?duration=1h")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Entries []struct {
			Timestamp string          `json:"timestamp"`
			Pressure  json.RawMessage `json:"pressure"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Entries) != 2 {
		t.Errorf("entries = %d, want 2", len(body.Entries))
	}
	for i, e := range body.Entries {
		if e.Timestamp == "" {
			t.Errorf("entries[%d].timestamp is empty", i)
		}
	}
}

func TestHandleMetricsHistory_DefaultDuration(t *testing.T) {
	// No duration param → defaults to 1h (non-empty duration in response).
	a := api.New(nil, nil, nil, zap.NewNop())
	ts := makeMetricsServer(t, a)

	resp := doGet(t, ts.URL+"/api/v1/metrics/history")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Duration string `json:"duration"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Duration != "1h0m0s" {
		t.Errorf("duration = %q, want 1h0m0s", body.Duration)
	}
}
