package api

import (
	"net/http"
	"time"

	"github.com/herbhall/samverk/internal/provider"
)

// SetHealthMonitor attaches the provider health monitor for the
// GET /api/v1/providers/health endpoint. Call from the serve or
// dispatch command after creating the HealthMonitor.
func (a *API) SetHealthMonitor(hm *provider.HealthMonitor) {
	a.healthMonitor = hm
}

// SetProviderRegistry attaches a provider registry so the health endpoint
// can include the routing_chains map. Call from serve after loading
// provider config. The registry may have no instantiated providers (routing
// metadata only).
func (a *API) SetProviderRegistry(reg *provider.Registry) {
	a.providerRegistry = reg
}

// healthEntry is the per-provider entry in the /api/v1/providers/health response.
type healthEntry struct {
	Name          string `json:"name"`
	Healthy       bool   `json:"healthy"`
	LastChecked   string `json:"last_checked,omitempty"`
	LastHealthy   string `json:"last_healthy,omitempty"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
	Error         string `json:"error,omitempty"`
	ModelLoaded   bool   `json:"model_loaded,omitempty"`
	VRAMFree      int64  `json:"vram_free_bytes,omitempty"`
	VRAMTotal     int64  `json:"vram_total_bytes,omitempty"`
}

// providerHealthResponse is the top-level JSON response for GET /api/v1/providers/health.
type providerHealthResponse struct {
	Providers     []healthEntry       `json:"providers"`
	Count         int                 `json:"count"`
	RoutingChains map[string][]string `json:"routing_chains,omitempty"`
}

// handleProviderHealth returns the cached health status for all providers,
// enriched with last_success_at timestamps from the store and routing chains
// from the provider registry.
func (a *API) handleProviderHealth(w http.ResponseWriter, r *http.Request) {
	if a.healthMonitor == nil {
		writeError(w, http.StatusServiceUnavailable, "health monitor not configured")
		return
	}

	allHealth := a.healthMonitor.AllHealth()

	// Fetch last-success timestamps per provider if store is available.
	var lastSuccess map[string]time.Time
	if a.store != nil {
		var storeErr error
		lastSuccess, storeErr = a.store.LatestSuccessByProvider(r.Context())
		if storeErr != nil {
			// Non-fatal: omit last_success_at rather than failing the whole response.
			lastSuccess = nil
		}
	}

	entries := make([]healthEntry, 0, len(allHealth))
	for i := range allHealth {
		ph := &allHealth[i]
		entry := healthEntry{
			Name:        ph.Name,
			Healthy:     ph.Healthy,
			Error:       ph.Error,
			ModelLoaded: ph.ModelLoaded,
			VRAMFree:    ph.VRAMFree,
			VRAMTotal:   ph.VRAMTotal,
		}
		if !ph.LastChecked.IsZero() {
			entry.LastChecked = ph.LastChecked.UTC().Format(time.RFC3339)
		}
		if !ph.LastHealthy.IsZero() {
			entry.LastHealthy = ph.LastHealthy.UTC().Format(time.RFC3339)
		}
		if lastSuccess != nil {
			if t, ok := lastSuccess[ph.Name]; ok {
				entry.LastSuccessAt = t.UTC().Format(time.RFC3339)
			} else {
				entry.LastSuccessAt = "never"
			}
		}
		entries = append(entries, entry)
	}

	resp := providerHealthResponse{
		Providers: entries,
		Count:     len(entries),
	}

	// Include routing chain configuration if provider registry is available.
	if a.providerRegistry != nil {
		resp.RoutingChains = a.providerRegistry.Routing()
	}

	writeJSON(w, http.StatusOK, resp)
}
