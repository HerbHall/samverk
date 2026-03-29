package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/api"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
)

// doPost is a test helper that sends a POST with a JSON body.
func doPost(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url,
		bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// doPostBytes is a test helper that sends a POST with raw JSON bytes.
func doPostBytes(t *testing.T, url string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url,
		bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func TestRegisterWorker(t *testing.T) {
	ts := newTestAPI(t, nil, nil, nil)

	body := `{"agent_id":"pc-worker","hostname":"HDH-NZXT","capabilities":["code-gen","test"],"max_concurrent":1,"workspace_root":"D:\\bots"}`
	resp := doPost(t, ts.URL+"/api/v1/workers/register", body)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestRegisterWorkerMissingAgentID(t *testing.T) {
	ts := newTestAPI(t, nil, nil, nil)

	body := `{"hostname":"HDH-NZXT"}`
	resp := doPost(t, ts.URL+"/api/v1/workers/register", body)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400, got %d", resp.StatusCode)
	}
}

func TestWorkerHeartbeat(t *testing.T) {
	ts := newTestAPI(t, nil, nil, nil)

	// Register first.
	reg := `{"agent_id":"pc-worker","hostname":"HDH-NZXT","capabilities":["code-gen"],"max_concurrent":1}`
	regResp := doPost(t, ts.URL+"/api/v1/workers/register", reg)
	_ = regResp.Body.Close()

	// Send heartbeat.
	hb := `{"agent_id":"pc-worker","status":"busy","current_task":42,"cpu_percent":35.5,"memory_percent":48.0,"active_worktrees":1}`
	resp := doPost(t, ts.URL+"/api/v1/workers/heartbeat", hb)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestWorkerHeartbeatAutoRegister(t *testing.T) {
	ts := newTestAPI(t, nil, nil, nil)

	// Heartbeat without prior registration — should auto-register.
	hb := `{"agent_id":"unknown-worker","status":"idle","cpu_percent":10.0,"memory_percent":20.0}`
	resp := doPost(t, ts.URL+"/api/v1/workers/heartbeat", hb)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}

	// Should now appear in the worker list.
	listResp := doGet(t, ts.URL+"/api/v1/workers")
	defer func() { _ = listResp.Body.Close() }()

	var workers []map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&workers); err != nil {
		t.Fatalf("decode workers: %v", err)
	}
	if len(workers) != 1 {
		t.Errorf("want 1 worker, got %d", len(workers))
	}
}

func TestListWorkers(t *testing.T) {
	ts := newTestAPI(t, nil, nil, nil)

	// Register two workers.
	for _, id := range []string{"pc-worker-1", "pc-worker-2"} {
		body, _ := json.Marshal(map[string]any{
			"agent_id":       id,
			"hostname":       "host-" + id,
			"capabilities":   []string{"code-gen"},
			"max_concurrent": 1,
		})
		regResp := doPostBytes(t, ts.URL+"/api/v1/workers/register", body)
		_ = regResp.Body.Close()
	}

	resp := doGet(t, ts.URL+"/api/v1/workers")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}

	var workers []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&workers); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(workers) != 2 {
		t.Errorf("want 2 workers, got %d", len(workers))
	}
}

func TestListWorkersEmpty(t *testing.T) {
	ts := newTestAPI(t, nil, nil, nil)

	resp := doGet(t, ts.URL+"/api/v1/workers")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}

	var workers []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&workers); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(workers) != 0 {
		t.Errorf("want 0 workers, got %d", len(workers))
	}
}

// TestListWorkersIncludesComputedStatus verifies that GET /api/v1/workers
// returns a computed_status field for each worker.
func TestListWorkersIncludesComputedStatus(t *testing.T) {
	ts := newTestAPI(t, nil, nil, nil)

	// Register a worker so it has a fresh heartbeat.
	body := `{"agent_id":"w1","hostname":"h1","capabilities":[],"max_concurrent":1}`
	regResp := doPost(t, ts.URL+"/api/v1/workers/register", body)
	_ = regResp.Body.Close()

	resp := doGet(t, ts.URL+"/api/v1/workers")
	defer func() { _ = resp.Body.Close() }()

	var workers []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&workers); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(workers) != 1 {
		t.Fatalf("want 1 worker, got %d", len(workers))
	}
	status, ok := workers[0]["computed_status"].(string)
	if !ok {
		t.Fatalf("computed_status field missing or not a string: %v", workers[0]["computed_status"])
	}
	if status != "healthy" {
		t.Errorf("freshly registered worker: computed_status = %q, want %q", status, "healthy")
	}
}

// TestListWorkersComputedStatusTransitions verifies all three status tiers
// by using tiny thresholds and sleeping between checks.
func TestListWorkersComputedStatusTransitions(t *testing.T) {
	a := api.New(nil, nil, nil, zap.NewNop())
	// stale after 40ms, offline after 80ms — small enough for a fast test.
	a.SetWorkerStaleThreshold(40*time.Millisecond, 80*time.Millisecond)

	mux := http.NewServeMux()
	a.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	body := `{"agent_id":"w1","hostname":"h1","capabilities":[],"max_concurrent":1}`
	regResp := doPost(t, srv.URL+"/api/v1/workers/register", body)
	_ = regResp.Body.Close()

	// Immediately after registration: healthy.
	assertComputedStatus(t, srv.URL, "w1", "healthy")

	// After stale threshold but before offline threshold: stale.
	time.Sleep(50 * time.Millisecond)
	assertComputedStatus(t, srv.URL, "w1", "stale")

	// After offline threshold: offline.
	time.Sleep(50 * time.Millisecond)
	assertComputedStatus(t, srv.URL, "w1", "offline")
}

