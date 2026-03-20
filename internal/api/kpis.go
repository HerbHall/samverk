package api

import (
	"net/http"

	"go.uber.org/zap"
)

// handleGetKPIs returns the rolling KPI report computed from failure_events.
func (a *API) handleGetKPIs(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not configured")
		return
	}

	report, err := a.store.GetKPIReport(r.Context())
	if err != nil {
		a.logger.Error("get kpi report", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, report)
}
