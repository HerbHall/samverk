package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/herbhall/samverk/internal/digest"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/store"
)

// API provides REST endpoints for the web dashboard.
type API struct {
	tracker forge.IssueTracker // may be nil if no forge credentials
	store   store.Store        // may be nil if no database
	costs   digest.CostSource  // may be nil if no cost tracking
}

// New creates an API handler with the given dependencies.
// Any dependency may be nil; endpoints that require a nil dependency
// return 503 Service Unavailable.
func New(tracker forge.IssueTracker, s store.Store, costs digest.CostSource) *API {
	return &API{
		tracker: tracker,
		store:   s,
		costs:   costs,
	}
}

// RegisterRoutes registers all API endpoints on the given mux.
// Routes use Go 1.22+ method+path patterns.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/issues", a.handleListIssues)
	mux.HandleFunc("GET /api/v1/issues/{number}", a.handleGetIssue)
	mux.HandleFunc("GET /api/v1/sessions", a.handleListSessions)
	mux.HandleFunc("GET /api/v1/costs", a.handleGetCosts)
	mux.HandleFunc("GET /api/v1/status", a.handleStatus)
}

// errorResponse is the JSON body returned for error responses.
type errorResponse struct {
	Error string `json:"error"`
}

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("api: failed to encode JSON response", "err", err)
	}
}

// writeError writes a JSON error response with the given status code.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
