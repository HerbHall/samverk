package api

import (
	"net/http"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

// sessionResponse is the JSON representation of a session.
type sessionResponse struct {
	ID          string  `json:"id"`
	IssueNumber int     `json:"issue_number"`
	AgentType   string  `json:"agent_type"`
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	Status      string  `json:"status"`
	StartedAt   string  `json:"started_at"`
	FinishedAt  *string `json:"finished_at,omitempty"`
	Error       string  `json:"error,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// toSessionResponse converts a models.Session to the API response type.
func toSessionResponse(s *models.Session) sessionResponse {
	r := sessionResponse{
		ID:          s.ID,
		IssueNumber: s.IssueNumber,
		AgentType:   s.AgentType,
		Provider:    s.Provider,
		Model:       s.Model,
		Status:      string(s.Status),
		StartedAt:   s.StartedAt.Format(time.RFC3339),
		Error:       s.Error,
		CreatedAt:   s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   s.UpdatedAt.Format(time.RFC3339),
	}
	if s.FinishedAt != nil {
		ts := s.FinishedAt.Format(time.RFC3339)
		r.FinishedAt = &ts
	}
	return r
}

// handleListSessions handles GET /api/v1/sessions.
// Supports query param: status (defaults to all sessions via empty status).
func (a *API) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeJSON(w, http.StatusOK, []sessionResponse{})
		return
	}

	status := models.SessionStatus(r.URL.Query().Get("status"))

	sessions, err := a.store.ListSessions(r.Context(), status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	resp := make([]sessionResponse, 0, len(sessions))
	for _, s := range sessions {
		resp = append(resp, toSessionResponse(s))
	}

	writeJSON(w, http.StatusOK, resp)
}

// costResponse is the JSON representation of a cost summary.
type costResponse struct {
	TokensUsed         int     `json:"tokens_used"`
	EstimatedCostUSD   float64 `json:"estimated_cost_usd"`
	BudgetRemainingUSD float64 `json:"budget_remaining_usd"`
}

// handleGetCosts handles GET /api/v1/costs.
// Supports query param: since (duration, defaults to 24h).
func (a *API) handleGetCosts(w http.ResponseWriter, r *http.Request) {
	if a.costs == nil {
		writeJSON(w, http.StatusOK, costResponse{})
		return
	}

	sinceStr := r.URL.Query().Get("since")
	if sinceStr == "" {
		sinceStr = "24h"
	}

	dur, err := time.ParseDuration(sinceStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid since duration")
		return
	}

	since := time.Now().Add(-dur)
	summary := a.costs.ComputeCostSince(r.Context(), since)

	writeJSON(w, http.StatusOK, costResponse{
		TokensUsed:         summary.TokensUsed,
		EstimatedCostUSD:   summary.EstimatedCostUSD,
		BudgetRemainingUSD: summary.BudgetRemainingUSD,
	})
}
