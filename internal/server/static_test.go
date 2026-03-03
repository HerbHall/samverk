package server_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSPAServesIndexAtRoot(t *testing.T) {
	ts := newTestServer(t)

	resp := get(t, ts.URL+"/")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "<div id=\"root\">") {
		t.Errorf("body does not contain root div:\n%s", body)
	}
}

func TestSPAFallbackForClientRoutes(t *testing.T) {
	ts := newTestServer(t)

	// Client-side routes like /settings, /issues/123 should all serve index.html.
	paths := []string{"/settings", "/issues/123", "/dashboard/overview"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			resp := get(t, ts.URL+path)
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if !strings.Contains(string(body), "<div id=\"root\">") {
				t.Errorf("body does not contain root div:\n%s", body)
			}
		})
	}
}

func TestSPAServesStaticFiles(t *testing.T) {
	ts := newTestServer(t)

	// The placeholder index.html exists in the embedded filesystem.
	resp := get(t, ts.URL+"/index.html")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "Samverk") {
		t.Errorf("body does not contain title:\n%s", body)
	}
}

func TestAPIRoutesPriority(t *testing.T) {
	ts := newTestServer(t)

	// API routes must still return 501 (not the SPA), proving route priority.
	resp := get(t, ts.URL+"/api/v1/issues")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotImplemented)
	}
}

func TestHealthRouteStillWorks(t *testing.T) {
	ts := newTestServer(t)

	// /healthz must still return JSON health check, not the SPA.
	resp := get(t, ts.URL+"/healthz")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json prefix", ct)
	}
}
