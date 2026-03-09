package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/digest"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/prwatcher"
)

// reviewPRInput is the typed input for the review_pr tool.
type reviewPRInput struct {
	Number int `json:"number" jsonschema:"required,the pull request number to review"`
}

// listOpenPRsInput is the typed input for the list_open_prs tool.
type listOpenPRsInput struct {
	// No required fields — lists all open PRs across all projects.
}

// bulkMergeInput is the typed input for the bulk_merge tool.
type bulkMergeInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"preview which PRs would be merged without merging"`
}

// registerPRReviewTools adds PR review and merge workflow tools to the server.
func registerPRReviewTools(srv *gosdk.Server, h *Handler) {
	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "review_pr",
		Description: "Review a PR: show diff stats, CI status, tier assessment, and merge eligibility",
	}, h.handleReviewPR)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "list_open_prs",
		Description: "List open PRs across all registered projects with CI status and tier",
	}, h.handleListOpenPRs)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "bulk_merge",
		Description: "Merge all Tier 1 PRs with green CI across all projects. Use dry_run to preview.",
	}, h.handleBulkMerge)
}

// prReviewResult is the JSON structure returned by review_pr.
type prReviewResult struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Author    string   `json:"author"`
	Head      string   `json:"head"`
	Base      string   `json:"base"`
	Mergeable bool     `json:"mergeable"`
	Draft     bool     `json:"draft"`
	Labels    []string `json:"labels"`
	Tier      string   `json:"tier"`
	TierDesc  string   `json:"tier_description"`
	CIStatus  string   `json:"ci_status"`
	Checks    []string `json:"checks,omitempty"`
	Eligible  bool     `json:"eligible"`
	Reason    string   `json:"reason,omitempty"`
}

// handleReviewPR retrieves PR details, CI checks, and tier classification.
func (h *Handler) handleReviewPR(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input reviewPRInput,
) (*gosdk.CallToolResult, any, error) {
	prm := h.activePRManager()
	if prm == nil {
		return nil, nil, fmt.Errorf("pull request operations are not available")
	}
	if input.Number <= 0 {
		return nil, nil, fmt.Errorf("number must be greater than 0")
	}

	pr, err := prm.GetPullRequest(ctx, input.Number)
	if err != nil {
		return nil, nil, fmt.Errorf("getting pull request #%d: %w", input.Number, err)
	}
	if pr == nil {
		return nil, nil, fmt.Errorf("pull request #%d not found", input.Number)
	}

	// Get CI checks.
	checks, err := prm.GetPRChecks(ctx, input.Number)
	if err != nil {
		return nil, nil, fmt.Errorf("getting checks for PR #%d: %w", input.Number, err)
	}

	ciStatus, checkNames := summarizeChecks(checks)

	// Classify tier (using PR labels + any linked issue labels we can find).
	tier := prwatcher.ClassifyPRTier(pr, nil)

	// Determine merge eligibility.
	eligible, reason := assessEligibility(pr, ciStatus, tier)

	result := prReviewResult{
		Number:    pr.Number,
		Title:     pr.Title,
		Author:    pr.Author,
		Head:      pr.Head,
		Base:      pr.Base,
		Mergeable: pr.Mergeable,
		Draft:     pr.Draft,
		Labels:    pr.Labels,
		Tier:      tier.String(),
		TierDesc:  tier.Description(),
		CIStatus:  ciStatus,
		Checks:    checkNames,
		Eligible:  eligible,
		Reason:    reason,
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling review result: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(data)},
		},
	}, nil, nil
}

// summarizeChecks returns an overall status string and per-check names.
func summarizeChecks(checks []forge.Check) (status string, names []string) {
	if len(checks) == 0 {
		return "no-ci", nil
	}

	names = make([]string, 0, len(checks))
	allPassed := true
	hasPending := false

	for _, c := range checks {
		label := fmt.Sprintf("%s: %s", c.Name, c.Status)
		names = append(names, label)
		if c.Status != forge.CheckStatusSuccess {
			allPassed = false
		}
		if c.Status == forge.CheckStatusPending {
			hasPending = true
		}
	}

	switch {
	case allPassed:
		return "passed", names
	case hasPending:
		return "pending", names
	default:
		return "failed", names
	}
}

// assessEligibility determines if a PR can be merged and why/why not.
func assessEligibility(pr *forge.PullRequest, ciStatus string, tier prwatcher.PRTier) (eligible bool, reason string) {
	if pr.Draft {
		return false, "PR is a draft"
	}
	if !pr.Mergeable {
		return false, "PR has merge conflicts"
	}
	if ciStatus == "failed" {
		return false, "CI checks failed"
	}
	if ciStatus == "pending" {
		return false, "CI checks still running"
	}
	if tier == prwatcher.PRTier3 {
		return false, "Tier 3: requires human review"
	}
	if ciStatus == "no-ci" && tier != prwatcher.PRTier1 {
		return false, "no CI configured and not Tier 1"
	}
	return true, ""
}

// projectPR is a single PR entry in the list_open_prs response.
type projectPR struct {
	Project  string `json:"project"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	Author   string `json:"author"`
	Tier     string `json:"tier"`
	TierDesc string `json:"tier_description"`
	CIStatus string `json:"ci_status"`
	Age      string `json:"age"`
}

