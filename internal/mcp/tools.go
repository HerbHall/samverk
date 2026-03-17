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
)

// getDigestInput is the typed input for the get_digest tool.
type getDigestInput struct {
	Since string `json:"since" jsonschema:"duration like 24h or 168h for the lookback window"`
}

// getCostSummaryInput is the typed input for the get_cost_summary tool.
type getCostSummaryInput struct {
	Since string `json:"since" jsonschema:"duration like 24h or 168h for the lookback window"`
}

// addLabelInput is the typed input for the add_label tool.
type addLabelInput struct {
	IssueNumber int    `json:"issue_number" jsonschema:"required,the issue number to add the label to"`
	Label       string `json:"label" jsonschema:"required,the label name to add"`
}

// removeLabelInput is the typed input for the remove_label tool.
type removeLabelInput struct {
	IssueNumber int    `json:"issue_number" jsonschema:"required,the issue number to remove the label from"`
	Label       string `json:"label" jsonschema:"required,the label name to remove"`
}

// addCommentInput is the typed input for the add_comment tool.
type addCommentInput struct {
	IssueNumber int    `json:"issue_number" jsonschema:"required,the issue number to comment on"`
	Body        string `json:"body" jsonschema:"required,the comment body text"`
}

// createIssueInput is the typed input for the create_issue tool.
type createIssueInput struct {
	Title     string   `json:"title" jsonschema:"required,the issue title"`
	Body      string   `json:"body" jsonschema:"required,the issue body text"`
	Labels    []string `json:"labels,omitempty" jsonschema:"optional label names to apply"`
	Assignees []string `json:"assignees,omitempty" jsonschema:"optional usernames to assign"`
}

// listIssuesInput is the typed input for the list_issues tool.
type listIssuesInput struct {
	State    string   `json:"state,omitempty" jsonschema:"filter by state: open or closed"`
	Labels   []string `json:"labels,omitempty" jsonschema:"filter by label names"`
	Assignee string   `json:"assignee,omitempty" jsonschema:"filter by assignee username"`
	Page     int      `json:"page,omitempty" jsonschema:"page number for pagination"`
	PerPage  int      `json:"per_page,omitempty" jsonschema:"results per page for pagination"`
}

// getIssueInput is the typed input for the get_issue tool.
type getIssueInput struct {
	IssueNumber int `json:"issue_number" jsonschema:"required,the issue number to retrieve"`
}

// updateIssueInput is the typed input for the update_issue tool.
type updateIssueInput struct {
	IssueNumber int     `json:"issue_number" jsonschema:"required,the issue number to update"`
	Title       *string `json:"title,omitempty" jsonschema:"new title for the issue"`
	Body        *string `json:"body,omitempty" jsonschema:"new body for the issue"`
	State       *string `json:"state,omitempty" jsonschema:"new state: open or closed"`
}

// closeIssueInput is the typed input for the close_issue tool.
type closeIssueInput struct {
	IssueNumber int `json:"issue_number" jsonschema:"required,the issue number to close"`
}

// reopenIssueInput is the typed input for the reopen_issue tool.
type reopenIssueInput struct {
	IssueNumber int `json:"issue_number" jsonschema:"required,the issue number to reopen"`
}

// setLabelsInput is the typed input for the set_labels tool.
type setLabelsInput struct {
	IssueNumber int      `json:"issue_number" jsonschema:"required,the issue number to set labels on"`
	Labels      []string `json:"labels" jsonschema:"required,the complete set of labels to apply"`
}

// listCommentsInput is the typed input for the list_comments tool.
type listCommentsInput struct {
	IssueNumber int `json:"issue_number" jsonschema:"required,the issue number to list comments for"`
}

// approveActionInput is the typed input for the approve_action tool.
type approveActionInput struct {
	ActionID string `json:"action_id" jsonschema:"required,the pending action ID to approve"`
}

// rejectActionInput is the typed input for the reject_action tool.
type rejectActionInput struct {
	ActionID string `json:"action_id" jsonschema:"required,the pending action ID to reject"`
	Reason   string `json:"reason,omitempty" jsonschema:"optional reason for rejection"`
}

