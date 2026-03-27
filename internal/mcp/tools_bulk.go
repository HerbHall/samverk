package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/forge"
)

// bulkUpdateIssuesInput is the typed input for the bulk_update_issues tool.
type bulkUpdateIssuesInput struct {
	Updates []issueUpdate `json:"updates" jsonschema:"required,list of issue updates to apply"`
}

// issueUpdate describes a single issue update within a bulk operation.
type issueUpdate struct {
	IssueNumber int      `json:"issue_number" jsonschema:"required,the issue number to update"`
	Labels      []string `json:"labels,omitempty" jsonschema:"replace all labels on the issue"`
	State       *string  `json:"state,omitempty" jsonschema:"new state: open or closed"`
	Title       *string  `json:"title,omitempty" jsonschema:"new title for the issue"`
}

// bulkUpdateResult holds the outcome of a single issue update.
type bulkUpdateResult struct {
	IssueNumber int    `json:"issue_number"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
}

// closeIssueWithCommentInput is the typed input for the close_issue_with_comment tool.
type closeIssueWithCommentInput struct {
	IssueNumber int    `json:"issue_number" jsonschema:"required,the issue number to close"`
	Comment     string `json:"comment" jsonschema:"required,comment to add before closing"`
}

// createIssueBatchInput is the typed input for the create_issue_batch tool.
type createIssueBatchInput struct {
	Issues []batchIssueSpec `json:"issues" jsonschema:"required,list of issues to create"`
}

// batchIssueSpec describes a single issue to create in a batch.
type batchIssueSpec struct {
	Title       string            `json:"title" jsonschema:"required,the issue title -- must begin with a conventional commit prefix"`
	Body        string            `json:"body" jsonschema:"required,the issue body text"`
	Labels      []string          `json:"labels,omitempty" jsonschema:"optional label names to apply -- agent:* and priority:* are injected automatically when absent"`
	Assignees   []string          `json:"assignees,omitempty" jsonschema:"optional usernames to assign"`
	Frontmatter map[string]string `json:"frontmatter,omitempty" jsonschema:"optional key/value pairs prepended to body as a YAML frontmatter block"`
}

// batchCreateResult holds the outcome of a single issue creation.
type batchCreateResult struct {
	Index       int    `json:"index"`
	IssueNumber int    `json:"issue_number,omitempty"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
}

// getProjectSummaryInput is the typed input for the get_project_summary tool.
type getProjectSummaryInput struct{}

// labelBucket holds a count of issues matching a label prefix.
type labelBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// projectSummary is the structured output of get_project_summary.
type projectSummary struct {
	OpenIssueCount   int           `json:"open_issue_count"`
	ClosedIssueCount int           `json:"closed_issue_count"`
	LabelBuckets     []labelBucket `json:"label_buckets"`
	OpenPRCount      int           `json:"open_pr_count,omitempty"`
	LastActivity     string        `json:"last_activity,omitempty"`
}

// bulkRelabelInput is the typed input for the bulk_relabel tool.
type bulkRelabelInput struct {
	IssueNumbers []int    `json:"issue_numbers" jsonschema:"required,list of issue numbers to relabel"`
	RemoveLabels []string `json:"remove_labels" jsonschema:"labels to remove from each issue"`
	AddLabels    []string `json:"add_labels" jsonschema:"labels to add to each issue"`
}

// bulkRelabelResult is the structured output of bulk_relabel.
type bulkRelabelResult struct {
	Succeeded int                `json:"succeeded"`
	Failed    int                `json:"failed"`
	Details   []bulkUpdateResult `json:"details"`
}

// registerBulkTools adds bulk forge operation and project summary MCP tools to the server.
func registerBulkTools(srv *gosdk.Server, h *Handler) {
	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "bulk_update_issues",
		Description: "Apply label, state, or title changes to multiple issues in one call",
	}, h.handleBulkUpdateIssues)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "close_issue_with_comment",
		Description: "Add a comment to an issue and close it in one atomic call",
	}, h.handleCloseIssueWithComment)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "create_issue_batch",
		Description: "Create multiple issues at once from an array of issue specs",
	}, h.handleCreateIssueBatch)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "get_project_summary",
		Description: "Get a structured snapshot of the project: open/closed issue counts by label bucket, open PR count, and last activity timestamp",
	}, h.handleGetProjectSummary)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "bulk_relabel",
		Description: "Remove and add labels on multiple issues in one call, preserving all other labels",
	}, h.handleBulkRelabel)
}

