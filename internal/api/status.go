package api

import (
	"net/http"
)

// statusResponse is the JSON representation of system status.
type statusResponse struct {
	Healthy           bool `json:"healthy"`
	ForgeConnected    bool `json:"forge_connected"`
	DatabaseConnected bool `json:"database_connected"`
}

// handleStatus handles GET /api/v1/status.
// Returns the health of external dependencies.
func (a *API) handleStatus(w http.ResponseWriter, _ *http.Request) {
	resp := statusResponse{
		ForgeConnected:    a.tracker != nil,
		DatabaseConnected: a.store != nil,
	}
	resp.Healthy = resp.ForgeConnected && resp.DatabaseConnected

	writeJSON(w, http.StatusOK, resp)
}
