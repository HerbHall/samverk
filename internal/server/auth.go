package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// BearerAuth returns middleware that validates Bearer token authentication.
// It checks the env-var token first (backwards compatible), then falls back
// to the KeyStore if provided. If both token and keyStore are empty/nil,
// the middleware is a no-op (passes through).
func BearerAuth(token string, keyStore *KeyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" && keyStore == nil {
				next.ServeHTTP(w, r)
				return
			}

			// bearerUnauthorized writes a 401 with WWW-Authenticate so that MCP
			// clients (e.g. Claude.ai) recognise bearer-token auth and do not
			// fall back to opening a browser-based login flow.
			bearerUnauthorized := func(msg string) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="Samverk"`)
				writeJSON(w, http.StatusUnauthorized, errorResponse{Error: msg})
			}

			auth := r.Header.Get("Authorization")
			if auth == "" {
				bearerUnauthorized("missing authorization header")
				return
			}

			const prefix = "Bearer "
			if !strings.HasPrefix(auth, prefix) {
				bearerUnauthorized("invalid authorization format")
				return
			}

			bearerToken := auth[len(prefix):]

			// Try env-var token first (constant-time comparison).
			if token != "" && subtle.ConstantTimeCompare([]byte(bearerToken), []byte(token)) == 1 {
				next.ServeHTTP(w, r)
				return
			}

			// Fall back to KeyStore validation.
			if keyStore != nil {
				if _, ok := keyStore.Validate(bearerToken); ok {
					next.ServeHTTP(w, r)
					return
				}
			}

			bearerUnauthorized("invalid token")
		})
	}
}

// BearerOrSessionAuth returns middleware that accepts EITHER a valid Bearer
// token (for programmatic/MCP access) OR a valid session cookie (for SPA
// browser access). If auth is not configured (empty token + nil keyStore +
// nil sessions), the middleware is a no-op.
func BearerOrSessionAuth(token string, keyStore *KeyStore, sessions *SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" && keyStore == nil && sessions == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Try session cookie first (browser access).
			if sessions != nil {
				if id := GetSessionID(r); id != "" && sessions.Validate(id) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Try Bearer token (programmatic access).
			auth := r.Header.Get("Authorization")
			if auth != "" {
				const prefix = "Bearer "
				if strings.HasPrefix(auth, prefix) {
					bearerToken := auth[len(prefix):]

					if token != "" && subtle.ConstantTimeCompare([]byte(bearerToken), []byte(token)) == 1 {
						next.ServeHTTP(w, r)
						return
					}

					if keyStore != nil {
						if _, ok := keyStore.Validate(bearerToken); ok {
							next.ServeHTTP(w, r)
							return
						}
					}
				}
			}

			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "authentication required"})
		})
	}
}

// requireSession is middleware that redirects unauthenticated users to /login.
// Used for the SPA and static file serving.
func requireSession(sessions *SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sessions == nil {
				next.ServeHTTP(w, r)
				return
			}

			id := GetSessionID(r)
			if id == "" || !sessions.Validate(id) {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
