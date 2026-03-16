package server_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/server"
)

func newOAuthTestServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	s := server.New(server.Config{
		Addr:      "localhost:0",
		AuthToken: token,
	}, zap.NewNop())
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestOAuthDiscovery(t *testing.T) {
	ts := newOAuthTestServer(t, "test-token")

	resp := get(t, ts.URL+"/.well-known/oauth-authorization-server")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var doc struct {
		Issuer                string   `json:"issuer"`
		AuthorizationEndpoint string   `json:"authorization_endpoint"`
		TokenEndpoint         string   `json:"token_endpoint"`
		RevocationEndpoint    string   `json:"revocation_endpoint"`
		ResponseTypes         []string `json:"response_types_supported"`
		GrantTypes            []string `json:"grant_types_supported"`
		CodeChallenge         []string `json:"code_challenge_methods_supported"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if doc.Issuer == "" {
		t.Error("issuer is empty")
	}
	if doc.AuthorizationEndpoint == "" {
		t.Error("authorization_endpoint is empty")
	}
	if doc.TokenEndpoint == "" {
		t.Error("token_endpoint is empty")
	}
	if doc.RevocationEndpoint == "" {
		t.Error("revocation_endpoint is empty")
	}
	if len(doc.ResponseTypes) == 0 || doc.ResponseTypes[0] != "code" {
		t.Errorf("response_types = %v, want [code]", doc.ResponseTypes)
	}
	if len(doc.GrantTypes) == 0 || doc.GrantTypes[0] != "authorization_code" {
		t.Errorf("grant_types = %v, want [authorization_code]", doc.GrantTypes)
	}
	if len(doc.CodeChallenge) == 0 || doc.CodeChallenge[0] != "S256" {
		t.Errorf("code_challenge_methods = %v, want [S256]", doc.CodeChallenge)
	}

	// Endpoints should use the test server's base URL.
	if !strings.HasPrefix(doc.AuthorizationEndpoint, "http://") {
		t.Errorf("authorization_endpoint = %q, want http:// prefix", doc.AuthorizationEndpoint)
	}
}

func TestOAuthFlow(t *testing.T) {
	const token = "my-secret-token"
	ts := newOAuthTestServer(t, token)

	// Use a no-redirect client so we can inspect the 302 Location header.
	noRedirect := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Step 1: Authorization request.
	redirectURI := "https://example.com/callback"
	state := "random-state-value"
	authURL := ts.URL + "/oauth/authorize?" + url.Values{
		"client_id":    {"claude-mobile"},
		"redirect_uri": {redirectURI},
		"state":        {state},
		"response_type": {"code"},
	}.Encode()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, authURL, http.NoBody)
	if err != nil {
		t.Fatalf("new auth request: %v", err)
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("authorize status = %d, want %d", resp.StatusCode, http.StatusFound)
	}

	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}

	code := loc.Query().Get("code")
	if code == "" {
		t.Fatal("no code in redirect")
	}
	if loc.Query().Get("state") != state {
		t.Errorf("state = %q, want %q", loc.Query().Get("state"), state)
	}

	// Step 2: Exchange code for token.
	tokenReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/oauth/token",
		strings.NewReader(url.Values{
			"grant_type":   {"authorization_code"},
			"code":         {code},
			"redirect_uri": {redirectURI},
		}.Encode()))
	if err != nil {
		t.Fatalf("new token request: %v", err)
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	tokenResp, err := http.DefaultClient.Do(tokenReq)
	if err != nil {
		t.Fatalf("token exchange: %v", err)
	}
	defer func() { _ = tokenResp.Body.Close() }()

	if tokenResp.StatusCode != http.StatusOK {
		t.Fatalf("token status = %d, want %d", tokenResp.StatusCode, http.StatusOK)
	}

	var tokenBody struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenBody); err != nil {
		t.Fatalf("decode token: %v", err)
	}

	if tokenBody.AccessToken != token {
		t.Errorf("access_token = %q, want %q", tokenBody.AccessToken, token)
	}
	if tokenBody.TokenType != "Bearer" {
		t.Errorf("token_type = %q, want Bearer", tokenBody.TokenType)
	}
	if tokenBody.ExpiresIn != 86400 {
		t.Errorf("expires_in = %d, want 86400", tokenBody.ExpiresIn)
	}
}

func TestOAuthPKCE(t *testing.T) {
	const token = "pkce-test-token"
	ts := newOAuthTestServer(t, token)

	noRedirect := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Generate PKCE pair.
	codeVerifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	h := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(h[:])

	// Step 1: Authorize with code_challenge.
	authURL := ts.URL + "/oauth/authorize?" + url.Values{
		"client_id":             {"claude-mobile"},
		"redirect_uri":         {"https://example.com/callback"},
		"state":                {"pkce-state"},
		"response_type":        {"code"},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, authURL, http.NoBody)
	if err != nil {
		t.Fatalf("new auth request: %v", err)
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	code := loc.Query().Get("code")

	// Step 2: Exchange with correct code_verifier -- should succeed.
	tokenResp := exchangeToken(t, ts.URL, code, "https://example.com/callback", codeVerifier)
	defer func() { _ = tokenResp.Body.Close() }()

	if tokenResp.StatusCode != http.StatusOK {
		t.Errorf("token status = %d, want %d", tokenResp.StatusCode, http.StatusOK)
	}
}

func TestOAuthPKCEWrongVerifier(t *testing.T) {
	const token = "pkce-wrong-test"
	ts := newOAuthTestServer(t, token)

	noRedirect := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	codeVerifier := "correct-verifier-value"
	h := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(h[:])

	authURL := ts.URL + "/oauth/authorize?" + url.Values{
		"client_id":             {"claude-mobile"},
		"redirect_uri":         {"https://example.com/callback"},
		"state":                {"s"},
		"response_type":        {"code"},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, authURL, http.NoBody)
	if err != nil {
		t.Fatalf("new auth request: %v", err)
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	code := loc.Query().Get("code")

	// Exchange with WRONG code_verifier -- should fail.
	tokenResp := exchangeToken(t, ts.URL, code, "https://example.com/callback", "wrong-verifier")
	defer func() { _ = tokenResp.Body.Close() }()

	if tokenResp.StatusCode != http.StatusBadRequest {
		t.Errorf("token status = %d, want %d", tokenResp.StatusCode, http.StatusBadRequest)
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "invalid_grant" {
		t.Errorf("error = %q, want invalid_grant", body.Error)
	}
}

func TestOAuthCodeReuse(t *testing.T) {
	const token = "reuse-test"
	ts := newOAuthTestServer(t, token)

	noRedirect := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	authURL := ts.URL + "/oauth/authorize?" + url.Values{
		"client_id":    {"claude-mobile"},
		"redirect_uri": {"https://example.com/callback"},
		"state":        {"s"},
		"response_type": {"code"},
	}.Encode()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, authURL, http.NoBody)
	if err != nil {
		t.Fatalf("new auth request: %v", err)
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	code := loc.Query().Get("code")

	// First exchange succeeds.
	tokenResp := exchangeToken(t, ts.URL, code, "https://example.com/callback", "")
	defer func() { _ = tokenResp.Body.Close() }()
	if tokenResp.StatusCode != http.StatusOK {
		t.Fatalf("first exchange status = %d, want %d", tokenResp.StatusCode, http.StatusOK)
	}

	// Second exchange with same code fails (code is single-use).
	tokenResp2 := exchangeToken(t, ts.URL, code, "https://example.com/callback", "")
	defer func() { _ = tokenResp2.Body.Close() }()
	if tokenResp2.StatusCode != http.StatusBadRequest {
		t.Errorf("second exchange status = %d, want %d", tokenResp2.StatusCode, http.StatusBadRequest)
	}
}

func TestOAuthInvalidCode(t *testing.T) {
	ts := newOAuthTestServer(t, "test-token")

	tokenResp := exchangeToken(t, ts.URL, "nonexistent-code", "https://example.com/callback", "")
	defer func() { _ = tokenResp.Body.Close() }()

	if tokenResp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", tokenResp.StatusCode, http.StatusBadRequest)
	}
}

func TestOAuthMissingRedirectURI(t *testing.T) {
	ts := newOAuthTestServer(t, "test-token")

	resp := get(t, ts.URL+"/oauth/authorize?client_id=test&state=s")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestOAuthRevoke(t *testing.T) {
	ts := newOAuthTestServer(t, "test-token")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/oauth/revoke",
		strings.NewReader("token=some-token"))
	if err != nil {
		t.Fatalf("new revoke request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestOAuthUnsupportedGrantType(t *testing.T) {
	ts := newOAuthTestServer(t, "test-token")

	tokenReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/oauth/token",
		strings.NewReader(url.Values{
			"grant_type": {"client_credentials"},
		}.Encode()))
	if err != nil {
		t.Fatalf("new token request: %v", err)
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(tokenReq)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "unsupported_grant_type" {
		t.Errorf("error = %q, want unsupported_grant_type", body.Error)
	}
}

func TestOAuthNoAuthToken(t *testing.T) {
	// Server with no auth token configured -- token exchange should fail.
	ts := newOAuthTestServer(t, "")

	noRedirect := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	authURL := ts.URL + "/oauth/authorize?" + url.Values{
		"client_id":    {"claude-mobile"},
		"redirect_uri": {"https://example.com/callback"},
		"state":        {"s"},
		"response_type": {"code"},
	}.Encode()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, authURL, http.NoBody)
	if err != nil {
		t.Fatalf("new auth request: %v", err)
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	code := loc.Query().Get("code")

	tokenResp := exchangeToken(t, ts.URL, code, "https://example.com/callback", "")
	defer func() { _ = tokenResp.Body.Close() }()

	if tokenResp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (no auth token configured)", tokenResp.StatusCode, http.StatusBadRequest)
	}
}

// exchangeToken performs a POST /oauth/token request.
func exchangeToken(t *testing.T, baseURL, code, redirectURI, codeVerifier string) *http.Response {
	t.Helper()

	values := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}
	if codeVerifier != "" {
		values.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/oauth/token",
		strings.NewReader(values.Encode()))
	if err != nil {
		t.Fatalf("new token request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("token exchange: %v", err)
	}
	return resp
}
