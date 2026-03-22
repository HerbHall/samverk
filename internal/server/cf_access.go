package server

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	cfJWTHeader      = "Cf-Access-Jwt-Assertion"
	jwksCacheTTL     = 24 * time.Hour
	jwksFetchPath    = "/cdn-cgi/access/certs"
	saasJWKSPathFmt  = "/cdn-cgi/access/sso/oidc/%s/jwks"
)

// jwksResponse is the JSON structure returned by the Cloudflare Access JWKS endpoint.
type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

// jwk is a single JSON Web Key (RSA public key fields only).
type jwk struct {
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	KID string `json:"kid"`
}

// jwksCache caches RSA public keys fetched from the Cloudflare Access JWKS endpoint.
type jwksCache struct {
	mu        sync.RWMutex
	keys      []*rsa.PublicKey
	fetchedAt time.Time
}

// get returns cached keys if still valid, or nil if the cache has expired or is empty.
func (c *jwksCache) get() []*rsa.PublicKey {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.keys) == 0 || time.Since(c.fetchedAt) > jwksCacheTTL {
		return nil
	}
	return c.keys
}

// set stores fetched keys and records the fetch time.
func (c *jwksCache) set(keys []*rsa.PublicKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys = keys
	c.fetchedAt = time.Now()
}

// cfSessionStore is the minimal interface needed by CFAccessMiddleware to create sessions.
type cfSessionStore interface {
	Create() (string, error)
	SetCookie(w http.ResponseWriter, sessionID string)
}

// CFAccessMiddleware auto-issues a session when a valid Cloudflare Access JWT is
// present in the Cf-Access-Jwt-Assertion header. Only active when teamDomain is
// non-empty (CF_ACCESS_TEAM_DOMAIN env var).
//
// On valid JWT:  create session → set cookie → redirect to /
// On missing/invalid JWT: fall through to next handler (no error returned)
// On disabled (teamDomain=""): fall through immediately
//
// The middleware only intercepts GET / and GET /login; all other paths pass through.
func CFAccessMiddleware(teamDomain string, sessions cfSessionStore) func(http.Handler) http.Handler {
	if teamDomain == "" {
		return func(next http.Handler) http.Handler { return next }
	}
	return cfAccessMiddlewareWithCache(teamDomain, sessions, &jwksCache{})
}

// cfAccessMiddlewareWithCache builds the middleware with the given JWKS cache.
// Separated from CFAccessMiddleware to allow pre-warming the cache in tests.
func cfAccessMiddlewareWithCache(teamDomain string, sessions cfSessionStore, cache *jwksCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only intercept GET / and GET /login.
			path := r.URL.Path
			if r.Method != http.MethodGet || (path != "/" && path != "/login") {
				next.ServeHTTP(w, r)
				return
			}

			jwt := r.Header.Get(cfJWTHeader)
			if jwt == "" {
				next.ServeHTTP(w, r)
				return
			}

			keys, err := getOrFetchJWKS(r.Context(), teamDomain, cache)
			if err != nil || len(keys) == 0 {
				// JWKS fetch failed -- fall through, don't block the request.
				next.ServeHTTP(w, r)
				return
			}

			if !validateCFJWT(jwt, keys) {
				// Invalid JWT -- fall through to normal auth flow.
				next.ServeHTTP(w, r)
				return
			}

			// Valid CF Access JWT: create a session and redirect to the dashboard.
			sessionID, err := sessions.Create()
			if err != nil {
				// Session creation failed -- fall through.
				next.ServeHTTP(w, r)
				return
			}

			sessions.SetCookie(w, sessionID)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})
	}
}

// getOrFetchJWKS returns cached keys or fetches fresh ones from the CF Access endpoint.
func getOrFetchJWKS(ctx context.Context, teamDomain string, cache *jwksCache) ([]*rsa.PublicKey, error) {
	if keys := cache.get(); keys != nil {
		return keys, nil
	}
	keys, err := fetchJWKS(ctx, teamDomain)
	if err != nil {
		return nil, err
	}
	cache.set(keys)
	return keys, nil
}

