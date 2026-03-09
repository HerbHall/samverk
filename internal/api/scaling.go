package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

// scalingControlDTO is the JSON-serializable form of models.ScalingControl.
type scalingControlDTO struct {
	Paused        bool   `json:"paused"`
	ManualWorkers int    `json:"manual_workers"`
	SetAt         string `json:"set_at,omitempty"`
	Note          string `json:"note,omitempty"`
}

// scalingSetRequest is the body for POST /api/v1/scaling/set.
type scalingSetRequest struct {
	Workers int    `json:"workers"`
	Note    string `json:"note,omitempty"`
}

// handleGetScalingControl handles GET /api/v1/scaling/control.
func (a *API) handleGetScalingControl(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	ctrl, err := a.store.GetScalingControl(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read scaling control")
		return
	}
	dto := scalingControlDTO{
		Paused:        ctrl.Paused,
		ManualWorkers: ctrl.ManualWorkers,
		Note:          ctrl.Note,
	}
	if !ctrl.SetAt.IsZero() {
		dto.SetAt = ctrl.SetAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, dto)
}

// handleScalingPause handles POST /api/v1/scaling/pause.
func (a *API) handleScalingPause(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	ctrl := models.ScalingControl{
		Paused: true,
		SetAt:  time.Now().UTC(),
		Note:   "paused via API",
	}
	if err := a.store.UpsertScalingControl(r.Context(), ctrl); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update scaling control")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

// handleScalingResume handles POST /api/v1/scaling/resume.
func (a *API) handleScalingResume(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	ctrl := models.ScalingControl{
		Paused:        false,
		ManualWorkers: 0,
		SetAt:         time.Now().UTC(),
		Note:          "resumed via API",
	}
	if err := a.store.UpsertScalingControl(r.Context(), ctrl); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update scaling control")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

// handleScalingSet handles POST /api/v1/scaling/set.
func (a *API) handleScalingSet(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	var req scalingSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Workers < 1 {
		writeError(w, http.StatusBadRequest, "workers must be >= 1")
		return
	}
	note := req.Note
	if note == "" {
		note = "set via API"
	}
	ctrl := models.ScalingControl{
		Paused:        true, // pause autonomous scaling while manual target is set
		ManualWorkers: req.Workers,
		SetAt:         time.Now().UTC(),
		Note:          note,
	}
	if err := a.store.UpsertScalingControl(r.Context(), ctrl); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update scaling control")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"manual_workers": req.Workers,
		"note":           note,
	})
}