// handleBulkUpdateIssues applies label, state, or title changes to multiple issues.
func (h *Handler) handleBulkUpdateIssues(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input bulkUpdateIssuesInput,
) (*gosdk.CallToolResult, any, error) {
	if len(input.Updates) == 0 {
		return nil, nil, fmt.Errorf("updates must not be empty")
	}

	tracker := h.activeTracker()
	if tracker == nil {
		return nil, nil, fmt.Errorf("no issue tracker configured")
	}

	results := make([]bulkUpdateResult, 0, len(input.Updates))
	for _, u := range input.Updates {
		r := bulkUpdateResult{IssueNumber: u.IssueNumber}

		if u.IssueNumber <= 0 {
			r.Status = "error"
			r.Error = "issue_number must be greater than 0"
			results = append(results, r)
			continue
		}

		if err := h.applyIssueUpdate(ctx, tracker, u); err != nil {
			r.Status = "error"
			r.Error = err.Error()
		} else {
			r.Status = "ok"
			h.recorder.recordToolCall(ctx, "bulk_update_issues", u.IssueNumber)
		}
		results = append(results, r)
	}

	out, err := json.Marshal(results)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling bulk update results: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{&gosdk.TextContent{Text: string(out)}},
	}, nil, nil
}

// applyIssueUpdate applies a single issue update (labels, state, title).
func (h *Handler) applyIssueUpdate(ctx context.Context, tracker forge.IssueTracker, u issueUpdate) error {
	if len(u.Labels) > 0 {
		if err := tracker.SetLabels(ctx, u.IssueNumber, u.Labels); err != nil {
			return fmt.Errorf("setting labels: %w", err)
		}
	}

	if u.State != nil || u.Title != nil {
		req := &forge.UpdateIssueRequest{Title: u.Title}
		if u.State != nil {
			s := forge.State(*u.State)
			req.State = &s
		}
		if _, err := tracker.UpdateIssue(ctx, u.IssueNumber, req); err != nil {
			return fmt.Errorf("updating issue: %w", err)
		}
	}

	return nil
}