// registerTools adds all MCP tools to the server.
func registerTools(srv *gosdk.Server, h *Handler) {
	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "get_digest",
		Description: "Get check-in digest showing pending decisions, completed work, and project status",
	}, h.handleGetDigest)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "get_cost_summary",
		Description: "Get token usage and cost summary for a time period",
	}, h.handleGetCostSummary)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "add_label",
		Description: "Add a label to an issue",
	}, h.handleAddLabel)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "remove_label",
		Description: "Remove a label from an issue",
	}, h.handleRemoveLabel)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "add_comment",
		Description: "Add a comment to an issue",
	}, h.handleAddComment)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "create_issue",
		Description: "Create a new issue on the forge",
	}, h.handleCreateIssue)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "list_issues",
		Description: "List issues with optional filters for state, labels, and assignee",
	}, h.handleListIssues)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "get_issue",
		Description: "Get full details of a single issue by number",
	}, h.handleGetIssue)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "update_issue",
		Description: "Update an existing issue's title, body, or state",
	}, h.handleUpdateIssue)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "close_issue",
		Description: "Close an issue",
	}, h.handleCloseIssue)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "reopen_issue",
		Description: "Reopen a closed issue",
	}, h.handleReopenIssue)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "set_labels",
		Description: "Replace all labels on an issue with the given set",
	}, h.handleSetLabels)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "list_comments",
		Description: "List all comments on an issue",
	}, h.handleListComments)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "approve_action",
		Description: "Approve a pending Tier 3 action for execution",
	}, h.handleApproveAction)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "reject_action",
		Description: "Reject a pending Tier 3 action",
	}, h.handleRejectAction)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "get_failure_summary",
		Description: "Get failure analysis summary: failure counts by class, top failing issues, looping issues, provider health, and circuit breaker status",
	}, h.handleGetFailureSummary)
}

// handleGetDigest builds and formats a check-in digest.
func (h *Handler) handleGetDigest(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input getDigestInput,
) (*gosdk.CallToolResult, any, error) {
	// Determine the last check-in anchor.
	// Prefer the persisted timestamp (actual last time get_digest was called) so
	// the "You've been away X" message reflects real elapsed time rather than
	// echoing the `since` parameter back to the user.
	// Fall back to the `since` parameter only when no stored value exists.
	var lastCheckIn time.Time
	if h.store != nil {
		if t, err := h.store.GetLastCheckIn(ctx); err == nil {
			lastCheckIn = t
		}
	}
	if lastCheckIn.IsZero() {
		dur, err := time.ParseDuration(input.Since)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid since duration %q: %w", input.Since, err)
		}
		lastCheckIn = time.Now().Add(-dur)
	}

	data, err := digest.BuildDigest(ctx, h.activeTracker(), h.costs, lastCheckIn)
	if err != nil {
		return nil, nil, fmt.Errorf("building digest: %w", err)
	}

	// Persist the current time as the new check-in anchor for the next call.
	if h.store != nil {
		_ = h.store.SetLastCheckIn(ctx, time.Now())
	}

	// Enrich digest with PR data from all registered projects.
	h.populateDigestPRs(ctx, &data)

	text := digest.FormatDigest(data) + h.formatMetricsSection()

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: text},
		},
	}, nil, nil
}

// handleAddLabel adds a label to an issue.
func (h *Handler) handleAddLabel(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input addLabelInput,
) (*gosdk.CallToolResult, any, error) {
	if input.IssueNumber <= 0 {
		return nil, nil, fmt.Errorf("issue_number must be greater than 0")
	}
	if input.Label == "" {
		return nil, nil, fmt.Errorf("label must not be empty")
	}

	if result := h.checkTier(autonomy.ActionLabelIssue, "add_label", func() (*gosdk.CallToolResult, error) {
		return h.executeAddLabel(ctx, input)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeAddLabel(ctx, input)
	return r, nil, err
}

func (h *Handler) executeAddLabel(ctx context.Context, input addLabelInput) (*gosdk.CallToolResult, error) {
	if err := h.activeTracker().AddLabel(ctx, input.IssueNumber, input.Label); err != nil {
		return nil, fmt.Errorf("adding label: %w", err)
	}

	h.recorder.recordToolCall(ctx, "add_label", input.IssueNumber)

	text := fmt.Sprintf("Added label %q to issue #%d", input.Label, input.IssueNumber)
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: text},
		},
	}, nil
}

