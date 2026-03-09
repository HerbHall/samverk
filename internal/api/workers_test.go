package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
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
