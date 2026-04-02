package api

import (
	"net/http"
	"strings"
	"time"

	"samverk.dev/samverk/internal/loganalyst"
)

// handleLogSummary returns an AI-powered or fallback summary of logs
// matching the requested scope.
//
//	GET /api/v1/logs/summary?scope=session&id=sess_123
//	GET /api/v1/logs/summary?scope=failures&since=24h
func (a *API) handleLogSummary(w http.ResponseWriter, r *http.Request) {
	if a.logAnalyst == nil {
		writeError(w, http.StatusServiceUnavailable, "log analyst not configured")
		return
	}

	q := r.URL.Query()
	scopeStr := q.Get("scope")
	if scopeStr == "" {
		writeError(w, http.StatusBadRequest, "scope parameter is required (session, issue, timerange, failures)")
		return
	}

	scope := loganalyst.ScopeFilter{
		Type: loganalyst.ScopeType(scopeStr),
		ID:   q.Get("id"),
	}

	// Parse "since" as either RFC3339 or a Go duration string (e.g. "24h").
	if sinceStr := q.Get("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			scope.Since = t
		} else if d, err := time.ParseDuration(sinceStr); err == nil {
			scope.Since = time.Now().Add(-d)
		} else {
			writeError(w, http.StatusBadRequest, "since must be RFC3339 or duration (e.g. 24h)")
			return
		}
	}

	if untilStr := q.Get("until"); untilStr != "" {
		t, err := time.Parse(time.RFC3339, untilStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "until must be RFC3339")
			return
		}
		scope.Until = t
	}

	summary, err := a.logAnalyst.Summarize(r.Context(), scope)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "requires") || strings.Contains(msg, "unknown scope") || strings.Contains(msg, "invalid") {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		writeError(w, http.StatusInternalServerError, "log analysis failed: "+msg)
		return
	}

	writeJSON(w, http.StatusOK, summary)
}
