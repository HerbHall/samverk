// Package gitea implements the forge.IssueTracker interface for Gitea
// using the official Gitea Go SDK.
package gitea

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	gogitea "code.gitea.io/sdk/gitea"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// Compile-time interface checks.
var (
	_ forge.IssueTracker      = (*Client)(nil)
	_ forge.RepoWriter        = (*Client)(nil)
	_ forge.PullRequestManager = (*Client)(nil)
)

// Client implements forge.IssueTracker for Gitea repositories.
type Client struct {
	gt      *gogitea.Client
	baseURL string // Gitea instance base URL for raw API calls
	token   string // API token for raw HTTP requests
	owner   string
	repo    string

	// polling state for Watch
	pollInterval time.Duration

	// label name-to-ID cache (populated on first label operation)
	labelOnce  sync.Once
	labelCache map[string]int64
	labelErr   error
}

// New creates a Gitea IssueTracker client for the given owner/repo.
// The url should be the Gitea instance base URL (e.g., "http://localhost:3000").
// Token authenticates via Gitea's "token <token>" header format.
func New(url, token, owner, repo string) (*Client, error) {
	gt, err := gogitea.NewClient(url, gogitea.SetToken(token))
	if err != nil {
		return nil, fmt.Errorf("gitea: create client: %w", err)
	}

	return &Client{
		gt:           gt,
		baseURL:      strings.TrimRight(url, "/"),
		token:        token,
		owner:        owner,
		repo:         repo,
		pollInterval: 30 * time.Second,
	}, nil
}

// SetPollInterval configures the polling interval for Watch.
func (c *Client) SetPollInterval(d time.Duration) {
	c.pollInterval = d
}

// CreateIssue creates a new issue on the repository.
func (c *Client) CreateIssue(ctx context.Context, req *forge.CreateIssueRequest) (issue *forge.Issue, err error) {
	opt := gogitea.CreateIssueOption{
		Title:     req.Title,
		Body:      req.Body,
		Assignees: req.Assignees,
	}

	if len(req.Labels) > 0 {
		ids, labelErr := c.resolveLabels(req.Labels)
		if labelErr != nil {
			return nil, fmt.Errorf("gitea: create issue: %w", labelErr)
		}
		opt.Labels = ids
	}

	gi, _, err := c.gt.CreateIssue(c.owner, c.repo, opt)
	if err != nil {
		return nil, fmt.Errorf("gitea: create issue: %w", err)
	}

	return convertIssue(gi), nil
}

// GetIssue retrieves a single issue by number.
func (c *Client) GetIssue(ctx context.Context, number int) (issue *forge.Issue, err error) {
	gi, _, err := c.gt.GetIssue(c.owner, c.repo, int64(number))
	if err != nil {
		return nil, fmt.Errorf("gitea: get issue #%d: %w", number, err)
	}

	return convertIssue(gi), nil
}

// UpdateIssue modifies an existing issue. Nil fields in the request are left unchanged.
func (c *Client) UpdateIssue(ctx context.Context, number int, req *forge.UpdateIssueRequest) (issue *forge.Issue, err error) {
	opt := gogitea.EditIssueOption{}
	if req.Title != nil {
		opt.Title = *req.Title
	}
	if req.Body != nil {
		opt.Body = req.Body
	}
	if req.State != nil {
		st := gogitea.StateType(*req.State)
		opt.State = &st
	}

	gi, _, err := c.gt.EditIssue(c.owner, c.repo, int64(number), opt)
	if err != nil {
		return nil, fmt.Errorf("gitea: update issue #%d: %w", number, err)
	}

	return convertIssue(gi), nil
}

// ListIssues returns issues matching the given filter options.
func (c *Client) ListIssues(ctx context.Context, opts *forge.ListOptions) (issues []*forge.Issue, err error) {
	pageSize := opts.PerPage
	if pageSize == 0 || pageSize > 50 {
		pageSize = 50
	}

	gOpts := gogitea.ListIssueOption{
		ListOptions: gogitea.ListOptions{
			Page:     opts.Page,
			PageSize: pageSize,
		},
		Type: gogitea.IssueTypeIssue,
	}
	if opts.State != "" {
		gOpts.State = gogitea.StateType(opts.State)
	}
	if len(opts.Labels) > 0 {
		gOpts.Labels = opts.Labels
	}
	if opts.Assignee != "" {
		gOpts.AssignedBy = opts.Assignee
	}

	gi, _, err := c.gt.ListRepoIssues(c.owner, c.repo, gOpts)
	if err != nil {
		return nil, fmt.Errorf("gitea: list issues: %w", err)
	}

	result := make([]*forge.Issue, 0, len(gi))
	for i := range gi {
		result = append(result, convertIssue(gi[i]))
	}

	return result, nil
}