// handleListOpenPRs aggregates open PRs across all registered projects.
func (h *Handler) handleListOpenPRs(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	_ listOpenPRsInput,
) (*gosdk.CallToolResult, any, error) {
	var results []projectPR

	if h.projects != nil {
		for _, proj := range h.projects.List() {
			if proj.PRManager == nil {
				continue
			}
			prs, err := proj.PRManager.ListPullRequests(ctx, &forge.ListPROptions{
				State:   forge.StateOpen,
				PerPage: 50,
			})
			if err != nil {
				// Log but continue with other projects.
				results = append(results, projectPR{
					Project: proj.Name,
					Title:   fmt.Sprintf("error listing PRs: %v", err),
				})
				continue
			}

			for _, pr := range prs {
				tier := prwatcher.ClassifyPRTier(pr, nil)

				checks, _ := proj.PRManager.GetPRChecks(ctx, pr.Number)
				ciStatus, _ := summarizeChecks(checks)

				results = append(results, projectPR{
					Project:  proj.Name,
					Number:   pr.Number,
					Title:    pr.Title,
					Author:   pr.Author,
					Tier:     tier.String(),
					TierDesc: tier.Description(),
					CIStatus: ciStatus,
					Age:      formatAge(pr.CreatedAt),
				})
			}
		}
	}

	// Also include the default/active project if no registry or registry is empty.
	if len(results) == 0 {
		prm := h.activePRManager()
		if prm != nil {
			prs, err := prm.ListPullRequests(ctx, &forge.ListPROptions{
				State:   forge.StateOpen,
				PerPage: 50,
			})
			if err != nil {
				return nil, nil, fmt.Errorf("listing open PRs: %w", err)
			}
			for _, pr := range prs {
				tier := prwatcher.ClassifyPRTier(pr, nil)
				checks, _ := prm.GetPRChecks(ctx, pr.Number)
				ciStatus, _ := summarizeChecks(checks)
				results = append(results, projectPR{
					Project:  "default",
					Number:   pr.Number,
					Title:    pr.Title,
					Author:   pr.Author,
					Tier:     tier.String(),
					TierDesc: tier.Description(),
					CIStatus: ciStatus,
					Age:      formatAge(pr.CreatedAt),
				})
			}
		}
	}

	data, err := json.Marshal(results)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling PR list: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(data)},
		},
	}, nil, nil
}

// bulkMergeResult is the response from bulk_merge.
type bulkMergeResult struct {
	Merged  []string `json:"merged,omitempty"`
	Skipped []string `json:"skipped,omitempty"`
	Errors  []string `json:"errors,omitempty"`
	DryRun  bool     `json:"dry_run"`
}

// handleBulkMerge merges all Tier 1 PRs with green CI.
func (h *Handler) handleBulkMerge(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input bulkMergeInput,
) (*gosdk.CallToolResult, any, error) {
	// Tier 3 action — requires confirmation unless dry-run.
	if !input.DryRun {
		if result := h.checkTier(autonomy.ActionMergeMain, "bulk_merge", func() (*gosdk.CallToolResult, error) {
			return h.executeBulkMerge(ctx, false)
		}); result != nil {
			return result, nil, nil
		}
	}

	r, err := h.executeBulkMerge(ctx, input.DryRun)
	return r, nil, err
}

