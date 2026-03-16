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

func TestSPATokenInjection(t *testing.T) {
	const token = "test-dashboard-token"

	s := server.New(server.Config{
		Addr:      "localhost:0",
		AuthToken: token,
	}, nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	tests := []struct {
		name string
		path string
	}{
		{"root", "/"},
		{"index.html", "/index.html"},
		{"client route fallback", "/settings"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := get(t, ts.URL+tt.path)
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			html := string(body)

			// Verify token and version are injected.
			if !strings.Contains(html, `__SAMVERK_TOKEN__="test-dashboard-token"`) {
				t.Errorf("token not found in response:\n%s", html)
			}
			if !strings.Contains(html, `__SAMVERK_VERSION__=`) {
				t.Errorf("version not found in response:\n%s", html)
			}
			// Injection script must appear before </head>.
			tagIdx := strings.Index(html, "__SAMVERK_VERSION__")
			headIdx := strings.Index(html, "</head>")
			if tagIdx > headIdx {
				t.Errorf("token tag at %d appears after </head> at %d", tagIdx, headIdx)
			}
		})
	}
}

func TestSPANoTokenWhenEmpty(t *testing.T) {
	// When AuthToken is empty, no script tag should be injected.
	ts := newTestServer(t) // uses empty Config

	resp := get(t, ts.URL+"/")
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	html := string(body)
	if strings.Contains(html, "__SAMVERK_TOKEN__") {
		t.Errorf("token should not be present when AuthToken is empty:\n%s", html)
	}
	// Version should always be injected, even without auth token.
	if !strings.Contains(html, "__SAMVERK_VERSION__") {
		t.Errorf("version should always be injected:\n%s", html)
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