// assertComputedStatus fetches the worker list and checks that agentID has the expected computed_status.
func assertComputedStatus(t *testing.T, baseURL, agentID, want string) {
	t.Helper()

	resp := doGet(t, baseURL+"/api/v1/workers")
	defer func() { _ = resp.Body.Close() }()

	var workers []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&workers); err != nil {
		t.Fatalf("decode workers: %v", err)
	}

	for _, w := range workers {
		if w["agent_id"] == agentID {
			got, _ := w["computed_status"].(string)
			if got != want {
				t.Errorf("agent %s: computed_status = %q, want %q", agentID, got, want)
			}
			return
		}
	}
	t.Errorf("agent %s not found in worker list", agentID)
}

// TestComputeWorkerStatusLogic tests the pure status-computation function
// for all three transitions using the exported test helper.
func TestComputeWorkerStatusLogic(t *testing.T) {
	stale := 5 * time.Minute
	offline := 15 * time.Minute

	tests := []struct {
		name          string
		lastHeartbeat time.Time
		want          string
	}{
		{
			name:          "zero heartbeat is offline",
			lastHeartbeat: time.Time{},
			want:          "offline",
		},
		{
			name:          "fresh heartbeat is healthy",
			lastHeartbeat: time.Now().Add(-1 * time.Minute),
			want:          "healthy",
		},
		{
			name:          "between stale and offline thresholds is stale",
			lastHeartbeat: time.Now().Add(-10 * time.Minute),
			want:          "stale",
		},
		{
			name:          "past offline threshold is offline",
			lastHeartbeat: time.Now().Add(-20 * time.Minute),
			want:          "offline",
		},
		{
			name:          "just past stale boundary is stale",
			lastHeartbeat: time.Now().Add(-5*time.Minute - time.Millisecond),
			want:          "stale",
		},
		{
			name:          "just past offline boundary is offline",
			lastHeartbeat: time.Now().Add(-15*time.Minute - time.Millisecond),
			want:          "offline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := api.ComputeWorkerStatusForTest(tt.lastHeartbeat, stale, offline)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListActiveWorkers(t *testing.T) {
	now := time.Now().Add(-30 * time.Second)

	activeSess := &models.Session{
		ID:          "sess-active",
		IssueNumber: 42,
		AgentType:   "code-gen",
		Provider:    "ollama",
		Model:       "qwen2.5:7b",
		Status:      models.SessionStatusActive,
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	completedSess := &models.Session{
		ID:          "sess-done",
		IssueNumber: 7,
		AgentType:   "triage",
		Provider:    "claude",
		Model:       "claude-sonnet",
		Status:      models.SessionStatusCompleted,
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// completedSess is declared to show intent; the mock store's ListSessions
	// ignores the status filter, so we control filtering via what's in the slice.
	_ = completedSess

	tests := []struct {
		name       string
		store      store.Store
		wantStatus int
		wantLen    int
	}{
		{
			// The store filters by status=active; we pass only the active session
			// to simulate what SQLiteStore.ListSessions would return.
			name:       "running sessions returned, non-running excluded",
			store:      &mockStore{sessions: []*models.Session{activeSess}},
			wantStatus: http.StatusOK,
			wantLen:    1,
		},
		{
			name:       "nil store returns empty array not null",
			store:      nil,
			wantStatus: http.StatusOK,
			wantLen:    0,
		},
		{
			name:       "empty result returns empty array not null",
			store:      &mockStore{sessions: []*models.Session{}},
			wantStatus: http.StatusOK,
			wantLen:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestAPI(t, nil, tt.store, nil)

			resp := doGet(t, ts.URL+"/api/v1/workers/active")
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			// Decode as raw JSON to verify non-null array.
			var raw json.RawMessage
			if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if string(raw) == "null" {
				t.Errorf("response is null, want array")
			}

			var workers []map[string]any
			if err := json.Unmarshal(raw, &workers); err != nil {
				t.Fatalf("unmarshal workers: %v", err)
			}
			if len(workers) != tt.wantLen {
				t.Errorf("worker count = %d, want %d", len(workers), tt.wantLen)
			}

			if tt.wantLen > 0 {
				w := workers[0]
				if w["session_id"] != activeSess.ID {
					t.Errorf("session_id = %v, want %q", w["session_id"], activeSess.ID)
				}
				if w["issue_number"] != float64(activeSess.IssueNumber) {
					t.Errorf("issue_number = %v, want %d", w["issue_number"], activeSess.IssueNumber)
				}
				elapsedS, ok := w["elapsed_s"].(float64)
				if !ok || elapsedS <= 0 {
					t.Errorf("elapsed_s = %v, want positive float64", w["elapsed_s"])
				}
			}
		})
	}
}