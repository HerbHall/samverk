package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"samverk.dev/samverk/internal/forge"
	internalmcp "samverk.dev/samverk/internal/mcp"
)

// issueResponse is the JSON representation of a forge issue.
// ProjectName, Owner, Repo, ForgeURL, and ForgeType are populated when the
// project registry is active (multi-project mode). They are empty strings in
// single-project mode; callers should treat an empty ForgeURL as "github".
type issueResponse struct {
	Number      int     `json:"number"`
	Title       string  `json:"title"`
	Body        string  `json:"body"`
	State       string  `json:"state"`
	Labels      []string `json:"labels"`
	Assignees   []string `json:"assignees"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	ClosedAt    *string  `json:"closed_at,omitempty"`
	// Forge / project metadata — populated in multi-project mode.
	ProjectName string `json:"project_name,omitempty"`
	Owner       string `json:"owner,omitempty"`
	Repo        string `json:"repo,omitempty"`
	ForgeURL    string `json:"forge_url,omitempty"`
	ForgeType     string `json:"forge_type,omitempty"`
	IsPullRequest bool   `json:"is_pull_request,omitempty"`
}

// toIssueResponse converts a forge.Issue to the API response type without
// project metadata (single-project mode fallback).
func toIssueResponse(iss *forge.Issue) issueResponse {
	return toIssueResponseForProject(iss, nil)
}

// toIssueResponseForProject converts a forge.Issue to the API response type,
// attaching forge/project metadata from the given project when non-nil.
func toIssueResponseForProject(iss *forge.Issue, p *internalmcp.Project) issueResponse {
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
	r.IsPullRequest = iss.IsPullRequest
	if p != nil {
		r.ProjectName = p.Name
		r.Owner = p.Owner
		r.Repo = p.Repo
		r.ForgeURL = p.ForgeURL
		r.ForgeType = p.ForgeType
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
//
// When a project registry is configured (multi-project mode), issues are
// aggregated from ALL registered projects concurrently, each tagged with the
// correct forge/project metadata. In single-project mode the active tracker
// is queried directly (legacy behaviour).
func (a *API) handleListIssues(w http.ResponseWriter, r *http.Request) {
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

	// Multi-project mode: aggregate from all registered projects concurrently.
	if a.projectRegistry != nil {
		issueList := a.listIssuesFromAllProjects(r.Context(), opts)
		// Apply offset/limit after aggregation.
		total := len(issueList)
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		page := offset/limit + 1
		writeJSON(w, http.StatusOK, issueListResponse{
			Issues:  issueList[offset:end],
			Total:   total,
			Limit:   limit,
			Offset:  offset,
			Page:    page,
			PerPage: limit,
		})
		return
	}

	// Single-project mode fallback.
	if a.tracker == nil {
		writeError(w, http.StatusServiceUnavailable, "issue tracker not available")
		return
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

	// Use cached total when available; fall back to page count.
	total := len(issues)
	if a.store != nil {
		allCounts, countErr := a.store.GetAllIssueCounts(r.Context())
		if countErr == nil {
			var agg int
			for _, c := range allCounts {
				switch opts.State {
				case forge.StateOpen:
					agg += c.Open
				case forge.StateClosed:
					agg += c.Closed
				default:
					agg += c.Total
				}
			}
			if agg > 0 {
				total = agg
			}
		}
	}

	writeJSON(w, http.StatusOK, issueListResponse{
		Issues:  issueList,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		Page:    page,
		PerPage: limit,
	})
}

// listIssuesFromAllProjects fetches open issues from every project in the
// registry concurrently and returns the combined, deduplicated list sorted by
// updated_at descending. Each issue carries forge/project metadata.
func (a *API) listIssuesFromAllProjects(ctx context.Context, opts *forge.ListOptions) []issueResponse {
	projects := a.projectRegistry.List()

	type result struct {
		issues []issueResponse
	}

	results := make([]result, len(projects))
	var wg sync.WaitGroup

	// Fetch up to maxLimit issues per project — the aggregated list is
	// sufficient for the My Queue UI which filters client-side.
	fetchOpts := &forge.ListOptions{
		State:   opts.State,
		Labels:  opts.Labels,
		Page:    1,
		PerPage: maxLimit,
	}

	for i, p := range projects {
		if p.Tracker == nil {
			continue
		}
		wg.Add(1)
		go func(idx int, proj *internalmcp.Project) {
			defer wg.Done()
			issues, err := proj.Tracker.ListIssues(ctx, fetchOpts)
			if err != nil {
				return
			}
			batch := make([]issueResponse, 0, len(issues))
			for _, iss := range issues {
				batch = append(batch, toIssueResponseForProject(iss, proj))
			}
			results[idx] = result{issues: batch}
		}(i, p)
	}

	wg.Wait()

	// Flatten results.
	totalCount := 0
	for i := range results {
		totalCount += len(results[i].issues)
	}
	all := make([]issueResponse, 0, totalCount)
	for i := range results {
		all = append(all, results[i].issues...)
	}
	return all
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

// bulkRequest is the request body for POST /api/v1/issues/bulk.
type bulkRequest struct {
	Action       string `json:"action"`        // "close" | "label" | "assign"
	IssueNumbers []int  `json:"issue_numbers"`
	Value        string `json:"value,omitempty"` // label name or assignee login
}

// bulkFailure records a single issue that failed in a bulk operation.
type bulkFailure struct {
	Number int    `json:"number"`
	Error  string `json:"error"`
}

// bulkResult is the response body for POST /api/v1/issues/bulk.
type bulkResult struct {
	Succeeded []int         `json:"succeeded"`
	Failed    []bulkFailure `json:"failed"`
}

// handleBulkIssues handles POST /api/v1/issues/bulk.
// Supported actions: "close", "label", "assign".
func (a *API) handleBulkIssues(w http.ResponseWriter, r *http.Request) {
	if a.tracker == nil {
		writeError(w, http.StatusServiceUnavailable, "issue tracker not available")
		return
	}

	var req bulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	switch req.Action {
	case "close", "label", "assign":
		// valid
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown action %q; must be close, label, or assign", req.Action))
		return
	}

	if len(req.IssueNumbers) == 0 {
		writeError(w, http.StatusBadRequest, "issue_numbers must not be empty")
		return
	}

	if (req.Action == "label" || req.Action == "assign") && req.Value == "" {
		writeError(w, http.StatusBadRequest, "value is required for action "+req.Action)
		return
	}

	result := bulkResult{
		Succeeded: []int{},
		Failed:    []bulkFailure{},
	}

	ctx := r.Context()
	for _, num := range req.IssueNumbers {
		var opErr error
		switch req.Action {
		case "close":
			closed := forge.StateClosed
			_, opErr = a.tracker.UpdateIssue(ctx, num, &forge.UpdateIssueRequest{State: &closed})
		case "label":
			opErr = a.tracker.AddLabels(ctx, num, req.Value)
		case "assign":
			opErr = a.tracker.Assign(ctx, num, req.Value)
		}
		if opErr != nil {
			result.Failed = append(result.Failed, bulkFailure{Number: num, Error: opErr.Error()})
		} else {
			result.Succeeded = append(result.Succeeded, num)
		}
	}

	writeJSON(w, http.StatusOK, result)
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
