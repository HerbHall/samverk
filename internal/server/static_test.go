package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/herbhall/samverk/internal/server"
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

func TestSPAVersionInjected(t *testing.T) {
	// Version should always be injected (no auth token in HTML).
	ts := newTestServer(t)

	resp := get(t, ts.URL+"/")
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	html := string(body)
	if !strings.Contains(html, "__SAMVERK_VERSION__") {
		t.Errorf("version should always be injected:\n%s", html)
	}
}

func TestSPANoTokenInHTML(t *testing.T) {
	// Even with AuthToken configured, the token must NOT appear in HTML.
	const token = "test-dashboard-token"

	s := server.New(server.Config{
		Addr:      "localhost:0",
		AuthToken: token,
	}, nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	// With auth enabled, unauthenticated requests redirect to /login.
	// Use a non-redirect client to check the redirect.
	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/", http.NoBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Should redirect to /login (not serve SPA with token).
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d (redirect to login)", resp.StatusCode, http.StatusSeeOther)
	}

	// Fetch the login page and verify no token is present.
	loginResp := get(t, ts.URL+"/login")
	defer func() { _ = loginResp.Body.Close() }()

	loginBody, _ := io.ReadAll(loginResp.Body)
	if strings.Contains(string(loginBody), token) {
		t.Errorf("auth token should not appear in login page HTML")
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
