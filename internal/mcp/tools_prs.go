package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/forge"
)

// createPRInput is the typed input for the create_pr tool.
type createPRInput struct {
	Title string `json:"title" jsonschema:"required,the pull request title"`
	Body  string `json:"body" jsonschema:"required,the pull request body"`
	Head  string `json:"head" jsonschema:"required,the branch to merge from"`
	Base  string `json:"base,omitempty" jsonschema:"the branch to merge into (default: main)"`
}

// mergePRInput is the typed input for the merge_pr tool.
type mergePRInput struct {
	Number    int    `json:"number" jsonschema:"required,the pull request number to merge"`
	Method    string `json:"method,omitempty" jsonschema:"merge method: merge, squash, or rebase (default: squash)"`
	CommitMsg string `json:"commit_message,omitempty" jsonschema:"optional commit message for the merge"`
}

// listPRsInput is the typed input for the list_prs tool.
type listPRsInput struct {
	State   string `json:"state,omitempty" jsonschema:"filter by state: open or closed"`
	Head    string `json:"head,omitempty" jsonschema:"filter by head branch name"`
	Base    string `json:"base,omitempty" jsonschema:"filter by base branch name"`
	Page    int    `json:"page,omitempty" jsonschema:"page number for pagination"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"results per page for pagination"`
}

// getPRInput is the typed input for the get_pr tool.
type getPRInput struct {
	Number int `json:"number" jsonschema:"required,the pull request number to retrieve"`
}

// registerPRTools adds pull request MCP tools to the server.
func registerPRTools(srv *gosdk.Server, h *Handler) {
	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "create_pr",
		Description: "Create a pull request from a branch to base (default: main)",
	}, h.handleCreatePR)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "merge_pr",
		Description: "Merge a pull request (squash by default). Deletes the branch after merge.",
	}, h.handleMergePR)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "list_prs",
		Description: "List pull requests with optional filters for state, head, and base branch",
	}, h.handleListPRs)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "get_pr",
		Description: "Get full details of a single pull request by number",
	}, h.handleGetPR)
}

// handleCreatePR creates a new pull request.
func (h *Handler) handleCreatePR(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input createPRInput,
) (*gosdk.CallToolResult, any, error) {
	prm := h.activePRManager()
	if prm == nil {
		return nil, nil, fmt.Errorf("pull request operations are not available")
	}
	if input.Title == "" {
		return nil, nil, fmt.Errorf("title must not be empty")
	}
	if input.Head == "" {
		return nil, nil, fmt.Errorf("head branch must not be empty")
	}

	base := input.Base
	if base == "" {
		base = "main"
	}

	if result := h.checkTier(autonomy.ActionCreatePR, "create_pr", func() (*gosdk.CallToolResult, error) {
		return h.executeCreatePR(ctx, input, base)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeCreatePR(ctx, input, base)
	return r, nil, err
}

func (h *Handler) executeCreatePR(ctx context.Context, input createPRInput, base string) (*gosdk.CallToolResult, error) {
	pr, err := h.activePRManager().CreatePullRequest(ctx, &forge.CreatePRRequest{
		Title: input.Title,
		Body:  input.Body,
		Head:  input.Head,
		Base:  base,
	})
	if err != nil {
		return nil, fmt.Errorf("creating pull request: %w", err)
	}

	h.recorder.recordToolCall(ctx, "create_pr", pr.Number)

	result, err := json.Marshal(map[string]any{
		"number": pr.Number,
		"title":  pr.Title,
		"head":   pr.Head,
		"base":   pr.Base,
		"state":  pr.State,
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling PR result: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil
}

// handleMergePR merges a pull request.
func (h *Handler) handleMergePR(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input mergePRInput,
) (*gosdk.CallToolResult, any, error) {
	prm := h.activePRManager()
	if prm == nil {
		return nil, nil, fmt.Errorf("pull request operations are not available")
	}
	if input.Number <= 0 {
		return nil, nil, fmt.Errorf("number must be greater than 0")
	}

	// merge_main is Tier 3 by default -- requires user confirmation.
	if result := h.checkTier(autonomy.ActionMergeMain, "merge_pr", func() (*gosdk.CallToolResult, error) {
		return h.executeMergePR(ctx, input)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeMergePR(ctx, input)
	return r, nil, err
}

func (h *Handler) executeMergePR(ctx context.Context, input mergePRInput) (*gosdk.CallToolResult, error) {
	method := forge.MergeMethodSquash
	switch forge.MergeMethod(input.Method) {
	case forge.MergeMethodMerge, forge.MergeMethodRebase:
		method = forge.MergeMethod(input.Method)
	case forge.MergeMethodSquash, "":
		method = forge.MergeMethodSquash
	}

	commitMsg := input.CommitMsg
	if commitMsg == "" {
		commitMsg = fmt.Sprintf("Merge PR #%d", input.Number)
	}

	if err := h.activePRManager().MergePullRequest(ctx, input.Number, method, commitMsg); err != nil {
		return nil, fmt.Errorf("merging pull request #%d: %w", input.Number, err)
	}

	h.recorder.recordToolCall(ctx, "merge_pr", input.Number)

	text := fmt.Sprintf("Merged pull request #%d (%s)", input.Number, method)
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: text},
		},
	}, nil
}

// handleListPRs lists pull requests with optional filters.
func (h *Handler) handleListPRs(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input listPRsInput,
) (*gosdk.CallToolResult, any, error) {
	prm := h.activePRManager()
	if prm == nil {
		return nil, nil, fmt.Errorf("pull request operations are not available")
	}

	opts := &forge.ListPROptions{
		State:   forge.State(input.State),
		Head:    input.Head,
		Base:    input.Base,
		Page:    input.Page,
		PerPage: input.PerPage,
	}

	prs, err := prm.ListPullRequests(ctx, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("listing pull requests: %w", err)
	}

	result, err := json.Marshal(prs)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling pull requests: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleGetPR retrieves full details of a single pull request.
func (h *Handler) handleGetPR(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input getPRInput,
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
		return nil, nil, fmt.Errorf("getting pull request: %w", err)
	}

	result, err := json.Marshal(pr)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling pull request: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}