// getOrFetchSAASJWKS returns cached keys or fetches fresh ones from the CF Access
// SAAS app OIDC JWKS endpoint. The SAAS app uses different signing keys than the
// team-level /cdn-cgi/access/certs endpoint.
func getOrFetchSAASJWKS(ctx context.Context, teamDomain, saasClientID string, cache *jwksCache) ([]*rsa.PublicKey, error) {
	if keys := cache.get(); keys != nil {
		return keys, nil
	}
	jwksURL := "https://" + teamDomain + fmt.Sprintf(saasJWKSPathFmt, saasClientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("cf_access: build SAAS JWKS request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G704: URL is constructed from trusted CF_ACCESS_TEAM_DOMAIN + CF_ACCESS_SAAS_CLIENT_ID config values
	if err != nil {
		return nil, fmt.Errorf("cf_access: fetch SAAS JWKS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cf_access: SAAS JWKS endpoint returned %d", resp.StatusCode)
	}
	var body jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("cf_access: decode SAAS JWKS: %w", err)
	}
	keys := make([]*rsa.PublicKey, 0, len(body.Keys))
	for _, k := range body.Keys {
		if k.Alg != "RS256" {
			continue
		}
		pub, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		keys = append(keys, pub)
	}
	cache.set(keys)
	return keys, nil
}

// fetchJWKS retrieves RSA public keys from the Cloudflare Access JWKS endpoint.
func fetchJWKS(ctx context.Context, teamDomain string) ([]*rsa.PublicKey, error) {
	url := "https://" + teamDomain + jwksFetchPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("cf_access: build JWKS request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G704: URL is constructed from the trusted CF_ACCESS_TEAM_DOMAIN config value
	if err != nil {
		return nil, fmt.Errorf("cf_access: fetch JWKS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cf_access: JWKS endpoint returned %d", resp.StatusCode)
	}

	var body jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("cf_access: decode JWKS: %w", err)
	}

	keys := make([]*rsa.PublicKey, 0, len(body.Keys))
	for _, k := range body.Keys {
		if k.Alg != "RS256" {
			continue
		}
		pub, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		keys = append(keys, pub)
	}

	return keys, nil
}

// parseRSAPublicKey decodes base64url-encoded n and e into an *rsa.PublicKey.
func parseRSAPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("cf_access: decode n: %w", err)
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("cf_access: decode e: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

// jwtClaims holds the parsed subset of JWT claims we care about.
type jwtClaims struct {
	Aud string `json:"aud"`
	Exp int64  `json:"exp"`
}

// validateAndParseCFJWT parses and validates a Cloudflare Access JWT against the given
// RSA keys. Returns the parsed claims and true on success. Any parse or validation error
// returns an empty jwtClaims and false.
func validateAndParseCFJWT(token string, keys []*rsa.PublicKey) (jwtClaims, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return jwtClaims{}, false
	}

	// Decode header to confirm RS256.
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return jwtClaims{}, false
	}

	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return jwtClaims{}, false
	}
	if header.Alg != "RS256" {
		return jwtClaims{}, false
	}

	// Decode claims to check expiry.
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtClaims{}, false
	}

	var claims jwtClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return jwtClaims{}, false
	}
	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return jwtClaims{}, false
	}

	// Decode the signature.
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return jwtClaims{}, false
	}

	// Compute the message digest: SHA-256 of "header.payload".
	message := parts[0] + "." + parts[1]
	digest := sha256.Sum256([]byte(message))

	// Try each key; succeed on the first valid signature.
	for _, key := range keys {
		if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err == nil {
			return claims, true
		}
	}

	return jwtClaims{}, false
}

// validateCFJWT parses and validates a Cloudflare Access JWT against the given RSA keys.
// Returns true only when the signature is valid and the token has not expired.
// Any parse or validation error results in false (fall-through, no error to caller).
func validateCFJWT(token string, keys []*rsa.PublicKey) bool {
	_, ok := validateAndParseCFJWT(token, keys)
	return ok
}

// CFAccessOrBearerAuth returns middleware that accepts three credential paths:
//
//  1. Cf-Access-Jwt-Assertion header — CF tunnel JWT validated against clientID (self-hosted AUD).
//  2. Authorization: Bearer <jwt> — OIDC-issued JWT from the OAuth flow, validated against
//     saasClientID (SAAS app AUD). Used when Claude.ai presents the access_token after completing
//     the /authorize → /token flow.
//  3. Authorization: Bearer <static> — static token for internal/LAN tools (Claude Code, agents).
//
// Returns 401 if no credential is present or valid.
func CFAccessOrBearerAuth(teamDomain, clientID, saasClientID, token string, keyStore *KeyStore) func(http.Handler) http.Handler {
	saasCache := &jwksCache{}
	return func(next http.Handler) http.Handler {
		cfHandler := CFAccessEnforceMiddleware(teamDomain, clientID)(next)
		bearerHandler := BearerAuth(token, keyStore)(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Path 1: CF tunnel injects JWT into dedicated header.
			if r.Header.Get(cfJWTHeader) != "" {
				cfHandler.ServeHTTP(w, r)
				return
			}

			// Path 2: OIDC access_token presented as Bearer JWT (after OAuth flow).
			// Uses the SAAS app OIDC JWKS — different keys from the team /certs endpoint.
			if saasClientID != "" {
				if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
					bearer := strings.TrimPrefix(auth, "Bearer ")
					if strings.Count(bearer, ".") == 2 { // three-part JWT
						keys, err := getOrFetchSAASJWKS(r.Context(), teamDomain, saasClientID, saasCache)
						if err == nil && len(keys) > 0 {
							claims, ok := validateAndParseCFJWT(bearer, keys)
							if ok && claims.Aud == saasClientID {
								next.ServeHTTP(w, r)
								return
							}
						}
					}
				}
			}

			// Path 3: Static bearer token for internal tools.
			bearerHandler.ServeHTTP(w, r)
		})
	}
}

// CFAccessEnforceMiddleware rejects all requests that do not carry a valid
// Cloudflare Access JWT in the Cf-Access-Jwt-Assertion header.
// Returns 401 JSON on missing/invalid JWT or aud mismatch.
// Only active when both teamDomain and clientID are non-empty.
func CFAccessEnforceMiddleware(teamDomain, clientID string) func(http.Handler) http.Handler {
	if teamDomain == "" || clientID == "" {
		return func(next http.Handler) http.Handler { return next }
	}
	return cfAccessEnforceWithCache(teamDomain, clientID, &jwksCache{})
}

// cfAccessEnforceWithCache builds the enforcement middleware with the given JWKS cache.
// Separated from CFAccessEnforceMiddleware to allow pre-warming the cache in tests.
func cfAccessEnforceWithCache(teamDomain, clientID string, cache *jwksCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jwt := r.Header.Get(cfJWTHeader)
			if jwt == "" {
				writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "CF Access JWT required"})
				return
			}

			keys, err := getOrFetchJWKS(r.Context(), teamDomain, cache)
			if err != nil || len(keys) == 0 {
				// JWKS fetch failed — fail closed.
				writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "CF Access JWT invalid"})
				return
			}

			claims, ok := validateAndParseCFJWT(jwt, keys)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "CF Access JWT invalid"})
				return
			}

			if claims.Aud != clientID {
				writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "CF Access JWT invalid"})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
