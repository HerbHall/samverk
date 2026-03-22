package api

import (
	"net/http"
)

// configReloadErrorSource provides the last project.yaml reload error.
// Implemented by *dispatcher.Dispatcher when hot-reload is configured.
type configReloadErrorSource interface {
	ConfigReloadError() error
}

// statusResponse is the JSON representation of system status.
type statusResponse struct {
	Healthy             bool   `json:"healthy"`
	ForgeConnected      bool   `json:"forge_connected"`
	DatabaseConnected   bool   `json:"database_connected"`
	ToolCount           int    `json:"tool_count"`
	ConfigReloadWarning string `json:"config_reload_warning,omitempty"`
}

// SetConfigReloadErrorSource attaches the dispatcher's config-reload-error source
// to the status endpoint. When set, /api/v1/status includes a config_reload_warning
// field when the last project.yaml reload attempt failed.
func (a *API) SetConfigReloadErrorSource(src configReloadErrorSource) {
	a.configReloadSrc = src
}

// handleStatus handles GET /api/v1/status.
// Returns the health of external dependencies.
func (a *API) handleStatus(w http.ResponseWriter, _ *http.Request) {
	resp := statusResponse{
		ForgeConnected:    a.tracker != nil,
		DatabaseConnected: a.store != nil,
		ToolCount:         a.toolCount,
	}
	resp.Healthy = resp.ForgeConnected && resp.DatabaseConnected

	if a.configReloadSrc != nil {
		if reloadErr := a.configReloadSrc.ConfigReloadError(); reloadErr != nil {
			resp.ConfigReloadWarning = reloadErr.Error()
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
