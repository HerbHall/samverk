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

const (
	defaultLimit = 50
	maxLimit     = 200
)

// issueListResponse wraps the issues array with pagination metadata.
type issueListResponse struct {
	Issues  []issueResponse `json:"issues"`
	Total   int             `json:"total"`
	Limit   int             `json:"limit"`
	Offset  int             `json:"offset"`
	Page    int             `json:"page"`
	PerPage int             `json:"per_page"`
}

// handleListIssues handles GET /api/v1/issues.
// Supports query params: state, labels, limit, offset, page, per_page.
// limit/offset take precedence over page/per_page when provided.
func (a *API) handleListIssues(w http.ResponseWriter, r *http.Request) {
	if a.tracker == nil {
		writeError(w, http.StatusServiceUnavailable, "issue tracker not available")
		return
	}

	q := r.URL.Query()
	opts := &forge.ListOptions{}

	if state := q.Get("state"); state != "" {
		opts.State = forge.State(state)
	} else {
		opts.State = forge.StateOpen
	}

	if labels := q.Get("labels"); labels != "" {
		opts.Labels = strings.Split(labels, ",")
	}

	// Resolve limit/offset (new style) vs page/per_page (legacy style).
	limit := defaultLimit
	offset := 0

	hasLimit := q.Has("limit")
	hasOffset := q.Has("offset")

	if hasLimit {
		if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
			limit = n
		}
	}
	if hasOffset {
		if n, err := strconv.Atoi(q.Get("offset")); err == nil && n >= 0 {
			offset = n
		}
	}

	// Cap limit.
	if limit > maxLimit {
		limit = maxLimit
	}

	// Legacy page/per_page: only used when limit/offset are not provided.
	if !hasLimit && !hasOffset {
		if perPage := q.Get("per_page"); perPage != "" {
			if n, err := strconv.Atoi(perPage); err == nil && n > 0 {
				limit = n
				if limit > maxLimit {
					limit = maxLimit
				}
			}
		}
		if page := q.Get("page"); page != "" {
			if n, err := strconv.Atoi(page); err == nil && n > 0 {
				offset = (n - 1) * limit
			}
		}
	}

	// Compute 1-based page from offset/limit for the forge call.
	page := offset/limit + 1
	opts.Page = page
	opts.PerPage = limit

	issues, err := a.tracker.ListIssues(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list issues")
		return
	}

	issueList := make([]issueResponse, 0, len(issues))
	for _, iss := range issues {
		issueList = append(issueList, toIssueResponse(iss))
	}

	writeJSON(w, http.StatusOK, issueListResponse{
		Issues:  issueList,
		Total:   len(issues),
		Limit:   limit,
		Offset:  offset,
		Page:    page,
		PerPage: limit,
	})
}

// handleSearchIssues handles GET /api/v1/issues/search.
// Supports query params: q (search term), state (open|closed; default open).
func (a *API) handleSearchIssues(w http.ResponseWriter, r *http.Request) {
	if a.tracker == nil {
		writeError(w, http.StatusServiceUnavailable, "issue tracker not available")
		return
	}

	q := r.URL.Query()
	opts := &forge.SearchOptions{
		Query: q.Get("q"),
	}

	if state := q.Get("state"); state != "" {
		opts.State = forge.State(state)
	}

	issues, err := a.tracker.SearchIssues(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to search issues")
		return
	}

	issueList := make([]issueResponse, 0, len(issues))
	for _, iss := range issues {
		issueList = append(issueList, toIssueResponse(iss))
	}

	writeJSON(w, http.StatusOK, issueListResponse{
		Issues:  issueList,
		Total:   len(issueList),
		Limit:   len(issueList),
		Offset:  0,
		Page:    1,
		PerPage: len(issueList),
	})
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