// handleRemoveLabel removes a label from an issue.
func (h *Handler) handleRemoveLabel(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input removeLabelInput,
) (*gosdk.CallToolResult, any, error) {
	if input.IssueNumber <= 0 {
		return nil, nil, fmt.Errorf("issue_number must be greater than 0")
	}
	if input.Label == "" {
		return nil, nil, fmt.Errorf("label must not be empty")
	}

	if result := h.checkTier(autonomy.ActionLabelIssue, "remove_label", func() (*gosdk.CallToolResult, error) {
		return h.executeRemoveLabel(ctx, input)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeRemoveLabel(ctx, input)
	return r, nil, err
}

func (h *Handler) executeRemoveLabel(ctx context.Context, input removeLabelInput) (*gosdk.CallToolResult, error) {
	if err := h.activeTracker().RemoveLabel(ctx, input.IssueNumber, input.Label); err != nil {
		return nil, fmt.Errorf("removing label: %w", err)
	}

	h.recorder.recordToolCall(ctx, "remove_label", input.IssueNumber)

	text := fmt.Sprintf("Removed label %q from issue #%d", input.Label, input.IssueNumber)
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: text},
		},
	}, nil
}

// handleAddComment adds a comment to an issue.
func (h *Handler) handleAddComment(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input addCommentInput,
) (*gosdk.CallToolResult, any, error) {
	if input.IssueNumber <= 0 {
		return nil, nil, fmt.Errorf("issue_number must be greater than 0")
	}
	if input.Body == "" {
		return nil, nil, fmt.Errorf("body must not be empty")
	}

	if result := h.checkTier(autonomy.ActionCommentIssue, "add_comment", func() (*gosdk.CallToolResult, error) {
		return h.executeAddComment(ctx, input)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeAddComment(ctx, input)
	return r, nil, err
}

func (h *Handler) executeAddComment(ctx context.Context, input addCommentInput) (*gosdk.CallToolResult, error) {
	comment, err := h.activeTracker().AddComment(ctx, input.IssueNumber, input.Body)
	if err != nil {
		return nil, fmt.Errorf("adding comment: %w", err)
	}

	h.recorder.recordToolCall(ctx, "add_comment", input.IssueNumber)

	result, err := json.Marshal(map[string]any{
		"comment_id":   comment.ID,
		"issue_number": input.IssueNumber,
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling comment result: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil
}

// handleCreateIssue creates a new issue on the forge.
func (h *Handler) handleCreateIssue(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input createIssueInput,
) (*gosdk.CallToolResult, any, error) {
	if input.Title == "" {
		return nil, nil, fmt.Errorf("title must not be empty")
	}

	if result := h.checkTier(autonomy.ActionOpenIssue, "create_issue", func() (*gosdk.CallToolResult, error) {
		return h.executeCreateIssue(ctx, input)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeCreateIssue(ctx, input)
	return r, nil, err
}

func (h *Handler) executeCreateIssue(ctx context.Context, input createIssueInput) (*gosdk.CallToolResult, error) {
	issue, err := h.activeTracker().CreateIssue(ctx, &forge.CreateIssueRequest{
		Title:     input.Title,
		Body:      input.Body,
		Labels:    input.Labels,
		Assignees: input.Assignees,
	})
	if err != nil {
		return nil, fmt.Errorf("creating issue: %w", err)
	}

	h.recorder.recordToolCall(ctx, "create_issue", issue.Number)

	result, err := json.Marshal(map[string]any{
		"issue_number": issue.Number,
		"title":        issue.Title,
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling issue result: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil
}

// handleListIssues lists issues with optional filters.
func (h *Handler) handleListIssues(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input listIssuesInput,
) (*gosdk.CallToolResult, any, error) {
	opts := &forge.ListOptions{
		State:    forge.State(input.State),
		Labels:   input.Labels,
		Assignee: input.Assignee,
		Page:     input.Page,
		PerPage:  input.PerPage,
	}

	issues, err := h.activeTracker().ListIssues(ctx, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("listing issues: %w", err)
	}

	result, err := json.Marshal(issues)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling issues: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleGetIssue retrieves full details of a single issue.
func (h *Handler) handleGetIssue(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input getIssueInput,
) (*gosdk.CallToolResult, any, error) {
	if input.IssueNumber <= 0 {
		return nil, nil, fmt.Errorf("issue_number must be greater than 0")
	}

	issue, err := h.activeTracker().GetIssue(ctx, input.IssueNumber)
	if err != nil {
		return nil, nil, fmt.Errorf("getting issue: %w", err)
	}

	result, err := json.Marshal(issue)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling issue: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleUpdateIssue updates an existing issue's title, body, or state.
func (h *Handler) handleUpdateIssue(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input updateIssueInput,
) (*gosdk.CallToolResult, any, error) {
	if input.IssueNumber <= 0 {
		return nil, nil, fmt.Errorf("issue_number must be greater than 0")
	}

	if result := h.checkTier(autonomy.ActionEditFile, "update_issue", func() (*gosdk.CallToolResult, error) {
		return h.executeUpdateIssue(ctx, input)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeUpdateIssue(ctx, input)
	return r, nil, err
}

func (h *Handler) executeUpdateIssue(ctx context.Context, input updateIssueInput) (*gosdk.CallToolResult, error) {
	req := &forge.UpdateIssueRequest{
		Title: input.Title,
		Body:  input.Body,
	}
	if input.State != nil {
		s := forge.State(*input.State)
		req.State = &s
	}

	issue, err := h.activeTracker().UpdateIssue(ctx, input.IssueNumber, req)
	if err != nil {
		return nil, fmt.Errorf("updating issue: %w", err)
	}

	h.recorder.recordToolCall(ctx, "update_issue", input.IssueNumber)

	result, err := json.Marshal(issue)
	if err != nil {
		return nil, fmt.Errorf("marshalling updated issue: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil
}

// handleCloseIssue is a convenience wrapper that closes an issue.
func (h *Handler) handleCloseIssue(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input closeIssueInput,
) (*gosdk.CallToolResult, any, error) {
	if input.IssueNumber <= 0 {
		return nil, nil, fmt.Errorf("issue_number must be greater than 0")
	}

	if result := h.checkTier(autonomy.ActionCloseIssue, "close_issue", func() (*gosdk.CallToolResult, error) {
		return h.executeCloseIssue(ctx, input)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeCloseIssue(ctx, input)
	return r, nil, err
}

func (h *Handler) executeCloseIssue(ctx context.Context, input closeIssueInput) (*gosdk.CallToolResult, error) {
	closedState := forge.StateClosed
	_, err := h.activeTracker().UpdateIssue(ctx, input.IssueNumber, &forge.UpdateIssueRequest{
		State: &closedState,
	})
	if err != nil {
		return nil, fmt.Errorf("closing issue: %w", err)
	}

	h.recorder.recordToolCall(ctx, "close_issue", input.IssueNumber)

	text := fmt.Sprintf("Closed issue #%d", input.IssueNumber)
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: text},
		},
	}, nil
}

// handleReopenIssue is a convenience wrapper that reopens a closed issue.
func (h *Handler) handleReopenIssue(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input reopenIssueInput,
) (*gosdk.CallToolResult, any, error) {
	if input.IssueNumber <= 0 {
		return nil, nil, fmt.Errorf("issue_number must be greater than 0")
	}

	if result := h.checkTier(autonomy.ActionCloseIssue, "reopen_issue", func() (*gosdk.CallToolResult, error) {
		return h.executeReopenIssue(ctx, input)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeReopenIssue(ctx, input)
	return r, nil, err
}

func (h *Handler) executeReopenIssue(ctx context.Context, input reopenIssueInput) (*gosdk.CallToolResult, error) {
	openState := forge.StateOpen
	_, err := h.activeTracker().UpdateIssue(ctx, input.IssueNumber, &forge.UpdateIssueRequest{
		State: &openState,
	})
	if err != nil {
		return nil, fmt.Errorf("reopening issue: %w", err)
	}

	h.recorder.recordToolCall(ctx, "reopen_issue", input.IssueNumber)

	text := fmt.Sprintf("Reopened issue #%d", input.IssueNumber)
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: text},
		},
	}, nil
}

// handleSetLabels replaces all labels on an issue with the given set.
func (h *Handler) handleSetLabels(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input setLabelsInput,
) (*gosdk.CallToolResult, any, error) {
	if input.IssueNumber <= 0 {
		return nil, nil, fmt.Errorf("issue_number must be greater than 0")
	}
	if len(input.Labels) == 0 {
		return nil, nil, fmt.Errorf("labels must not be empty")
	}

	if result := h.checkTier(autonomy.ActionLabelIssue, "set_labels", func() (*gosdk.CallToolResult, error) {
		return h.executeSetLabels(ctx, input)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeSetLabels(ctx, input)
	return r, nil, err
}

func (h *Handler) executeSetLabels(ctx context.Context, input setLabelsInput) (*gosdk.CallToolResult, error) {
	if err := h.activeTracker().SetLabels(ctx, input.IssueNumber, input.Labels); err != nil {
		return nil, fmt.Errorf("setting labels: %w", err)
	}

	h.recorder.recordToolCall(ctx, "set_labels", input.IssueNumber)

	text := fmt.Sprintf("Set %d label(s) on issue #%d", len(input.Labels), input.IssueNumber)
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: text},
		},
	}, nil
}

// handleListComments lists all comments on an issue.
func (h *Handler) handleListComments(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input listCommentsInput,
) (*gosdk.CallToolResult, any, error) {
	if input.IssueNumber <= 0 {
		return nil, nil, fmt.Errorf("issue_number must be greater than 0")
	}

	comments, err := h.activeTracker().ListComments(ctx, input.IssueNumber)
	if err != nil {
		return nil, nil, fmt.Errorf("listing comments: %w", err)
	}

	result, err := json.Marshal(comments)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling comments: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleGetCostSummary returns token usage and cost data.
func (h *Handler) handleGetCostSummary(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input getCostSummaryInput,
) (*gosdk.CallToolResult, any, error) {
	if h.costs == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "no cost data available"},
			},
		}, nil, nil
	}

	dur, err := time.ParseDuration(input.Since)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid since duration %q: %w", input.Since, err)
	}

	sinceTime := time.Now().Add(-dur)
	summary := h.costs.ComputeCostSince(ctx, sinceTime)

	text := fmt.Sprintf("Cost Summary (last %s):\n"+
		"  Tokens used: %d\n"+
		"  Estimated cost: $%.2f\n"+
		"  Budget remaining: $%.2f\n",
		input.Since,
		summary.TokensUsed,
		summary.EstimatedCostUSD,
		summary.BudgetRemainingUSD,
	)

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: text},
		},
	}, nil, nil
}

// handleApproveAction executes a pending Tier 3 action after user confirmation.
func (h *Handler) handleApproveAction(
	_ context.Context,
	_ *gosdk.CallToolRequest,
	input approveActionInput,
) (*gosdk.CallToolResult, any, error) {
	if input.ActionID == "" {
		return nil, nil, fmt.Errorf("action_id must not be empty")
	}

	action, ok := h.pending.get(input.ActionID)
	if !ok {
		return nil, nil, fmt.Errorf("pending action %q not found or expired", input.ActionID)
	}

	h.pending.remove(input.ActionID)

	result, err := action.Execute()
	if err != nil {
		return nil, nil, err
	}
	return result, nil, nil
}

// handleRejectAction removes a pending Tier 3 action without executing it.
func (h *Handler) handleRejectAction(
	_ context.Context,
	_ *gosdk.CallToolRequest,
	input rejectActionInput,
) (*gosdk.CallToolResult, any, error) {
	if input.ActionID == "" {
		return nil, nil, fmt.Errorf("action_id must not be empty")
	}

	_, ok := h.pending.get(input.ActionID)
	if !ok {
		return nil, nil, fmt.Errorf("pending action %q not found or expired", input.ActionID)
	}

	h.pending.remove(input.ActionID)

	reason := input.Reason
	if reason == "" {
		reason = "rejected by user"
	}

	text := fmt.Sprintf("Action %s rejected: %s", input.ActionID, reason)
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: text},
		},
	}, nil, nil
}
