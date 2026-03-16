package api

import (
	"net/http"

	"github.com/herbhall/samverk/internal/provider"
)

// SetHealthMonitor attaches the provider health monitor for the
// GET /api/v1/providers/health endpoint. Call from the serve or
// dispatch command after creating the HealthMonitor.
func (a *API) SetHealthMonitor(hm *provider.HealthMonitor) {
	a.healthMonitor = hm
}

// handleProviderHealth returns the cached health status for all providers.
func (a *API) handleProviderHealth(w http.ResponseWriter, _ *http.Request) {
	if a.healthMonitor == nil {
		writeError(w, http.StatusServiceUnavailable, "health monitor not configured")
		return
	}

	health := a.healthMonitor.AllHealth()
	writeJSON(w, http.StatusOK, health)
}
