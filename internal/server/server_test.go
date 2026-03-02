package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/server"
)

// get performs a context-aware GET request.
func get(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("new GET request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

// post performs a context-aware POST request with an empty JSON body.
func post(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader(""))
	if err != nil {
		t.Fatalf("new POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// newTestServer creates a Server whose handler is wired but not yet listening.
// It returns an httptest.Server backed by the same mux for unit tests.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := server.New(server.Config{Addr: "localhost:0"})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestHealthEndpoint(t *testing.T) {
	ts := newTestServer(t)

	resp := get(t, ts.URL+"/healthz")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestMCPNotImplemented(t *testing.T) {
	ts := newTestServer(t)

	resp := post(t, ts.URL+"/mcp")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotImplemented)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "not implemented" {
		t.Errorf("error = %q, want %q", body["error"], "not implemented")
	}
}

func TestAPINotImplemented(t *testing.T) {
	ts := newTestServer(t)

	resp := get(t, ts.URL+"/api/v1/anything")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotImplemented)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "not implemented" {
		t.Errorf("error = %q, want %q", body["error"], "not implemented")
	}
}

func TestGracefulShutdown(t *testing.T) {
	s := server.New(server.Config{Addr: "localhost:0"})

	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		// Signal readiness once Start has bound the port.
		// We probe until the server responds.
		go func() {
			client := &http.Client{}
			for {
				req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
					fmt.Sprintf("http://%s/healthz", s.Addr()), http.NoBody)
				if err != nil {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				resp, err := client.Do(req)
				if err == nil {
					_ = resp.Body.Close()
					close(started)
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}()
		errCh <- s.Start(ctx)
	}()

	select {
	case <-started:
		// server is up
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start within 5s")
	}

	// Signal shutdown.
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned error after cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within 5s")
	}
}
