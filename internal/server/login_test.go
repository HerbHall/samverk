package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/herbhall/samverk/internal/server"
)

func newAuthTestServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	s := server.New(server.Config{
		Addr:      "localhost:0",
		AuthToken: token,
	}, nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// noRedirectClient returns an HTTP client that does not follow redirects.
func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func TestLoginPage_Renders(t *testing.T) {
	ts := newAuthTestServer(t, "secret-123")

	resp := get(t, ts.URL+"/login")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	if !strings.Contains(html, "Samverk") {
		t.Error("login page should contain title")
	}
	if !strings.Contains(html, `action="/login"`) {
		t.Error("login page should contain form action")
	}
}

func TestLoginSubmit_CorrectPassword(t *testing.T) {
	const token = "correct-password-123"
	ts := newAuthTestServer(t, token)
	client := noRedirectClient()

	resp, err := client.PostForm(ts.URL+"/login", url.Values{
		"password": {token},
	})
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Should redirect to /
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
	loc := resp.Header.Get("Location")
	if loc != "/" {
		t.Errorf("Location = %q, want %q", loc, "/")
	}

	// Should have a session cookie.
	cookies := resp.Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "samverk_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no samverk_session cookie set after login")
	}
	if sessionCookie.Value == "" {
		t.Error("session cookie value is empty")
	}
}

func TestLoginSubmit_WrongPassword(t *testing.T) {
	ts := newAuthTestServer(t, "correct-password-123")
	client := noRedirectClient()

	resp, err := client.PostForm(ts.URL+"/login", url.Values{
		"password": {"wrong-password"},
	})
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Invalid auth token") {
		t.Error("should show error message for wrong password")
	}

	// Should NOT set a session cookie.
	for _, c := range resp.Cookies() {
		if c.Name == "samverk_session" && c.Value != "" {
			t.Error("should not set session cookie on failed login")
		}
	}
}

func TestSPARequiresSession(t *testing.T) {
	ts := newAuthTestServer(t, "secret-123")
	client := noRedirectClient()

	// Unauthenticated request to SPA should redirect to /login.
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/", http.NoBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

func TestSPAAccessibleWithSession(t *testing.T) {
	const token = "test-token-456"
	ts := newAuthTestServer(t, token)
	client := noRedirectClient()

	// Login first.
	loginResp, err := client.PostForm(ts.URL+"/login", url.Values{
		"password": {token},
	})
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	_ = loginResp.Body.Close()

	// Extract session cookie.
	var sessionCookie *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == "samverk_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie after login")
	}

	// Access SPA with session cookie.
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/", http.NoBody)
	req.AddCookie(sessionCookie)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET / with session: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<div id=\"root\">") {
		t.Error("SPA should be served with valid session")
	}
}

func TestAPIAcceptsSessionCookie(t *testing.T) {
	const token = "api-test-token"

	s := server.New(server.Config{
		Addr:       "localhost:0",
		AuthToken:  token,
		APIHandler: stubAPI{},
	}, nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	client := noRedirectClient()

	// Login first.
	loginResp, err := client.PostForm(ts.URL+"/login", url.Values{
		"password": {token},
	})
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	_ = loginResp.Body.Close()

	var sessionCookie *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == "samverk_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie after login")
	}

	// API request with session cookie (no Bearer token).
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/api/v1/ping", http.NoBody)
	req.AddCookie(sessionCookie)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/ping: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d (session cookie should grant API access)", resp.StatusCode, http.StatusOK)
	}
}

func TestAPIStillAcceptsBearerToken(t *testing.T) {
	const token = "bearer-test-token"

	s := server.New(server.Config{
		Addr:       "localhost:0",
		AuthToken:  token,
		APIHandler: stubAPI{},
	}, nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	// API request with Bearer token (no session cookie).
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/api/v1/ping", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/ping: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d (bearer token should still work)", resp.StatusCode, http.StatusOK)
	}
}

func TestMCPRequiresBearerToken(t *testing.T) {
	const token = "mcp-auth-token"
	mcpHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	s := server.New(server.Config{
		Addr:       "localhost:0",
		AuthToken:  token,
		MCPHandler: mcpHandler,
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
			authHeader: "Bearer wrong",
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
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/mcp", http.NoBody)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("POST /mcp: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}

func TestLogout(t *testing.T) {
	const token = "logout-test-token"
	ts := newAuthTestServer(t, token)
	client := noRedirectClient()

	// Login first.
	loginResp, err := client.PostForm(ts.URL+"/login", url.Values{
		"password": {token},
	})
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	_ = loginResp.Body.Close()

	var sessionCookie *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == "samverk_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie after login")
	}

	// Logout.
	logoutReq, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/logout", http.NoBody)
	logoutReq.AddCookie(sessionCookie)
	logoutResp, err := client.Do(logoutReq)
	if err != nil {
		t.Fatalf("POST /logout: %v", err)
	}
	_ = logoutResp.Body.Close()

	if logoutResp.StatusCode != http.StatusSeeOther {
		t.Errorf("logout status = %d, want %d", logoutResp.StatusCode, http.StatusSeeOther)
	}

	// SPA should no longer be accessible with old session cookie.
	spaReq, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/", http.NoBody)
	spaReq.AddCookie(sessionCookie)
	spaResp, err := client.Do(spaReq)
	if err != nil {
		t.Fatalf("GET / after logout: %v", err)
	}
	_ = spaResp.Body.Close()

	if spaResp.StatusCode != http.StatusSeeOther {
		t.Errorf("status after logout = %d, want %d (redirect to login)", spaResp.StatusCode, http.StatusSeeOther)
	}
}
