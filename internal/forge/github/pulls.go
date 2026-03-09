package github

import (
	"context"
	"fmt"

	gogithub "github.com/google/go-github/v68/github"

	"github.com/herbhall/samverk/internal/forge"
)

// Compile-time interface check.
var _ forge.PullRequestManager = (*Client)(nil)

// CreatePullRequest creates a new pull request on the repository.
func (c *Client) CreatePullRequest(ctx context.Context, req *forge.CreatePRRequest) (*forge.PullRequest, error) {
	ghReq := &gogithub.NewPullRequest{
		Title: gogithub.Ptr(req.Title),
		Body:  gogithub.Ptr(req.Body),
		Head:  gogithub.Ptr(req.Head),
		Base:  gogithub.Ptr(req.Base),
	}

	pr, _, err := c.gh.PullRequests.Create(ctx, c.owner, c.repo, ghReq)
	if err != nil {
		return nil, fmt.Errorf("github: create pull request: %w", err)
	}

	return convertPR(pr), nil
}

// GetPullRequest retrieves a single pull request by number.
func (c *Client) GetPullRequest(ctx context.Context, number int) (*forge.PullRequest, error) {
	pr, _, err := c.gh.PullRequests.Get(ctx, c.owner, c.repo, number)
	if err != nil {
		return nil, fmt.Errorf("github: get pull request #%d: %w", number, err)
	}

	return convertPR(pr), nil
}

// ListPullRequests returns pull requests matching the given filter options.
func (c *Client) ListPullRequests(ctx context.Context, opts *forge.ListPROptions) ([]*forge.PullRequest, error) {
	ghOpts := &gogithub.PullRequestListOptions{
		ListOptions: gogithub.ListOptions{
			Page:    opts.Page,
			PerPage: opts.PerPage,
		},
	}
	if opts.State != "" {
		ghOpts.State = string(opts.State)
	}
	if opts.Head != "" {
		ghOpts.Head = opts.Head
	}
	if opts.Base != "" {
		ghOpts.Base = opts.Base
	}

	prs, _, err := c.gh.PullRequests.List(ctx, c.owner, c.repo, ghOpts)
	if err != nil {
		return nil, fmt.Errorf("github: list pull requests: %w", err)
	}

	result := make([]*forge.PullRequest, 0, len(prs))
	for i := range prs {
		result = append(result, convertPR(prs[i]))
	}

	return result, nil
}

// MergePullRequest merges a pull request using the specified method.
func (c *Client) MergePullRequest(ctx context.Context, number int, method forge.MergeMethod, commitMsg string) error {
	opts := &gogithub.PullRequestOptions{
		MergeMethod: string(method),
		CommitTitle: commitMsg,
	}

	_, _, err := c.gh.PullRequests.Merge(ctx, c.owner, c.repo, number, commitMsg, opts)
	if err != nil {
		return fmt.Errorf("github: merge pull request #%d: %w", number, err)
	}

	return nil
}

// GetPRChecks returns CI status for a pull request's head by querying both the
// legacy commit status API and the check runs API (used by GitHub Actions).
// Results from both sources are merged; check runs take precedence over
// commit statuses with the same name.
func (c *Client) GetPRChecks(ctx context.Context, number int) ([]forge.Check, error) {
	pr, _, err := c.gh.PullRequests.Get(ctx, c.owner, c.repo, number)
	if err != nil {
		return nil, fmt.Errorf("github: get PR #%d for checks: %w", number, err)
	}

	ref := pr.Head.GetSHA()
	if ref == "" {
		return nil, nil
	}

	// Collect checks by name; check runs overwrite legacy statuses.
	byName := make(map[string]forge.Check)

	// Legacy commit status API (external CI systems, older integrations).
	status, _, err := c.gh.Repositories.GetCombinedStatus(ctx, c.owner, c.repo, ref, nil)
	if err != nil {
		return nil, fmt.Errorf("github: get combined status for %s: %w", ref, err)
	}
	for _, s := range status.Statuses {
		byName[s.GetContext()] = forge.Check{
			Name:   s.GetContext(),
			Status: mapCommitStatus(s.GetState()),
		}
	}

	// Check runs API (GitHub Actions, GitHub Apps).
	runs, _, err := c.gh.Checks.ListCheckRunsForRef(ctx, c.owner, c.repo, ref, nil)
	if err != nil {
		return nil, fmt.Errorf("github: list check runs for %s: %w", ref, err)
	}
	for _, run := range runs.CheckRuns {
		byName[run.GetName()] = forge.Check{
			Name:   run.GetName(),
			Status: mapCheckRunStatus(run.GetStatus(), run.GetConclusion()),
		}
	}

	checks := make([]forge.Check, 0, len(byName))
	for _, ch := range byName {
		checks = append(checks, ch)
	}

	return checks, nil
}