func (h *Handler) executeBulkMerge(ctx context.Context, dryRun bool) (*gosdk.CallToolResult, error) {
	result := bulkMergeResult{DryRun: dryRun}

	type projectPRM struct {
		name string
		prm  forge.PullRequestManager
	}

	var managers []projectPRM
	if h.projects != nil {
		for _, proj := range h.projects.List() {
			if proj.PRManager != nil {
				managers = append(managers, projectPRM{name: proj.Name, prm: proj.PRManager})
			}
		}
	}
	if len(managers) == 0 {
		prm := h.activePRManager()
		if prm != nil {
			managers = append(managers, projectPRM{name: "default", prm: prm})
		}
	}

	for _, mgr := range managers {
		prs, err := mgr.prm.ListPullRequests(ctx, &forge.ListPROptions{
			State:   forge.StateOpen,
			PerPage: 50,
		})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: list error: %v", mgr.name, err))
			continue
		}

		for _, pr := range prs {
			label := fmt.Sprintf("%s#%d %s", mgr.name, pr.Number, pr.Title)
			tier := prwatcher.ClassifyPRTier(pr, nil)
			if tier != prwatcher.PRTier1 {
				result.Skipped = append(result.Skipped, fmt.Sprintf("%s (tier-%d)", label, tier))
				continue
			}
			if pr.Draft || !pr.Mergeable {
				result.Skipped = append(result.Skipped, fmt.Sprintf("%s (draft or conflicts)", label))
				continue
			}

			checks, _ := mgr.prm.GetPRChecks(ctx, pr.Number)
			ciStatus, _ := summarizeChecks(checks)
			if ciStatus != "passed" {
				result.Skipped = append(result.Skipped, fmt.Sprintf("%s (CI: %s)", label, ciStatus))
				continue
			}

			if dryRun {
				result.Merged = append(result.Merged, fmt.Sprintf("%s (would merge)", label))
				continue
			}

			commitMsg := fmt.Sprintf("auto-merge: %s (#%d)", pr.Title, pr.Number)
			if mergeErr := mgr.prm.MergePullRequest(ctx, pr.Number, forge.MergeMethodSquash, commitMsg); mergeErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", label, mergeErr))
			} else {
				result.Merged = append(result.Merged, label)
			}
		}
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshalling bulk merge result: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(data)},
		},
	}, nil
}

// populateDigestPRs enriches DigestData with open PRs from all registered projects.
func (h *Handler) populateDigestPRs(ctx context.Context, d *digest.DigestData) {
	type projectPRM struct {
		name string
		prm  forge.PullRequestManager
	}

	var managers []projectPRM
	if h.projects != nil {
		for _, proj := range h.projects.List() {
			if proj.PRManager != nil {
				managers = append(managers, projectPRM{name: proj.Name, prm: proj.PRManager})
			}
		}
	}
	if len(managers) == 0 {
		prm := h.activePRManager()
		if prm != nil {
			managers = append(managers, projectPRM{name: "default", prm: prm})
		}
	}

	for _, mgr := range managers {
		prs, err := mgr.prm.ListPullRequests(ctx, &forge.ListPROptions{
			State:   forge.StateOpen,
			PerPage: 50,
		})
		if err != nil {
			continue
		}
		for _, pr := range prs {
			tier := prwatcher.ClassifyPRTier(pr, nil)
			checks, _ := mgr.prm.GetPRChecks(ctx, pr.Number)
			ciStatus, _ := summarizeChecks(checks)

			// Count auto-merged PRs (closed + merged since last check-in).
			// Open PRs go into the awaiting list.
			d.PRsAwaiting = append(d.PRsAwaiting, digest.PRAwaitingReview{
				Project:  mgr.name,
				Number:   pr.Number,
				Title:    pr.Title,
				Author:   pr.Author,
				Tier:     tier.String(),
				TierDesc: tier.Description(),
				CIStatus: ciStatus,
				Age:      formatAge(pr.CreatedAt),
			})
		}
	}
}

// formatAge returns a human-readable age string like "2d", "5h", "30m".
func formatAge(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	hours := int(d.Hours())
	if hours < 1 {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if hours < 48 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dd", hours/24)
}
