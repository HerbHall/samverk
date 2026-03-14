package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// handleFailureSummary returns an aggregated failure summary for the requested
// time window. Query parameters:
//   - hours: lookback window in hours (default: 24)
//   - limit: max recent failure events to include (default: 50)
func (a *API) handleFailureSummary(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		http.Error(w, "store not configured", http.StatusServiceUnavailable)
		return
	}

	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		if parsed, err := strconv.Atoi(h); err == nil && parsed > 0 {
			hours = parsed
		}
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	summary, err := a.store.GetFailureSummary(r.Context(), since)
	if err != nil {
		a.logger.Error("failure summary", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	events, err := a.store.ListFailureEvents(r.Context(), since, limit)
	if err != nil {
		a.logger.Error("list failure events", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := struct {
		Summary interface{} `json:"summary"`
		Events  interface{} `json:"events"`
	}{
		Summary: summary,
		Events:  events,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}