// handleCloseIssueWithComment adds a comment and closes an issue in one call.
func (h *Handler) handleCloseIssueWithComment(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input closeIssueWithCommentInput,
) (*gosdk.CallToolResult, any, error) {
	if input.IssueNumber <= 0 {
		return nil, nil, fmt.Errorf("issue_number must be greater than 0")
	}
	if input.Comment == "" {
		return nil, nil, fmt.Errorf("comment must not be empty")
	}

	if result := h.checkTier(autonomy.ActionCloseIssue, "close_issue_with_comment", func() (*gosdk.CallToolResult, error) {
		return h.executeCloseIssueWithComment(ctx, input)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeCloseIssueWithComment(ctx, input)
	return r, nil, err
}

func (h *Handler) executeCloseIssueWithComment(ctx context.Context, input closeIssueWithCommentInput) (*gosdk.CallToolResult, error) {
	tracker := h.activeTracker()
	if tracker == nil {
		return nil, fmt.Errorf("no issue tracker configured")
	}

	if _, err := tracker.AddComment(ctx, input.IssueNumber, input.Comment); err != nil {
		return nil, fmt.Errorf("adding comment: %w", err)
	}

	closedState := forge.StateClosed
	if _, err := tracker.UpdateIssue(ctx, input.IssueNumber, &forge.UpdateIssueRequest{
		State: &closedState,
	}); err != nil {
		return nil, fmt.Errorf("closing issue: %w", err)
	}

	h.recorder.recordToolCall(ctx, "close_issue_with_comment", input.IssueNumber)

	text := fmt.Sprintf("Added comment and closed issue #%d", input.IssueNumber)
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{&gosdk.TextContent{Text: text}},
	}, nil
}

// handleCreateIssueBatch creates multiple issues at once.
func (h *Handler) handleCreateIssueBatch(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input createIssueBatchInput,
) (*gosdk.CallToolResult, any, error) {
	if len(input.Issues) == 0 {
		return nil, nil, fmt.Errorf("issues must not be empty")
	}

	if result := h.checkTier(autonomy.ActionOpenIssue, "create_issue_batch", func() (*gosdk.CallToolResult, error) {
		return h.executeCreateIssueBatch(ctx, input)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeCreateIssueBatch(ctx, input)
	return r, nil, err
}

func (h *Handler) executeCreateIssueBatch(ctx context.Context, input createIssueBatchInput) (*gosdk.CallToolResult, error) {
	tracker := h.activeTracker()
	if tracker == nil {
		return nil, fmt.Errorf("no issue tracker configured")
	}

	results := make([]batchCreateResult, 0, len(input.Issues))
	for i, spec := range input.Issues {
		r := batchCreateResult{Index: i, Title: spec.Title}

		if spec.Title == "" {
			r.Status = "error"
			r.Error = "title must not be empty"
			results = append(results, r)
			continue
		}

		enrichedLabels, enrichedBody, valErr := validateAndEnrichIssue(spec.Title, spec.Body, spec.Labels, spec.Frontmatter)
		if valErr != nil {
			r.Status = "error"
			r.Error = valErr.Error()
			results = append(results, r)
			continue
		}

		issue, err := tracker.CreateIssue(ctx, &forge.CreateIssueRequest{
			Title:     spec.Title,
			Body:      enrichedBody,
			Labels:    enrichedLabels,
			Assignees: spec.Assignees,
		})
		if err != nil {
			r.Status = "error"
			r.Error = err.Error()
		} else {
			r.IssueNumber = issue.Number
			r.Status = "created"
			h.recorder.recordToolCall(ctx, "create_issue_batch", issue.Number)
		}
		results = append(results, r)
	}

	out, err := json.Marshal(results)
	if err != nil {
		return nil, fmt.Errorf("marshalling batch create results: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{&gosdk.TextContent{Text: string(out)}},
	}, nil
}

// handleGetProjectSummary returns a structured snapshot of the project.
func (h *Handler) handleGetProjectSummary(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	_ getProjectSummaryInput,
) (*gosdk.CallToolResult, any, error) {
	h.touchCheckIn(ctx)

	tracker := h.activeTracker()
	if tracker == nil {
		return nil, nil, fmt.Errorf("no issue tracker configured")
	}

	summary, err := h.buildProjectSummary(ctx, tracker)
	if err != nil {
		return nil, nil, fmt.Errorf("building project summary: %w", err)
	}

	out, err := json.Marshal(summary)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling project summary: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{&gosdk.TextContent{Text: string(out)}},
	}, nil, nil
}

// buildProjectSummary aggregates issue and PR data into a summary.
func (h *Handler) buildProjectSummary(ctx context.Context, tracker forge.IssueTracker) (summary projectSummary, err error) {
	openIssues, err := tracker.ListIssues(ctx, &forge.ListOptions{State: forge.StateOpen, PerPage: 100})
	if err != nil {
		return summary, fmt.Errorf("listing open issues: %w", err)
	}

	closedIssues, err := tracker.ListIssues(ctx, &forge.ListOptions{State: forge.StateClosed, PerPage: 100})
	if err != nil {
		return summary, fmt.Errorf("listing closed issues: %w", err)
	}

	summary.OpenIssueCount = len(openIssues)
	summary.ClosedIssueCount = len(closedIssues)

	// Count labels across open issues.
	labelCounts := make(map[string]int)
	for _, iss := range openIssues {
		for _, label := range iss.Labels {
			labelCounts[label]++
		}
	}

	buckets := make([]labelBucket, 0, len(labelCounts))
	for label, count := range labelCounts {
		buckets = append(buckets, labelBucket{Label: label, Count: count})
	}
	summary.LabelBuckets = buckets

	// Find last activity across all issues.
	var latest string
	allIssues := make([]*forge.Issue, 0, len(openIssues)+len(closedIssues))
	allIssues = append(allIssues, openIssues...)
	allIssues = append(allIssues, closedIssues...)
	for _, iss := range allIssues {
		ts := iss.UpdatedAt.Format("2006-01-02T15:04:05Z")
		if ts > latest {
			latest = ts
		}
	}
	summary.LastActivity = latest

	// Add PR count if a PR manager is available.
	prm := h.activePRManager()
	if prm != nil {
		prs, prErr := prm.ListPullRequests(ctx, &forge.ListPROptions{State: forge.StateOpen, PerPage: 100})
		if prErr == nil {
			summary.OpenPRCount = len(prs)
		}
	}

	return summary, nil
}

// handleBulkRelabel removes and adds labels on multiple issues, preserving other labels.
func (h *Handler) handleBulkRelabel(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input bulkRelabelInput,
) (*gosdk.CallToolResult, any, error) {
	if len(input.IssueNumbers) == 0 {
		return nil, nil, fmt.Errorf("issue_numbers must not be empty")
	}

	tracker := h.activeTracker()
	if tracker == nil {
		return nil, nil, fmt.Errorf("no issue tracker configured")
	}

	details := make([]bulkUpdateResult, 0, len(input.IssueNumbers))
	succeeded := 0
	failed := 0

	for _, num := range input.IssueNumbers {
		r := bulkUpdateResult{IssueNumber: num}

		if num <= 0 {
			r.Status = "error"
			r.Error = "issue_number must be greater than 0"
			details = append(details, r)
			failed++
			continue
		}

		if err := applyRelabel(ctx, tracker, num, input.RemoveLabels, input.AddLabels); err != nil {
			r.Status = "error"
			r.Error = err.Error()
			failed++
		} else {
			r.Status = "ok"
			succeeded++
			h.recorder.recordToolCall(ctx, "bulk_relabel", num)
		}
		details = append(details, r)
	}

	out, err := json.Marshal(bulkRelabelResult{
		Succeeded: succeeded,
		Failed:    failed,
		Details:   details,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling bulk relabel results: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{&gosdk.TextContent{Text: string(out)}},
	}, nil, nil
}

// applyRelabel removes then adds labels on a single issue.
func applyRelabel(ctx context.Context, tracker forge.IssueTracker, issueNumber int, removeLabels, addLabels []string) error {
	for i := range removeLabels {
		if err := tracker.RemoveLabel(ctx, issueNumber, removeLabels[i]); err != nil {
			return fmt.Errorf("removing label %q: %w", removeLabels[i], err)
		}
	}
	for i := range addLabels {
		if err := tracker.AddLabel(ctx, issueNumber, addLabels[i]); err != nil {
			return fmt.Errorf("adding label %q: %w", addLabels[i], err)
		}
	}
	return nil
}
