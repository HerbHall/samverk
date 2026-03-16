// Package github implements the forge.IssueTracker interface for GitHub
// using the google/go-github SDK.
package github

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	gogithub "github.com/google/go-github/v68/github"

	"github.com/herbhall/samverk/internal/forge"
)

// Compile-time interface check.
var _ forge.IssueTracker = (*Client)(nil)

// Client implements forge.IssueTracker for GitHub repositories.
type Client struct {
	gh    *gogithub.Client
	owner string
	repo  string

	// polling state for Watch
	pollInterval time.Duration
}

// New creates a GitHub IssueTracker client for the given owner/repo.
// The httpClient should carry authentication (e.g., via oauth2.TokenSource).
// If httpClient is nil, unauthenticated requests are made (60 req/hr limit).
func New(owner, repo string, httpClient *http.Client) *Client {
	return &Client{
		gh:           gogithub.NewClient(httpClient),
		owner:        owner,
		repo:         repo,
		pollInterval: 30 * time.Second,
	}
}

// SetBaseURL overrides the GitHub API base URL (useful for GitHub Enterprise
// or test servers). The URL must include a trailing slash.
func (c *Client) SetBaseURL(url string) {
	c.gh.BaseURL.Host = ""
	c.gh.BaseURL.Path = ""

	parsed, err := c.gh.BaseURL.Parse(url)
	if err == nil {
		c.gh.BaseURL = parsed
	}
}

// SetPollInterval configures the polling interval for Watch.
func (c *Client) SetPollInterval(d time.Duration) {
	c.pollInterval = d
}

// CreateIssue creates a new issue on the repository.
func (c *Client) CreateIssue(ctx context.Context, req *forge.CreateIssueRequest) (*forge.Issue, error) {
	ghReq := &gogithub.IssueRequest{
		Title: gogithub.Ptr(req.Title),
		Body:  gogithub.Ptr(req.Body),
	}
	if len(req.Labels) > 0 {
		ghReq.Labels = &req.Labels
	}
	if len(req.Assignees) > 0 {
		ghReq.Assignees = &req.Assignees
	}

	issue, _, err := c.gh.Issues.Create(ctx, c.owner, c.repo, ghReq)
	if err != nil {
		return nil, fmt.Errorf("github: create issue: %w", err)
	}

	return convertIssue(issue), nil
}

// GetIssue retrieves a single issue by number.
func (c *Client) GetIssue(ctx context.Context, number int) (*forge.Issue, error) {
	issue, _, err := c.gh.Issues.Get(ctx, c.owner, c.repo, number)
	if err != nil {
		return nil, fmt.Errorf("github: get issue #%d: %w", number, err)
	}

	return convertIssue(issue), nil
}

// UpdateIssue modifies an existing issue. Nil fields in the request are left unchanged.
func (c *Client) UpdateIssue(ctx context.Context, number int, req *forge.UpdateIssueRequest) (*forge.Issue, error) {
	ghReq := &gogithub.IssueRequest{}
	if req.Title != nil {
		ghReq.Title = req.Title
	}
	if req.Body != nil {
		ghReq.Body = req.Body
	}
	if req.State != nil {
		s := string(*req.State)
		ghReq.State = &s
	}

	issue, _, err := c.gh.Issues.Edit(ctx, c.owner, c.repo, number, ghReq)
	if err != nil {
		return nil, fmt.Errorf("github: update issue #%d: %w", number, err)
	}

	return convertIssue(issue), nil
}

// ListIssues returns issues matching the given filter options.
func (c *Client) ListIssues(ctx context.Context, opts *forge.ListOptions) ([]*forge.Issue, error) {
	ghOpts := &gogithub.IssueListByRepoOptions{
		ListOptions: gogithub.ListOptions{
			Page:    opts.Page,
			PerPage: opts.PerPage,
		},
	}
	if opts.State != "" {
		ghOpts.State = string(opts.State)
	}
	if len(opts.Labels) > 0 {
		ghOpts.Labels = opts.Labels
	}
	if opts.Assignee != "" {
		ghOpts.Assignee = opts.Assignee
	}

	issues, _, err := c.gh.Issues.ListByRepo(ctx, c.owner, c.repo, ghOpts)
	if err != nil {
		return nil, fmt.Errorf("github: list issues: %w", err)
	}

	result := make([]*forge.Issue, 0, len(issues))
	for i := range issues {
		result = append(result, convertIssue(issues[i]))
	}

	return result, nil
}

