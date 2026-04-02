package api

import (
	"net/http"

	"samverk.dev/samverk/internal/advisor"
	"samverk.dev/samverk/pkg/models"
)

// SetAdvisor attaches the advisory engine to the API handler.
// May be called before RegisterRoutes. May be nil (recommendations will return empty).
func (a *API) SetAdvisor(adv *advisor.Advisor) {
	a.advisor = adv
}

// handleGetRecommendations returns the current quality advisory recommendations.
func (a *API) handleGetRecommendations(w http.ResponseWriter, r *http.Request) {
	var recs []models.QualityRecommendation
	if a.advisor != nil {
		recs = a.advisor.Recommendations()
	}
	if recs == nil {
		recs = []models.QualityRecommendation{}
	}
	writeJSON(w, http.StatusOK, recs)
}