// mapCommitStatus converts a legacy commit status state to forge.CheckStatus.
func mapCommitStatus(state string) forge.CheckStatus {
	switch state {
	case "success":
		return forge.CheckStatusSuccess
	case "failure", "error":
		return forge.CheckStatusFailure
	default:
		return forge.CheckStatusPending
	}
}

// mapCheckRunStatus converts GitHub check run status + conclusion to forge.CheckStatus.
// A check run with status "completed" uses its conclusion; otherwise it is pending.
func mapCheckRunStatus(status, conclusion string) forge.CheckStatus {
	if status != "completed" {
		return forge.CheckStatusPending
	}
	switch conclusion {
	case "success", "neutral", "skipped":
		return forge.CheckStatusSuccess
	case "failure", "cancelled", "timed_out", "action_required":
		return forge.CheckStatusFailure
	default:
		return forge.CheckStatusPending
	}
}

// ListReviewComments returns all review comments on a pull request.
func (c *Client) ListReviewComments(ctx context.Context, prNumber int) ([]forge.ReviewComment, error) {
	comments, _, err := c.gh.PullRequests.ListComments(ctx, c.owner, c.repo, prNumber, &gogithub.PullRequestListCommentsOptions{
		ListOptions: gogithub.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, fmt.Errorf("github: list review comments on PR #%d: %w", prNumber, err)
	}

	result := make([]forge.ReviewComment, 0, len(comments))
	for _, rc := range comments {
		comment := forge.ReviewComment{
			Body:      rc.GetBody(),
			Path:      rc.GetPath(),
			EndLine:   rc.GetLine(),
			CreatedAt: rc.GetCreatedAt().Time,
		}
		if rc.User != nil {
			comment.Author = rc.User.GetLogin()
		}
		if rc.StartLine != nil {
			comment.StartLine = rc.GetStartLine()
		} else {
			comment.StartLine = comment.EndLine
		}
		result = append(result, comment)
	}

	return result, nil
}

// convertPR transforms a GitHub SDK pull request into a forge.PullRequest.
func convertPR(gh *gogithub.PullRequest) *forge.PullRequest {
	pr := &forge.PullRequest{
		Number:    gh.GetNumber(),
		Title:     gh.GetTitle(),
		Body:      gh.GetBody(),
		State:     forge.State(gh.GetState()),
		Draft:     gh.GetDraft(),
		Mergeable: gh.GetMergeable(),
		CreatedAt: gh.GetCreatedAt().Time,
		UpdatedAt: gh.GetUpdatedAt().Time,
	}

	if gh.Head != nil {
		pr.Head = gh.Head.GetRef()
	}
	if gh.Base != nil {
		pr.Base = gh.Base.GetRef()
	}
	if gh.User != nil {
		pr.Author = gh.User.GetLogin()
	}

	labels := make([]string, 0, len(gh.Labels))
	for _, l := range gh.Labels {
		labels = append(labels, l.GetName())
	}
	pr.Labels = labels

	return pr
}
