package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// BearerAuth returns middleware that validates Bearer token authentication.
// If token is empty, the middleware is a no-op (passes through).
func BearerAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
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

			if subtle.ConstantTimeCompare([]byte(auth[len(prefix):]), []byte(token)) != 1 {
				writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid token"})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
