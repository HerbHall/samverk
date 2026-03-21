package server

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockSessionStore satisfies cfSessionStore for tests.
type mockSessionStore struct {
	created  []string // IDs that were created
	createFn func() (string, error)
}

func (m *mockSessionStore) Create() (string, error) {
	if m.createFn != nil {
		id, err := m.createFn()
		if err == nil {
			m.created = append(m.created, id)
		}
		return id, err
	}
	id := fmt.Sprintf("sess-%d", len(m.created)+1)
	m.created = append(m.created, id)
	return id, nil
}

func (m *mockSessionStore) SetCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:  sessionCookieName,
		Value: sessionID,
		Path:  "/",
	})
}

// encodeExponent converts an RSA public exponent int to big-endian bytes,
// suitable for base64url encoding in a JWKS response.
func encodeExponent(e int) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(e))
	// Trim leading zero bytes (as required by JWKS spec).
	for len(b) > 1 && b[0] == 0 {
		b = b[1:]
	}
	return b
}

// buildTestJWT signs a minimal RS256 JWT using the given key and expiry.
func buildTestJWT(t *testing.T, key *rsa.PrivateKey, exp int64) string {
	t.Helper()

	headerJSON, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claimsJSON, _ := json.Marshal(map[string]any{"sub": "test", "exp": exp})

	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	message := header + "." + payload

	digest := sha256.Sum256([]byte(message))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}

	return message + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// TestCFAccessMiddleware_Disabled verifies that an empty teamDomain is a no-op.
func TestCFAccessMiddleware_Disabled(t *testing.T) {
	store := &mockSessionStore{}
	mw := CFAccessMiddleware("", store)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set(cfJWTHeader, "anything")
	rr := httptest.NewRecorder()

	mw(next).ServeHTTP(rr, req)

	if !called {
		t.Error("expected next handler to be called when teamDomain is empty")
	}
	if len(store.created) != 0 {
		t.Errorf("expected no sessions created, got %d", len(store.created))
	}
}

// TestCFAccessMiddleware_NoHeader verifies that missing JWT header passes through.
func TestCFAccessMiddleware_NoHeader(t *testing.T) {
	store := &mockSessionStore{}
	mw := CFAccessMiddleware("example.cloudflareaccess.com", store)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/login", http.NoBody)
	// Deliberately no Cf-Access-Jwt-Assertion header.
	rr := httptest.NewRecorder()

	mw(next).ServeHTTP(rr, req)

	if !called {
		t.Error("expected next handler to be called when JWT header is absent")
	}
	if len(store.created) != 0 {
		t.Errorf("expected no sessions created, got %d", len(store.created))
	}
}

// TestCFAccessMiddleware_InvalidJWT verifies that a malformed JWT passes through.
func TestCFAccessMiddleware_InvalidJWT(t *testing.T) {
	// Pre-warm the cache with a real key so the JWKS fetch is skipped, and the
	// test exercises JWT parsing failure without a network call.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
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

	store := &mockSessionStore{}
	mw := cfAccessMiddlewareWithCache("example.cloudflareaccess.com", store, cache)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set(cfJWTHeader, "not.a.valid.jwt")
	rr := httptest.NewRecorder()

	mw(next).ServeHTTP(rr, req)

	if !called {
		t.Error("expected next handler to be called when JWT is malformed")
	}
	if len(store.created) != 0 {
		t.Errorf("expected no sessions created on invalid JWT, got %d", len(store.created))
	}
}

