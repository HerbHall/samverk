package api

import (
	"net/http"
	"strconv"
)

// issueCostResponse is the JSON representation of per-issue cost aggregation.
type issueCostResponse struct {
	IssueNumber       int     `json:"issue_number"`
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	SessionCount      int     `json:"session_count"`
}

// handleGetIssueCost handles GET /api/v1/issues/{number}/cost.
func (a *API) handleGetIssueCost(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeJSON(w, http.StatusOK, issueCostResponse{})
		return
	}

	numStr := r.PathValue("number")
	num, err := strconv.Atoi(numStr)
	if err != nil || num < 1 {
		writeError(w, http.StatusBadRequest, "invalid issue number")
		return
	}

	summary, err := a.store.ComputeCostForIssue(r.Context(), num)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to compute issue cost")
		return
	}

	writeJSON(w, http.StatusOK, issueCostResponse{
		IssueNumber:       num,
		TotalInputTokens:  summary.TotalInputTokens,
		TotalOutputTokens: summary.TotalOutputTokens,
		TotalCostUSD:      summary.TotalCostUSD,
		SessionCount:      summary.RecordCount,
	})
}
