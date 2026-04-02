package api

import (
	"net/http"
	"strconv"
	"time"

	"samverk.dev/samverk/internal/logstore"
)

// handleListLogs serves GET /api/v1/logs with optional query filters.
func (a *API) handleListLogs(w http.ResponseWriter, r *http.Request) {
	if a.logStore == nil {
		writeError(w, http.StatusServiceUnavailable, "log store not configured")
		return
	}

	q := r.URL.Query()
	f := logstore.QueryFilter{
		Level:     q.Get("level"),
		Component: q.Get("component"),
		SessionID: q.Get("session_id"),
		Search:    q.Get("q"),
	}

	if v := q.Get("issue"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "issue must be an integer")
			return
		}
		f.Issue = n
	}

	if v := q.Get("since"); v != "" {
		t, err := parseSinceParam(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid since parameter: "+err.Error())
			return
		}
		f.Since = t
	}

	if v := q.Get("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid until parameter: "+err.Error())
			return
		}
		f.Until = t
	}

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "limit must be an integer")
			return
		}
		f.Limit = n
	}

	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "offset must be an integer")
			return
		}
		f.Offset = n
	}

	entries, err := a.logStore.Query(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed: "+err.Error())
		return
	}

	if entries == nil {
		entries = []logstore.LogEntry{}
	}

	writeJSON(w, http.StatusOK, entries)
}

// parseSinceParam parses a since value that is either an RFC3339 timestamp
// or a Go duration string (e.g., "24h", "720h").
func parseSinceParam(v string) (time.Time, error) {
	// Try RFC3339 first.
	t, err := time.Parse(time.RFC3339, v)
	if err == nil {
		return t, nil
	}

	// Try as a Go duration (relative to now).
	d, dErr := time.ParseDuration(v)
	if dErr != nil {
		return time.Time{}, dErr
	}
	return time.Now().Add(-d), nil
}
