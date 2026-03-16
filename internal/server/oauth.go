package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// oauthDiscovery is the OAuth 2.1 authorization server metadata document.
// See RFC 8414 and the OAuth 2.1 draft specification.
type oauthDiscovery struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	RevocationEndpoint    string   `json:"revocation_endpoint,omitempty"`
	ResponseTypes         []string `json:"response_types_supported"`
	GrantTypes            []string `json:"grant_types_supported"`
	CodeChallenge         []string `json:"code_challenge_methods_supported"`
}

// oauthPending tracks a pending authorization code exchange.
type oauthPending struct {
	redirectURI   string
	codeChallenge string
	expiresAt     time.Time
}

// oauthCodeStore manages short-lived authorization codes.
// Codes expire after 5 minutes per OAuth 2.1 spec.
type oauthCodeStore struct {
	codes sync.Map // map[string]*oauthPending
}

func (s *oauthCodeStore) store(code string, pending *oauthPending) {
	s.codes.Store(code, pending)
}

func (s *oauthCodeStore) loadAndDelete(code string) (*oauthPending, bool) {
	val, ok := s.codes.LoadAndDelete(code)
	if !ok {
		return nil, false
	}
	return val.(*oauthPending), true
}

// oauthTokenResponse is returned by the token endpoint.
type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// oauthErrorResponse is returned for OAuth errors.
type oauthErrorResponse struct {
	Error string `json:"error"`
}

// handleOAuthDiscovery serves the OAuth 2.1 authorization server metadata.
// GET /.well-known/oauth-authorization-server
func (s *Server) handleOAuthDiscovery(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if r.TLS == nil {
		// Check X-Forwarded-Proto for reverse proxy setups (Caddy, Cloudflare).
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else {
			scheme = "http"
		}
	}
	base := scheme + "://" + r.Host

	doc := oauthDiscovery{
		Issuer:                base,
		AuthorizationEndpoint: base + "/oauth/authorize",
		TokenEndpoint:         base + "/oauth/token",
		RevocationEndpoint:    base + "/oauth/revoke",
		ResponseTypes:         []string{"code"},
		GrantTypes:            []string{"authorization_code"},
		CodeChallenge:         []string{"S256"},
	}
	writeJSON(w, http.StatusOK, doc)
}

// handleOAuthAuthorize handles the authorization request.
// GET /oauth/authorize?client_id=...&redirect_uri=...&state=...&code_challenge=...&code_challenge_method=S256&response_type=code
//
// This auto-approves the request and redirects back with an authorization code.
// The code must be exchanged at the token endpoint within 5 minutes.
func (s *Server) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	redirectURI := r.URL.Query().Get("redirect_uri")
	state := r.URL.Query().Get("state")

	if redirectURI == "" {
		writeJSON(w, http.StatusBadRequest, oauthErrorResponse{Error: "invalid_request"})
		return
	}

	code := generateOAuthCode()

	s.oauthCodes.store(code, &oauthPending{
		redirectURI:   redirectURI,
		codeChallenge: r.URL.Query().Get("code_challenge"),
		expiresAt:     time.Now().Add(5 * time.Minute),
	})

	u, err := url.Parse(redirectURI)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, oauthErrorResponse{Error: "invalid_request"})
		return
	}
	q := u.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// handleOAuthToken exchanges an authorization code for an access token.
// POST /oauth/token
// Content-Type: application/x-www-form-urlencoded
// grant_type=authorization_code&code=...&redirect_uri=...&code_verifier=...
//
// Returns the server's configured auth token as the access_token,
// bridging OAuth to the existing Bearer token auth system.
func (s *Server) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, oauthErrorResponse{Error: "invalid_request"})
		return
	}

	grantType := r.FormValue("grant_type")
	if grantType != "authorization_code" {
		writeJSON(w, http.StatusBadRequest, oauthErrorResponse{Error: "unsupported_grant_type"})
		return
	}

	code := r.FormValue("code")
	if code == "" {
		writeJSON(w, http.StatusBadRequest, oauthErrorResponse{Error: "invalid_request"})
		return
	}

	pending, ok := s.oauthCodes.loadAndDelete(code)
	if !ok {
		writeJSON(w, http.StatusBadRequest, oauthErrorResponse{Error: "invalid_grant"})
		return
	}

	if time.Now().After(pending.expiresAt) {
		writeJSON(w, http.StatusBadRequest, oauthErrorResponse{Error: "invalid_grant"})
		return
	}

	// Verify PKCE S256 challenge if one was provided during authorization.
	codeVerifier := r.FormValue("code_verifier")
	if pending.codeChallenge != "" {
		if codeVerifier == "" {
			writeJSON(w, http.StatusBadRequest, oauthErrorResponse{Error: "invalid_grant"})
			return
		}
		h := sha256.Sum256([]byte(codeVerifier))
		challenge := base64.RawURLEncoding.EncodeToString(h[:])
		if challenge != pending.codeChallenge {
			writeJSON(w, http.StatusBadRequest, oauthErrorResponse{Error: "invalid_grant"})
			return
		}
	}

	// Bridge: return the server's configured auth token as the OAuth access token.
	token := s.cfg.AuthToken
	if token == "" {
		writeJSON(w, http.StatusBadRequest, oauthErrorResponse{Error: "server_error"})
		return
	}

	writeJSON(w, http.StatusOK, oauthTokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   86400,
	})
}

// handleOAuthRevoke handles token revocation requests.
// POST /oauth/revoke
// This is a no-op because tokens are server-configured, not revocable.
// Returns 200 per RFC 7009 (revocation always succeeds from client perspective).
func (s *Server) handleOAuthRevoke(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// generateOAuthCode produces a cryptographically random 32-byte hex string
// for use as an OAuth authorization code.
func generateOAuthCode() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback is intentionally weak -- crypto/rand failure is exceptional.
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}
