package server

// Integration tests for JWT enforcement on the /mcp route wired in registerRoutes.
// These tests live in the internal package to access the buildTestJWTWithAud helper
// and the cfAccessEnforceWithCache constructor for cache pre-warming.

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// stubMCPHandler records whether it was called.
type stubMCPHandler struct {
	called bool
}

func (h *stubMCPHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.called = true
	w.WriteHeader(http.StatusOK)
}

// newTestJWTKey generates a throwaway RSA key pair and returns the key
// and a pre-warmed jwksCache containing the corresponding public key.
func newTestJWTKey(t *testing.T) (*rsa.PrivateKey, *jwksCache) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	pub, err := parseRSAPublicKey(
		base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		base64.RawURLEncoding.EncodeToString(encodeExponent(key.E)),
	)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}
	cache := &jwksCache{}
	cache.set([]*rsa.PublicKey{pub})
	return key, cache
}

// newServerWithJWT creates a Server configured with CFAccess JWT enforcement and
// a pre-warmed JWKS cache so tests never make real HTTP calls to Cloudflare.
//
// The server's /mcp route will use cfAccessEnforceWithCache (pre-warmed cache)
// rather than the production CFAccessEnforceMiddleware (which builds a fresh cache).
// We do this by wiring the middleware ourselves after constructing the server,
// using the same httptest.Server pattern.
func newServerWithJWT(t *testing.T, clientID string, cache *jwksCache, mcpHandler http.Handler) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	enforce := cfAccessEnforceWithCache("test.cloudflareaccess.com", clientID, cache)
	mux.Handle("/mcp", enforce(mcpHandler))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// TestMCPRoute_JWTEnforcement_ValidJWT verifies that a valid JWT with the correct
// audience reaches the MCP handler (200).
func TestMCPRoute_JWTEnforcement_ValidJWT(t *testing.T) {
	const clientID = "test-mcp-client-id"
	key, cache := newTestJWTKey(t)
	stub := &stubMCPHandler{}

	ts := newServerWithJWT(t, clientID, cache, stub)

	jwtToken := buildTestJWTWithAud(t, key, time.Now().Add(time.Hour).Unix(), clientID)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL+"/mcp", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Cf-Access-Jwt-Assertion", jwtToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if !stub.called {
		t.Error("expected MCP handler to be called")
	}
}

// TestMCPRoute_JWTEnforcement_MissingJWT verifies that a request without the JWT
// header is rejected with 401.
func TestMCPRoute_JWTEnforcement_MissingJWT(t *testing.T) {
	const clientID = "test-mcp-client-id"
	_, cache := newTestJWTKey(t)
	stub := &stubMCPHandler{}

	ts := newServerWithJWT(t, clientID, cache, stub)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL+"/mcp", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	// No Cf-Access-Jwt-Assertion header.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if stub.called {
		t.Error("MCP handler must not be called when JWT is missing")
	}
}

// TestMCPRoute_JWTEnforcement_WrongAud verifies that a valid JWT with the wrong
// audience is rejected with 401.
func TestMCPRoute_JWTEnforcement_WrongAud(t *testing.T) {
	const clientID = "correct-client-id"
	key, cache := newTestJWTKey(t)
	stub := &stubMCPHandler{}

	ts := newServerWithJWT(t, clientID, cache, stub)

	jwtToken := buildTestJWTWithAud(t, key, time.Now().Add(time.Hour).Unix(), "wrong-client-id")

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL+"/mcp", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Cf-Access-Jwt-Assertion", jwtToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if stub.called {
		t.Error("MCP handler must not be called on wrong audience")
	}
}

// TestMCPRoute_BearerAuth_ValidToken verifies that existing Bearer-only auth
// (no JWT configured) still passes with a valid token (200).
func TestMCPRoute_BearerAuth_ValidToken(t *testing.T) {
	const token = "test-bearer-token"
	stub := &stubMCPHandler{}

	s := New(Config{
		Addr:       "localhost:0",
		AuthToken:  token,
		MCPHandler: stub,
	}, nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL+"/mcp", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if !stub.called {
		t.Error("expected MCP handler to be called with valid Bearer token")
	}
}

// TestMCPRoute_CFAccessOrBearer_BearerFallback verifies that when both JWT and
// Bearer are configured, a request carrying only a Bearer token is accepted (200).
// This covers the internal/LAN access path where no CF JWT header is present.
func TestMCPRoute_CFAccessOrBearer_BearerFallback(t *testing.T) {
	const (
		token    = "test-bearer-token"
		clientID = "some-cf-client-id"
	)
	stub := &stubMCPHandler{}

	s := New(Config{
		Addr:                "localhost:0",
		AuthToken:           token,
		CFAccessTeamDomain:  "test.cloudflareaccess.com",
		CFAccessMCPClientID: clientID,
		MCPHandler:          stub,
	}, nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL+"/mcp", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (bearer should be accepted when CF JWT header is absent)", resp.StatusCode)
	}
	if !stub.called {
		t.Error("expected MCP handler to be called with valid Bearer token")
	}
}

// TestMCPRoute_CFAccessOrBearer_NoCredentials verifies that when both JWT and
// Bearer are configured, a request with no credentials is rejected (401).
func TestMCPRoute_CFAccessOrBearer_NoCredentials(t *testing.T) {
	const (
		token    = "test-bearer-token"
		clientID = "some-cf-client-id"
	)
	stub := &stubMCPHandler{}

	s := New(Config{
		Addr:                "localhost:0",
		AuthToken:           token,
		CFAccessTeamDomain:  "test.cloudflareaccess.com",
		CFAccessMCPClientID: clientID,
		MCPHandler:          stub,
	}, nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL+"/mcp", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	// No auth headers.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if stub.called {
		t.Error("MCP handler must not be called without credentials")
	}
}

// TestMCPRoute_BearerAuth_NoToken verifies that Bearer-only auth (no JWT configured)
// rejects requests without a token (401).
func TestMCPRoute_BearerAuth_NoToken(t *testing.T) {
	const token = "test-bearer-token"
	stub := &stubMCPHandler{}

	s := New(Config{
		Addr:       "localhost:0",
		AuthToken:  token,
		MCPHandler: stub,
	}, nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL+"/mcp", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	// No Authorization header.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if stub.called {
		t.Error("MCP handler must not be called without a token")
	}
}