// SearchIssues performs full-text search across issue titles and bodies
// using Gitea's keyword search parameter.
func (c *Client) SearchIssues(ctx context.Context, opts *forge.SearchOptions) (issues []*forge.Issue, err error) {
	issueType := gogitea.IssueTypeIssue
	if opts.IsPullRequest != nil && *opts.IsPullRequest {
		issueType = gogitea.IssueTypePull
	}

	gOpts := gogitea.ListIssueOption{
		ListOptions: gogitea.ListOptions{PageSize: 50},
		KeyWord:     opts.Query,
		Type:        issueType,
	}
	if opts.State != "" {
		gOpts.State = gogitea.StateType(opts.State)
	}
	if len(opts.Labels) > 0 {
		gOpts.Labels = opts.Labels
	}

	gi, _, err := c.gt.ListRepoIssues(c.owner, c.repo, gOpts)
	if err != nil {
		return nil, fmt.Errorf("gitea: search issues %q: %w", opts.Query, err)
	}

	result := make([]*forge.Issue, 0, len(gi))
	for i := range gi {
		result = append(result, convertIssue(gi[i]))
	}

	return result, nil
}

// AddComment posts a new comment on the given issue.
func (c *Client) AddComment(ctx context.Context, number int, body string) (comment *forge.Comment, err error) {
	gc, _, err := c.gt.CreateIssueComment(c.owner, c.repo, int64(number), gogitea.CreateIssueCommentOption{
		Body: body,
	})
	if err != nil {
		return nil, fmt.Errorf("gitea: add comment to #%d: %w", number, err)
	}

	return convertComment(gc), nil
}

// ListComments returns all comments on the given issue.
func (c *Client) ListComments(ctx context.Context, number int) ([]*forge.Comment, error) {
	var result []*forge.Comment
	page := 1
	for {
		gc, _, err := c.gt.ListIssueComments(c.owner, c.repo, int64(number), gogitea.ListIssueCommentOptions{
			ListOptions: gogitea.ListOptions{Page: page, PageSize: 50},
		})
		if err != nil {
			return nil, fmt.Errorf("gitea: list comments on #%d: %w", number, err)
		}
		for i := range gc {
			result = append(result, convertComment(gc[i]))
		}
		if len(gc) < 50 {
			break
		}
		page++
	}
	return result, nil
}

// SetLabels replaces all labels on the given issue.
func (c *Client) SetLabels(ctx context.Context, number int, labels []string) error {
	ids, err := c.resolveLabels(labels)
	if err != nil {
		return fmt.Errorf("gitea: set labels on #%d: %w", number, err)
	}

	_, _, err = c.gt.ReplaceIssueLabels(c.owner, c.repo, int64(number), gogitea.IssueLabelsOption{
		Labels: ids,
	})
	if err != nil {
		return fmt.Errorf("gitea: set labels on #%d: %w", number, err)
	}

	return nil
}

// AddLabel adds a single label to the given issue.
func (c *Client) AddLabel(ctx context.Context, number int, label string) error {
	id, err := c.resolveLabelID(label)
	if err != nil {
		return fmt.Errorf("gitea: add label %q to #%d: %w", label, number, err)
	}

	_, _, err = c.gt.AddIssueLabels(c.owner, c.repo, int64(number), gogitea.IssueLabelsOption{
		Labels: []int64{id},
	})
	if err != nil {
		return fmt.Errorf("gitea: add label %q to #%d: %w", label, number, err)
	}

	return nil
}

// RemoveLabel removes a single label from the given issue.
func (c *Client) RemoveLabel(ctx context.Context, number int, label string) error {
	id, err := c.resolveLabelID(label)
	if err != nil {
		return fmt.Errorf("gitea: remove label %q from #%d: %w", label, number, err)
	}

	_, err = c.gt.DeleteIssueLabel(c.owner, c.repo, int64(number), id)
	if err != nil {
		return fmt.Errorf("gitea: remove label %q from #%d: %w", label, number, err)
	}

	return nil
}

// Assign adds an assignee to the given issue via read-modify-write.
// Gitea has no dedicated assign endpoint; we read the current assignees,
// append the new one, and write back via EditIssue.
func (c *Client) Assign(ctx context.Context, number int, assignee string) error {
	gi, _, err := c.gt.GetIssue(c.owner, c.repo, int64(number))
	if err != nil {
		return fmt.Errorf("gitea: assign %q to #%d: get issue: %w", assignee, number, err)
	}

	assignees := extractAssigneeLogins(gi.Assignees)

	// Check if already assigned.
	for _, a := range assignees {
		if a == assignee {
			return nil
		}
	}

	assignees = append(assignees, assignee)
	_, _, err = c.gt.EditIssue(c.owner, c.repo, int64(number), gogitea.EditIssueOption{
		Assignees: assignees,
	})
	if err != nil {
		return fmt.Errorf("gitea: assign %q to #%d: %w", assignee, number, err)
	}

	return nil
}

