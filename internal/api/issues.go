package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/herbhall/samverk/internal/forge"
)

// issueResponse is the JSON representation of a forge issue.
type issueResponse struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Labels    []string  `json:"labels"`
	Assignees []string  `json:"assignees"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
	ClosedAt  *string   `json:"closed_at,omitempty"`
}

// toIssueResponse converts a forge.Issue to the API response type.
func toIssueResponse(iss *forge.Issue) issueResponse {
	r := issueResponse{
		Number:    iss.Number,
		Title:     iss.Title,
		Body:      iss.Body,
		State:     string(iss.State),
		Labels:    iss.Labels,
		Assignees: iss.Assignees,
		CreatedAt: iss.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: iss.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if iss.ClosedAt != nil {
		s := iss.ClosedAt.Format("2006-01-02T15:04:05Z")
		r.ClosedAt = &s
	}
	// Ensure nil slices are serialized as empty arrays.
	if r.Labels == nil {
		r.Labels = []string{}
	}
	if r.Assignees == nil {
		r.Assignees = []string{}
	}
	return r
}

// handleListIssues handles GET /api/v1/issues.
// Supports query params: state, labels, page, per_page.
func (a *API) handleListIssues(w http.ResponseWriter, r *http.Request) {
	if a.tracker == nil {
		writeError(w, http.StatusServiceUnavailable, "issue tracker not available")
		return
	}

	opts := &forge.ListOptions{}

	if state := r.URL.Query().Get("state"); state != "" {
		opts.State = forge.State(state)
	} else {
		opts.State = forge.StateOpen
	}

	if labels := r.URL.Query().Get("labels"); labels != "" {
		opts.Labels = strings.Split(labels, ",")
	}

	if page := r.URL.Query().Get("page"); page != "" {
		if n, err := strconv.Atoi(page); err == nil && n > 0 {
			opts.Page = n
		}
	}

	if perPage := r.URL.Query().Get("per_page"); perPage != "" {
		if n, err := strconv.Atoi(perPage); err == nil && n > 0 {
			opts.PerPage = n
		}
	}

	issues, err := a.tracker.ListIssues(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list issues")
		return
	}

	resp := make([]issueResponse, 0, len(issues))
	for _, iss := range issues {
		resp = append(resp, toIssueResponse(iss))
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleGetIssue handles GET /api/v1/issues/{number}.
func (a *API) handleGetIssue(w http.ResponseWriter, r *http.Request) {
	if a.tracker == nil {
		writeError(w, http.StatusServiceUnavailable, "issue tracker not available")
		return
	}

	numStr := r.PathValue("number")
	num, err := strconv.Atoi(numStr)
	if err != nil || num < 1 {
		writeError(w, http.StatusBadRequest, "invalid issue number")
		return
	}

	issue, err := a.tracker.GetIssue(r.Context(), num)
	if err != nil {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}

	writeJSON(w, http.StatusOK, toIssueResponse(issue))
}