// AddComment posts a new comment on the given issue.
func (c *Client) AddComment(ctx context.Context, number int, body string) (*forge.Comment, error) {
	comment, _, err := c.gh.Issues.CreateComment(ctx, c.owner, c.repo, number, &gogithub.IssueComment{
		Body: gogithub.Ptr(body),
	})
	if err != nil {
		return nil, fmt.Errorf("github: add comment to #%d: %w", number, err)
	}

	return convertComment(comment), nil
}

// ListComments returns all comments on the given issue.
func (c *Client) ListComments(ctx context.Context, number int) ([]*forge.Comment, error) {
	var result []*forge.Comment
	page := 1
	for {
		comments, _, err := c.gh.Issues.ListComments(ctx, c.owner, c.repo, number, &gogithub.IssueListCommentsOptions{
			ListOptions: gogithub.ListOptions{Page: page, PerPage: 100},
		})
		if err != nil {
			return nil, fmt.Errorf("github: list comments on #%d: %w", number, err)
		}
		for i := range comments {
			result = append(result, convertComment(comments[i]))
		}
		if len(comments) < 100 {
			break
		}
		page++
	}
	return result, nil
}

// SetLabels replaces all labels on the given issue.
func (c *Client) SetLabels(ctx context.Context, number int, labels []string) error {
	_, _, err := c.gh.Issues.ReplaceLabelsForIssue(ctx, c.owner, c.repo, number, labels)
	if err != nil {
		return fmt.Errorf("github: set labels on #%d: %w", number, err)
	}

	return nil
}

// AddLabel adds a single label to the given issue.
func (c *Client) AddLabel(ctx context.Context, number int, label string) error {
	_, _, err := c.gh.Issues.AddLabelsToIssue(ctx, c.owner, c.repo, number, []string{label})
	if err != nil {
		return fmt.Errorf("github: add label %q to #%d: %w", label, number, err)
	}

	return nil
}

// RemoveLabel removes a single label from the given issue.
// A 404 response is treated as a no-op: the label was already absent,
// which is the desired end state.
func (c *Client) RemoveLabel(ctx context.Context, number int, label string) error {
	resp, err := c.gh.Issues.RemoveLabelForIssue(ctx, c.owner, c.repo, number, label)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("github: remove label %q from #%d: %w", label, number, err)
	}

	return nil
}

// Assign adds an assignee to the given issue.
func (c *Client) Assign(ctx context.Context, number int, assignee string) error {
	_, _, err := c.gh.Issues.AddAssignees(ctx, c.owner, c.repo, number, []string{assignee})
	if err != nil {
		return fmt.Errorf("github: assign %q to #%d: %w", assignee, number, err)
	}

	return nil
}

// Unassign removes an assignee from the given issue.
func (c *Client) Unassign(ctx context.Context, number int, assignee string) error {
	_, _, err := c.gh.Issues.RemoveAssignees(ctx, c.owner, c.repo, number, []string{assignee})
	if err != nil {
		return fmt.Errorf("github: unassign %q from #%d: %w", assignee, number, err)
	}

	return nil
}

