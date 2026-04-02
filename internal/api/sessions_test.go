package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"samverk.dev/samverk/internal/api"
	"samverk.dev/samverk/internal/logstore"
)

// newLogStoreServer creates an API with a real logstore wired via SetLogStore
// and returns the httptest.Server.
func newLogStoreServer(t *testing.T, ls *logstore.LogStore) *httptest.Server {
	t.Helper()
	a := api.New(nil, nil, nil, zap.NewNop())
	if ls != nil {
		a.SetLogStore(ls)
	}
	mux := http.NewServeMux()
	a.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// newTestLogStore opens a temporary SQLite logstore and registers cleanup.
func newTestLogStore(t *testing.T) *logstore.LogStore {
	t.Helper()
	dir := t.TempDir()
	ls, err := logstore.New(filepath.Join(dir, "logs.db"))
	if err != nil {
		t.Fatalf("logstore.New: %v", err)
	}
	t.Cleanup(func() { _ = ls.Close() })
	return ls
}

func TestGetSessionLog(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		setupLS     func(t *testing.T) *logstore.LogStore
		path        string
		wantStatus  int
		wantLen     int
		wantNonNull bool // entries must not be JSON null
		wantError   string
	}{
		{
			name: "happy path - session has entries",
			setupLS: func(t *testing.T) *logstore.LogStore {
				t.Helper()
				ls := newTestLogStore(t)
				entries := []*logstore.LogEntry{
					{Timestamp: now, Level: "info", Message: "task started", Component: "agent", SessionID: "sess-1"},
					{Timestamp: now.Add(time.Second), Level: "info", Message: "task done", Component: "agent", SessionID: "sess-1"},
					{Timestamp: now, Level: "debug", Message: "other session", Component: "agent", SessionID: "sess-2"},
				}
				for _, e := range entries {
					if err := ls.Insert(context.Background(), e); err != nil {
						t.Fatalf("insert: %v", err)
					}
				}
				return ls
			},
			path:        "/api/v1/sessions/sess-1/log",
			wantStatus:  http.StatusOK,
			wantLen:     2,
			wantNonNull: true,
		},
		{
			name: "empty result - no entries for session",
			setupLS: func(t *testing.T) *logstore.LogStore {
				t.Helper()
				ls := newTestLogStore(t)
				// Insert an entry for a different session to confirm filtering.
				if err := ls.Insert(context.Background(), &logstore.LogEntry{
					Timestamp: now, Level: "info", Message: "other", Component: "agent", SessionID: "sess-other",
				}); err != nil {
					t.Fatalf("insert: %v", err)
				}
				return ls
			},
			path:        "/api/v1/sessions/sess-empty/log",
			wantStatus:  http.StatusOK,
			wantLen:     0,
			wantNonNull: true,
		},
		{
			name:       "nil logStore returns 404",
			setupLS:    func(_ *testing.T) *logstore.LogStore { return nil },
			path:       "/api/v1/sessions/sess-1/log",
			wantStatus: http.StatusNotFound,
			wantError:  "log store not configured",
		},
		{
			name: "invalid since param returns 400",
			setupLS: func(t *testing.T) *logstore.LogStore {
				t.Helper()
				return newTestLogStore(t)
			},
			path:       "/api/v1/sessions/sess-1/log?since=notadate",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid since parameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls := tt.setupLS(t)
			ts := newLogStoreServer(t, ls)

			resp := doGet(t, ts.URL+tt.path)
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			if tt.wantError != "" {
				var body map[string]string
				decodeJSON(t, resp, &body)
				if body["error"] == "" {
					t.Errorf("error field missing, got body: %v", body)
				}
				// Verify the error message contains the expected substring.
				if got := body["error"]; len(got) < len(tt.wantError) || got[:len(tt.wantError)] != tt.wantError {
					t.Errorf("error = %q, want prefix %q", got, tt.wantError)
				}
				return
			}

			var body struct {
				SessionID string              `json:"session_id"`
				Entries   []json.RawMessage   `json:"entries"`
			}
			decodeJSON(t, resp, &body)

			if tt.wantNonNull && body.Entries == nil {
				t.Error("entries = null, want non-null array")
			}
			if len(body.Entries) != tt.wantLen {
				t.Errorf("entries count = %d, want %d", len(body.Entries), tt.wantLen)
			}
		})
	}
}
