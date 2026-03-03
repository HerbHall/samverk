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

			auth := r.Header.Get("Authorization")
			if auth == "" {
				writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing authorization header"})
				return
			}

			const prefix = "Bearer "
			if !strings.HasPrefix(auth, prefix) {
				writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid authorization format"})
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

			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid token"})
		})
	}
}
