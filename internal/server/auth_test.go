package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/herbhall/samverk/internal/server"
)

func TestBearerAuth(t *testing.T) {
	// okHandler is the downstream handler that returns 200 when reached.
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	tests := []struct {
		name       string
		token      string // middleware config token; empty = no-op mode
		authHeader string // Authorization header value
		wantStatus int
		wantError  string // expected error field in JSON body; empty if no error
	}{
		{
			name:       "valid token",
			token:      "secret-token-123",
			authHeader: "Bearer secret-token-123",
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing authorization header",
			token:      "secret-token-123",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
			wantError:  "missing authorization header",
		},
		{
			name:       "wrong format no bearer prefix",
			token:      "secret-token-123",
			authHeader: "Basic c2VjcmV0",
			wantStatus: http.StatusUnauthorized,
			wantError:  "invalid authorization format",
		},
		{
			name:       "invalid token",
			token:      "secret-token-123",
			authHeader: "Bearer wrong-token",
			wantStatus: http.StatusUnauthorized,
			wantError:  "invalid token",
		},
		{
			name:       "empty config token passes through without header",
			token:      "",
			authHeader: "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "empty config token passes through with any header",
			token:      "",
			authHeader: "Bearer anything",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := server.BearerAuth(tt.token)(okHandler)

			req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantError != "" {
				var body map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if body["error"] != tt.wantError {
					t.Errorf("error = %q, want %q", body["error"], tt.wantError)
				}
			}
		})
	}
}