// listAllOpenIssues fetches all open issues, paginating automatically.
func (c *Client) listAllOpenIssues(ctx context.Context) ([]*forge.Issue, error) {
	var all []*forge.Issue
	page := 1
	for {
		batch, err := c.ListIssues(ctx, &forge.ListOptions{
			State:   forge.StateOpen,
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			break
		}
		page++
	}
	return all, nil
}

// Watch polls the GitHub API for issue changes and calls handler for each detected event.
// It blocks until the context is cancelled. The initial poll establishes baseline state;
// subsequent polls emit events for changes.
func (c *Client) Watch(ctx context.Context, handler func(forge.Event)) error {
	// Wrap handler to inject owner/repo into every emitted event.
	wrappedHandler := func(ev forge.Event) {
		ev.Owner = c.owner
		ev.Repo = c.repo
		handler(ev)
	}

	known := make(map[int]*forge.Issue)
	var mu sync.Mutex

	// Initial load.
	issues, err := c.listAllOpenIssues(ctx)
	if err != nil {
		return fmt.Errorf("github: watch initial load: %w", err)
	}

	mu.Lock()
	for _, iss := range issues {
		known[iss.Number] = iss
	}
	// Emit events for pre-existing queued issues so the dispatcher
	// routes them on startup, not only when new issues appear.
	for _, iss := range issues {
		for _, label := range iss.Labels {
			if label == "status:queued" {
				wrappedHandler(forge.Event{
					Type:          forge.EventIssueOpened,
					IssueNumber:   iss.Number,
					Issue:         iss,
					IsPullRequest: iss.IsPullRequest,
				})
				break
			}
		}
	}
	mu.Unlock()

	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			current, err := c.listAllOpenIssues(ctx)
			if err != nil {
				continue // transient errors are retried on next tick
			}

			mu.Lock()
			c.diffAndEmit(known, current, wrappedHandler)
			mu.Unlock()
		}
	}
}

// diffAndEmit compares known state with current issues and emits events.
// Caller must hold mu.
func (c *Client) diffAndEmit(known map[int]*forge.Issue, current []*forge.Issue, handler func(forge.Event)) {
	seen := make(map[int]bool, len(current))

	for _, iss := range current {
		seen[iss.Number] = true
		prev, exists := known[iss.Number]

		if !exists {
			handler(forge.Event{
				Type:          forge.EventIssueOpened,
				IssueNumber:   iss.Number,
				Issue:         iss,
				IsPullRequest: iss.IsPullRequest,
			})
			known[iss.Number] = iss
			continue
		}

		// Detect label changes.
		if !stringSliceEqual(prev.Labels, iss.Labels) {
			handler(forge.Event{
				Type:        forge.EventIssueLabeled,
				IssueNumber: iss.Number,
				Issue:       iss,
			})
		}

		// Detect assignee changes.
		if !stringSliceEqual(prev.Assignees, iss.Assignees) {
			handler(forge.Event{
				Type:        forge.EventIssueAssigned,
				IssueNumber: iss.Number,
				Issue:       iss,
			})
		}

		// Detect edits (title or body changed).
		if prev.Title != iss.Title || prev.Body != iss.Body {
			handler(forge.Event{
				Type:        forge.EventIssueEdited,
				IssueNumber: iss.Number,
				Issue:       iss,
			})
		}

		known[iss.Number] = iss
	}

	// Detect closed issues (present in known but not in current open list).
	for num, iss := range known {
		if !seen[num] {
			handler(forge.Event{
				Type:        forge.EventIssueClosed,
				IssueNumber: num,
				Issue:       iss,
			})
			delete(known, num)
		}
	}
}

// convertIssue transforms a GitHub SDK issue into a forge.Issue.
func convertIssue(gh *gogithub.Issue) *forge.Issue {
	issue := &forge.Issue{
		Number:        gh.GetNumber(),
		Title:         gh.GetTitle(),
		Body:          gh.GetBody(),
		State:         forge.State(gh.GetState()),
		IsPullRequest: gh.PullRequestLinks != nil,
		CreatedAt:     gh.GetCreatedAt().Time,
		UpdatedAt:     gh.GetUpdatedAt().Time,
	}

	if gh.ClosedAt != nil {
		t := gh.GetClosedAt().Time
		issue.ClosedAt = &t
	}

	issue.Labels = make([]string, 0, len(gh.Labels))
	for i := range gh.Labels {
		issue.Labels = append(issue.Labels, gh.Labels[i].GetName())
	}

	issue.Assignees = make([]string, 0, len(gh.Assignees))
	for i := range gh.Assignees {
		issue.Assignees = append(issue.Assignees, gh.Assignees[i].GetLogin())
	}

	return issue
}

// convertComment transforms a GitHub SDK comment into a forge.Comment.
func convertComment(gh *gogithub.IssueComment) *forge.Comment {
	return &forge.Comment{
		ID:        gh.GetID(),
		Body:      gh.GetBody(),
		Author:    gh.GetUser().GetLogin(),
		CreatedAt: gh.GetCreatedAt().Time,
	}
}

// stringSliceEqual reports whether two string slices have the same elements.
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
