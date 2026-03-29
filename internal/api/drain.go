package api

import (
	"encoding/json"
	"net/http"
)

// DrainResponse is the JSON representation of dispatcher drain status.
type DrainResponse struct {
	Draining      bool  `json:"draining"`
	ActiveWorkers int   `json:"active_workers"`
	ClaimedIssues []int `json:"claimed_issues"`
	QueueDepth    int   `json:"queue_depth"`
}

// drainController provides live drain control from the dispatcher.
type drainController struct {
	drainFn       func() DrainResponse
	statusFn      func() DrainResponse
	cancelDrainFn func() DrainResponse
}

// SetDrainController attaches drain control functions from the dispatcher.
// Call before registering routes. When set, drain endpoints become available.
func (a *API) SetDrainController(
	drainFn func() DrainResponse,
	statusFn func() DrainResponse,
	cancelDrainFn func() DrainResponse,
) {
	a.drainCtrl = &drainController{
		drainFn:       drainFn,
		statusFn:      statusFn,
		cancelDrainFn: cancelDrainFn,
	}
}

// handleDrainPost handles POST /api/v1/dispatcher/drain.
// Enters drain mode atomically and returns the current drain status.
func (a *API) handleDrainPost(w http.ResponseWriter, _ *http.Request) {
	if a.drainCtrl == nil {
		writeError(w, http.StatusServiceUnavailable, "drain not available (dispatch process only)")
		return
	}
	resp := a.drainCtrl.drainFn()
	writeJSON(w, http.StatusOK, resp)
}

// handleDrainGet handles GET /api/v1/dispatcher/drain.
// Returns the current live drain status without changing state.
func (a *API) handleDrainGet(w http.ResponseWriter, _ *http.Request) {
	if a.drainCtrl == nil {
		writeError(w, http.StatusServiceUnavailable, "drain not available (dispatch process only)")
		return
	}
	resp := a.drainCtrl.statusFn()
	writeJSON(w, http.StatusOK, resp)
}

// handleDrainDelete handles DELETE /api/v1/dispatcher/drain.
// Exits drain mode and returns the updated drain status.
func (a *API) handleDrainDelete(w http.ResponseWriter, _ *http.Request) {
	if a.drainCtrl == nil {
		writeError(w, http.StatusServiceUnavailable, "drain not available (dispatch process only)")
		return
	}
	resp := a.drainCtrl.cancelDrainFn()
	writeJSON(w, http.StatusOK, resp)
}

// RegisterDrainRoutes registers drain-related API endpoints on the given mux.
// Separated from RegisterRoutes because drain endpoints are only available
// on the dispatch process management server, not the main serve process.
func (a *API) RegisterDrainRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/dispatcher/drain", a.handleDrainPost)
	mux.HandleFunc("GET /api/v1/dispatcher/drain", a.handleDrainGet)
	mux.HandleFunc("DELETE /api/v1/dispatcher/drain", a.handleDrainDelete)
}

// DrainMux returns a standalone http.ServeMux with only the drain endpoints
// and a health check. Used by the dispatch command to run a lightweight
// management HTTP server.
func DrainMux(
	drainFn func() DrainResponse,
	statusFn func() DrainResponse,
	cancelDrainFn func() DrainResponse,
) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/dispatcher/drain", func(w http.ResponseWriter, _ *http.Request) {
		resp := drainFn()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("GET /api/v1/dispatcher/drain", func(w http.ResponseWriter, _ *http.Request) {
		resp := statusFn()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("DELETE /api/v1/dispatcher/drain", func(w http.ResponseWriter, _ *http.Request) {
		resp := cancelDrainFn()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}
