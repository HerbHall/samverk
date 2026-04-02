package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"samverk.dev/samverk/internal/api"
)

func TestDrainMux_PostEntersDrainMode(t *testing.T) {
	var drainCalled bool
	mux := api.DrainMux(
		func() api.DrainResponse {
			drainCalled = true
			return api.DrainResponse{
				Draining:      true,
				ActiveWorkers: 2,
				ClaimedIssues: []int{42, 99},
				QueueDepth:    0,
			}
		},
		func() api.DrainResponse {
			return api.DrainResponse{Draining: true}
		},
		func() api.DrainResponse {
			return api.DrainResponse{Draining: false}
		},
	)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/v1/dispatcher/drain", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST drain: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if !drainCalled {
		t.Error("drain function was not called")
	}

	var dr api.DrainResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !dr.Draining {
		t.Error("expected draining=true")
	}
	if dr.ActiveWorkers != 2 {
		t.Errorf("expected 2 active workers, got %d", dr.ActiveWorkers)
	}
	if len(dr.ClaimedIssues) != 2 {
		t.Errorf("expected 2 claimed issues, got %d", len(dr.ClaimedIssues))
	}
}

func TestDrainMux_GetReturnsStatus(t *testing.T) {
	mux := api.DrainMux(
		func() api.DrainResponse { return api.DrainResponse{} },
		func() api.DrainResponse {
			return api.DrainResponse{
				Draining:      true,
				ActiveWorkers: 1,
				ClaimedIssues: []int{42},
				QueueDepth:    3,
			}
		},
		func() api.DrainResponse { return api.DrainResponse{} },
	)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/v1/dispatcher/drain", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET drain: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var dr api.DrainResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !dr.Draining {
		t.Error("expected draining=true")
	}
	if dr.ActiveWorkers != 1 {
		t.Errorf("expected 1 active worker, got %d", dr.ActiveWorkers)
	}
	if dr.QueueDepth != 3 {
		t.Errorf("expected queue_depth=3, got %d", dr.QueueDepth)
	}
}

func TestDrainMux_DeleteCancelsDrain(t *testing.T) {
	var cancelCalled bool
	mux := api.DrainMux(
		func() api.DrainResponse { return api.DrainResponse{} },
		func() api.DrainResponse { return api.DrainResponse{} },
		func() api.DrainResponse {
			cancelCalled = true
			return api.DrainResponse{Draining: false, ActiveWorkers: 0, ClaimedIssues: []int{}}
		},
	)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, srv.URL+"/api/v1/dispatcher/drain", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE drain: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !cancelCalled {
		t.Error("cancel function was not called")
	}

	var dr api.DrainResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dr.Draining {
		t.Error("expected draining=false after cancel")
	}
}

func TestDrainMux_HealthCheck(t *testing.T) {
	mux := api.DrainMux(
		func() api.DrainResponse { return api.DrainResponse{} },
		func() api.DrainResponse { return api.DrainResponse{} },
		func() api.DrainResponse { return api.DrainResponse{} },
	)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/healthz", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
