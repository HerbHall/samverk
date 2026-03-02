package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

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
}

// handleGetDigest builds and formats a check-in digest.
func (h *Handler) handleGetDigest(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input getDigestInput,
) (*gosdk.CallToolResult, any, error) {
	dur, err := time.ParseDuration(input.Since)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid since duration %q: %w", input.Since, err)
	}

	sinceTime := time.Now().Add(-dur)
	data, err := digest.BuildDigest(ctx, h.tracker, h.costs, sinceTime)
	if err != nil {
		return nil, nil, fmt.Errorf("building digest: %w", err)
	}

	text := digest.FormatDigest(data)

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

	if err := h.tracker.AddLabel(ctx, input.IssueNumber, input.Label); err != nil {
		return nil, nil, fmt.Errorf("adding label: %w", err)
	}

	h.recorder.recordToolCall(ctx, "add_label", input.IssueNumber)

	text := fmt.Sprintf("Added label %q to issue #%d", input.Label, input.IssueNumber)
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: text},
		},
	}, nil, nil
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

	if err := h.tracker.RemoveLabel(ctx, input.IssueNumber, input.Label); err != nil {
		return nil, nil, fmt.Errorf("removing label: %w", err)
	}

	h.recorder.recordToolCall(ctx, "remove_label", input.IssueNumber)

	text := fmt.Sprintf("Removed label %q from issue #%d", input.Label, input.IssueNumber)
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: text},
		},
	}, nil, nil
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

	comment, err := h.tracker.AddComment(ctx, input.IssueNumber, input.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("adding comment: %w", err)
	}

	h.recorder.recordToolCall(ctx, "add_comment", input.IssueNumber)

	result, err := json.Marshal(map[string]any{
		"comment_id":   comment.ID,
		"issue_number": input.IssueNumber,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling comment result: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
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

	issue, err := h.tracker.CreateIssue(ctx, &forge.CreateIssueRequest{
		Title:     input.Title,
		Body:      input.Body,
		Labels:    input.Labels,
		Assignees: input.Assignees,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("creating issue: %w", err)
	}

	h.recorder.recordToolCall(ctx, "create_issue", issue.Number)

	result, err := json.Marshal(map[string]any{
		"issue_number": issue.Number,
		"title":        issue.Title,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling issue result: %w", err)
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