// TestCFAccessMiddleware_ValidJWT exercises the full happy path with a real RSA key.
func TestCFAccessMiddleware_ValidJWT(t *testing.T) {
	// Generate a throwaway RSA key pair for this test.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	nBytes := key.N.Bytes()
	eBytes := encodeExponent(key.E)

	// Build a valid RS256 JWT with a future expiry.
	jwtToken := buildTestJWT(t, key, time.Now().Add(time.Hour).Unix())

	t.Run("parseRSAPublicKey", func(t *testing.T) {
		pub, parseErr := parseRSAPublicKey(
			base64.RawURLEncoding.EncodeToString(nBytes),
			base64.RawURLEncoding.EncodeToString(eBytes),
		)
		if parseErr != nil {
			t.Fatalf("parseRSAPublicKey: %v", parseErr)
		}
		if pub.N.Cmp(key.N) != 0 {
			t.Error("N mismatch")
		}
		if pub.E != key.E {
			t.Errorf("E mismatch: want %d got %d", key.E, pub.E)
		}
	})

	t.Run("validateCFJWT_valid", func(t *testing.T) {
		pub, _ := parseRSAPublicKey(
			base64.RawURLEncoding.EncodeToString(nBytes),
			base64.RawURLEncoding.EncodeToString(eBytes),
		)
		if !validateCFJWT(jwtToken, []*rsa.PublicKey{pub}) {
			t.Error("expected valid JWT to pass validation")
		}
	})

	t.Run("validateCFJWT_expired", func(t *testing.T) {
		expired := buildTestJWT(t, key, time.Now().Add(-time.Hour).Unix())
		pub, _ := parseRSAPublicKey(
			base64.RawURLEncoding.EncodeToString(nBytes),
			base64.RawURLEncoding.EncodeToString(eBytes),
		)
		if validateCFJWT(expired, []*rsa.PublicKey{pub}) {
			t.Error("expected expired JWT to fail validation")
		}
	})

	t.Run("validateCFJWT_wrong_key", func(t *testing.T) {
		otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		pub := &otherKey.PublicKey
		if validateCFJWT(jwtToken, []*rsa.PublicKey{pub}) {
			t.Error("expected JWT signed with different key to fail validation")
		}
	})

	t.Run("middleware_redirect_on_valid_jwt", func(t *testing.T) {
		// Pre-warm cache with the test public key.
		pub, _ := parseRSAPublicKey(
			base64.RawURLEncoding.EncodeToString(nBytes),
			base64.RawURLEncoding.EncodeToString(eBytes),
		)
		cache := &jwksCache{}
		cache.set([]*rsa.PublicKey{pub})

		sess := &mockSessionStore{}
		innerMW := cfAccessMiddlewareWithCache("any-domain", sess, cache)

		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/login", http.NoBody)
		req.Header.Set(cfJWTHeader, jwtToken)
		rr := httptest.NewRecorder()

		innerMW(next).ServeHTTP(rr, req)

		if called {
			t.Error("expected next handler NOT to be called on valid JWT (redirect expected)")
		}
		if rr.Code != http.StatusSeeOther {
			t.Errorf("expected 303 redirect, got %d", rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != "/" {
			t.Errorf("expected redirect to /, got %q", loc)
		}
		if len(sess.created) != 1 {
			t.Errorf("expected 1 session created, got %d", len(sess.created))
		}
	})

	t.Run("middleware_non_spa_path_passes_through", func(t *testing.T) {
		// JWT present but path is /api/v1/something -- must not intercept.
		pub, _ := parseRSAPublicKey(
			base64.RawURLEncoding.EncodeToString(nBytes),
			base64.RawURLEncoding.EncodeToString(eBytes),
		)
		cache := &jwksCache{}
		cache.set([]*rsa.PublicKey{pub})

		sess := &mockSessionStore{}
		innerMW := cfAccessMiddlewareWithCache("any-domain", sess, cache)

		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/kpis", http.NoBody)
		req.Header.Set(cfJWTHeader, jwtToken)
		rr := httptest.NewRecorder()

		innerMW(next).ServeHTTP(rr, req)

		if !called {
			t.Error("expected next handler to be called for non-SPA path")
		}
		if len(sess.created) != 0 {
			t.Errorf("expected no sessions for API path, got %d", len(sess.created))
		}
	})
}
