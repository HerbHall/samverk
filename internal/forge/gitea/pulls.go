package gitea

import (
	"context"
	"fmt"

	gogitea "code.gitea.io/sdk/gitea"

	"github.com/herbhall/samverk/internal/forge"
)

// convertPR transforms a Gitea SDK pull request into a forge.PullRequest.
func convertPR(gpr *gogitea.PullRequest) *forge.PullRequest {
	pr := &forge.PullRequest{
		Number:    int(gpr.Index),
		Title:     gpr.Title,
		Body:      gpr.Body,
		State:     forge.State(gpr.State),
		Mergeable: gpr.Mergeable,
		Draft:     gpr.Draft,
	}
	if gpr.Head != nil {
		pr.Head = gpr.Head.Ref
	}
	if gpr.Base != nil {
		pr.Base = gpr.Base.Ref
	}
	if gpr.Poster != nil {
		pr.Author = gpr.Poster.UserName
	}
	pr.Labels = make([]string, 0, len(gpr.Labels))
	for i := range gpr.Labels {
		pr.Labels = append(pr.Labels, gpr.Labels[i].Name)
	}
	if gpr.Created != nil {
		pr.CreatedAt = *gpr.Created
	}
	if gpr.Updated != nil {
		pr.UpdatedAt = *gpr.Updated
	}
	return pr
}

// mapCheckStatus maps a Gitea StatusState to a forge CheckStatus.
func mapCheckStatus(s gogitea.StatusState) forge.CheckStatus {
	switch s {
	case gogitea.StatusSuccess:
		return forge.CheckStatusSuccess
	case gogitea.StatusFailure, gogitea.StatusError:
		return forge.CheckStatusFailure
	default:
		return forge.CheckStatusPending
	}
}

// CreatePullRequest creates a new pull request on the repository.
func (c *Client) CreatePullRequest(ctx context.Context, req *forge.CreatePRRequest) (*forge.PullRequest, error) {
	gpr, _, err := c.gt.CreatePullRequest(c.owner, c.repo, gogitea.CreatePullRequestOption{
		Title: req.Title,
		Body:  req.Body,
		Head:  req.Head,
		Base:  req.Base,
	})
	if err != nil {
		return nil, fmt.Errorf("gitea: create pull request: %w", err)
	}

	return convertPR(gpr), nil
}

// GetPullRequest retrieves a single pull request by number.
func (c *Client) GetPullRequest(ctx context.Context, number int) (*forge.PullRequest, error) {
	gpr, _, err := c.gt.GetPullRequest(c.owner, c.repo, int64(number))
	if err != nil {
		return nil, fmt.Errorf("gitea: get pull request #%d: %w", number, err)
	}

	return convertPR(gpr), nil
}

// ListPullRequests returns pull requests matching the given filter options.
func (c *Client) ListPullRequests(ctx context.Context, opts *forge.ListPROptions) ([]*forge.PullRequest, error) {
	pageSize := opts.PerPage
	if pageSize == 0 || pageSize > 50 {
		pageSize = 50
	}

	gOpts := gogitea.ListPullRequestsOptions{
		ListOptions: gogitea.ListOptions{
			Page:     opts.Page,
			PageSize: pageSize,
		},
	}
	if opts.State != "" {
		gOpts.State = gogitea.StateType(opts.State)
	}

	gprs, _, err := c.gt.ListRepoPullRequests(c.owner, c.repo, gOpts)
	if err != nil {
		return nil, fmt.Errorf("gitea: list pull requests: %w", err)
	}

	result := make([]*forge.PullRequest, 0, len(gprs))
	for i := range gprs {
		result = append(result, convertPR(gprs[i]))
	}

	return result, nil
}

// MergePullRequest merges a pull request using the specified method.
func (c *Client) MergePullRequest(ctx context.Context, number int, method forge.MergeMethod, commitMsg string) error {
	var style gogitea.MergeStyle
	switch method {
	case forge.MergeMethodSquash:
		style = gogitea.MergeStyleSquash
	case forge.MergeMethodRebase:
		style = gogitea.MergeStyleRebase
	default:
		style = gogitea.MergeStyleMerge
	}

	_, _, err := c.gt.MergePullRequest(c.owner, c.repo, int64(number), gogitea.MergePullRequestOption{
		Style:   style,
		Title:   commitMsg,
		Message: commitMsg,
	})
	if err != nil {
		return fmt.Errorf("gitea: merge pull request #%d: %w", number, err)
	}

	return nil
}

// GetPRChecks returns the CI status checks for a pull request's head commit.
func (c *Client) GetPRChecks(ctx context.Context, number int) ([]forge.Check, error) {
	gpr, _, err := c.gt.GetPullRequest(c.owner, c.repo, int64(number))
	if err != nil {
		return nil, fmt.Errorf("gitea: get PR #%d for checks: %w", number, err)
	}

	if gpr.Head == nil || gpr.Head.Sha == "" {
		return nil, nil
	}

	headSHA := gpr.Head.Sha

	combined, _, err := c.gt.GetCombinedStatus(c.owner, c.repo, headSHA)
	if err != nil {
		return nil, fmt.Errorf("gitea: get combined status for %s: %w", headSHA, err)
	}

	if len(combined.Statuses) == 0 {
		return []forge.Check{
			{Name: "combined", Status: mapCheckStatus(combined.State)},
		}, nil
	}

	checks := make([]forge.Check, 0, len(combined.Statuses))
	for _, s := range combined.Statuses {
		checks = append(checks, forge.Check{
			Name:   s.Context,
			Status: mapCheckStatus(s.State),
		})
	}

	return checks, nil
}

// ListReviewComments returns all inline review comments on a pull request.
func (c *Client) ListReviewComments(ctx context.Context, prNumber int) ([]forge.ReviewComment, error) {
	reviews, _, err := c.gt.ListPullReviews(c.owner, c.repo, int64(prNumber), gogitea.ListPullReviewsOptions{
		ListOptions: gogitea.ListOptions{PageSize: 50},
	})
	if err != nil {
		return nil, fmt.Errorf("gitea: list pull reviews on PR #%d: %w", prNumber, err)
	}

	var result []forge.ReviewComment

	for _, review := range reviews {
		rcs, _, err := c.gt.ListPullReviewComments(c.owner, c.repo, int64(prNumber), review.ID)
		if err != nil {
			return nil, fmt.Errorf("gitea: list review comments for review %d on PR #%d: %w", review.ID, prNumber, err)
		}

		for _, rc := range rcs {
			if rc.Body == "" {
				continue
			}

			author := ""
			if rc.Reviewer != nil {
				author = rc.Reviewer.UserName
			}

			startLine := int(rc.OldLineNum)
			if startLine == 0 {
				startLine = int(rc.LineNum)
			}

			result = append(result, forge.ReviewComment{
				Author:    author,
				Body:      rc.Body,
				Path:      rc.Path,
				StartLine: startLine,
				EndLine:   int(rc.LineNum),
				Resolved:  rc.Resolver != nil,
				CreatedAt: rc.Created,
			})
		}
	}

	if result == nil {
		result = []forge.ReviewComment{}
	}

	return result, nil
}
