package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
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
	s := server.New(server.Config{Addr: "localhost:0"}, zap.NewNop())
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

// stubAPI is a minimal APIRegistrar that registers a single test endpoint.
type stubAPI struct{}

func (stubAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"pong":true}`))
	})
}

func TestAPIRoutesRequireAuth(t *testing.T) {
	const token = "test-api-token"

	s := server.New(server.Config{
		Addr:       "localhost:0",
		AuthToken:  token,
		APIHandler: stubAPI{},
	}, nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{
			name:       "no auth returns 401",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong token returns 401",
			authHeader: "Bearer wrong-token",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "valid token returns 200",
			authHeader: "Bearer " + token,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/api/v1/ping", http.NoBody)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}

func TestAPIRoutesNoAuthWhenTokenEmpty(t *testing.T) {
	s := server.New(server.Config{
		Addr:       "localhost:0",
		AuthToken:  "", // no token = no auth
		APIHandler: stubAPI{},
	}, nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	resp := get(t, ts.URL+"/api/v1/ping")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d (no auth should pass through)", resp.StatusCode, http.StatusOK)
	}
}

func TestGracefulShutdown(t *testing.T) {
	s := server.New(server.Config{Addr: "localhost:0"}, zap.NewNop())

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