// Unassign removes an assignee from the given issue via read-modify-write.
// Gitea has no dedicated unassign endpoint; we read the current assignees,
// remove the target, and write back via EditIssue.
func (c *Client) Unassign(ctx context.Context, number int, assignee string) error {
	gi, _, err := c.gt.GetIssue(c.owner, c.repo, int64(number))
	if err != nil {
		return fmt.Errorf("gitea: unassign %q from #%d: get issue: %w", assignee, number, err)
	}

	current := extractAssigneeLogins(gi.Assignees)
	updated := make([]string, 0, len(current))
	for _, a := range current {
		if a != assignee {
			updated = append(updated, a)
		}
	}

	_, _, err = c.gt.EditIssue(c.owner, c.repo, int64(number), gogitea.EditIssueOption{
		Assignees: updated,
	})
	if err != nil {
		return fmt.Errorf("gitea: unassign %q from #%d: %w", assignee, number, err)
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
			PerPage: 50,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 50 {
			break
		}
		page++
	}
	return all, nil
}

// Watch polls the Gitea API for issue changes and calls handler for each detected event.
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
		return fmt.Errorf("gitea: watch initial load: %w", err)
	}

	mu.Lock()
	for _, iss := range issues {
		known[iss.Number] = iss
	}
	// Emit events for pre-existing queued issues so the dispatcher
	// routes them on startup, not only when new issues appear.
	for _, iss := range issues {
		for _, label := range iss.Labels {
			if label == models.LabelStatusQueued {
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
			current, pollErr := c.listAllOpenIssues(ctx)
			if pollErr != nil {
				continue // transient errors are retried on next tick
			}

			mu.Lock()
			diffAndEmit(known, current, wrappedHandler)
			mu.Unlock()
		}
	}
}

// diffAndEmit compares known state with current issues and emits events.
// Caller must hold mu.
func diffAndEmit(known map[int]*forge.Issue, current []*forge.Issue, handler func(forge.Event)) {
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

func (c *Client) ensureLabelCache() error {
	c.labelOnce.Do(func() {
		c.labelCache = make(map[string]int64)
		page := 1
		for {
			labels, _, err := c.gt.ListRepoLabels(c.owner, c.repo, gogitea.ListLabelsOptions{
				ListOptions: gogitea.ListOptions{Page: page, PageSize: 50},
			})
			if err != nil {
				c.labelErr = fmt.Errorf("fetch repo labels: %w", err)
				return
			}
			for i := range labels {
				c.labelCache[labels[i].Name] = labels[i].ID
			}
			if len(labels) < 50 {
				break
			}
			page++
		}
	})

	return c.labelErr
}

// resolveLabels converts label names to Gitea integer IDs.
func (c *Client) resolveLabels(names []string) (ids []int64, err error) {
	if err := c.ensureLabelCache(); err != nil {
		return nil, err
	}

	ids = make([]int64, 0, len(names))
	for _, name := range names {
		id, ok := c.labelCache[name]
		if !ok {
			return nil, fmt.Errorf("unknown label %q", name)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// resolveLabelID converts a single label name to its Gitea integer ID.
func (c *Client) resolveLabelID(name string) (int64, error) {
	if err := c.ensureLabelCache(); err != nil {
		return 0, err
	}

	id, ok := c.labelCache[name]
	if !ok {
		return 0, fmt.Errorf("unknown label %q", name)
	}

	return id, nil
}

// convertIssue transforms a Gitea SDK issue into a forge.Issue.
func convertIssue(gi *gogitea.Issue) *forge.Issue {
	issue := &forge.Issue{
		Number:        int(gi.Index),
		Title:         gi.Title,
		Body:          gi.Body,
		State:         forge.State(gi.State),
		IsPullRequest: gi.PullRequest != nil,
		CreatedAt:     gi.Created,
		UpdatedAt:     gi.Updated,
		ClosedAt:      gi.Closed,
	}

	issue.Labels = make([]string, 0, len(gi.Labels))
	for i := range gi.Labels {
		issue.Labels = append(issue.Labels, gi.Labels[i].Name)
	}

	issue.Assignees = extractAssigneeLogins(gi.Assignees)

	return issue
}

// convertComment transforms a Gitea SDK comment into a forge.Comment.
func convertComment(gc *gogitea.Comment) *forge.Comment {
	author := ""
	if gc.Poster != nil {
		author = gc.Poster.UserName
	}

	return &forge.Comment{
		ID:        gc.ID,
		Body:      gc.Body,
		Author:    author,
		CreatedAt: gc.Created,
	}
}

// extractAssigneeLogins extracts usernames from a slice of Gitea User pointers.
func extractAssigneeLogins(users []*gogitea.User) []string {
	result := make([]string, 0, len(users))
	for i := range users {
		result = append(result, users[i].UserName)
	}
	return result
}

// errNotImplemented is an internal alias for forge.ErrNotSupported used by
// Gitea methods that have no equivalent in Gitea SDK v0.23.2 (e.g. SearchCode).
var errNotImplemented = forge.ErrNotSupported

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
