package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"samverk.dev/samverk/internal/autonomy"
)

// claimIssueInput is the typed input for the claim_issue tool.
type claimIssueInput struct {
	Number   int    `json:"number" jsonschema:"required,the issue number to claim"`
	WorkerID string `json:"worker_id,omitempty" jsonschema:"optional worker identity (defaults to interactive:<timestamp>)"`
}

// requestWorkInput is the typed input for the request_work tool.
type requestWorkInput struct {
	Project    string   `json:"project,omitempty" jsonschema:"optional project name to filter by"`
	Labels     []string `json:"labels,omitempty" jsonschema:"optional label filters"`
	Priority   string   `json:"priority,omitempty" jsonschema:"optional priority filter (critical, high, medium, low)"`
	Complexity string   `json:"complexity,omitempty" jsonschema:"optional complexity filter (high, medium, low)"`
	WorkerID   string   `json:"worker_id,omitempty" jsonschema:"optional worker identity (defaults to interactive:<timestamp>)"`
}

// completeIssueInput is the typed input for the complete_issue tool.
type completeIssueInput struct {
	Number   int    `json:"number" jsonschema:"required,the issue number to complete"`
	PRNumber int    `json:"pr_number,omitempty" jsonschema:"optional PR number to link"`
	Summary  string `json:"summary,omitempty" jsonschema:"optional completion summary"`
	WorkerID string `json:"worker_id,omitempty" jsonschema:"optional worker identity"`
}

// releaseIssueInput is the typed input for the release_issue tool.
type releaseIssueInput struct {
	Number   int    `json:"number" jsonschema:"required,the issue number to release"`
	Reason   string `json:"reason" jsonschema:"required,why the issue is being released"`
	WorkerID string `json:"worker_id,omitempty" jsonschema:"optional worker identity"`
}

// heartbeatIssueInput is the typed input for the heartbeat_issue tool.
type heartbeatIssueInput struct {
	Number   int    `json:"number" jsonschema:"required,the issue number to heartbeat"`
	WorkerID string `json:"worker_id,omitempty" jsonschema:"optional worker identity"`
}

// registerWorkerTools adds work checkout/checkin tools to the MCP server.
func registerWorkerTools(srv *gosdk.Server, h *Handler) {
	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "claim_issue",
		Description: "Claim a specific issue for processing. Marks it as claimed and starts heartbeat tracking.",
	}, h.handleClaimIssue)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "request_work",
		Description: "Ask the dispatcher for the next best queued issue matching optional filters. Claims it automatically.",
	}, h.handleRequestWork)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "complete_issue",
		Description: "Mark a claimed issue as done. Transitions to needs-qc and posts completion summary.",
	}, h.handleCompleteIssue)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "release_issue",
		Description: "Return a claimed issue to the queue without completing it. Posts reason as comment.",
	}, h.handleReleaseIssue)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "heartbeat_issue",
		Description: "Keep a claim alive during long sessions. Resets the heartbeat timer.",
	}, h.handleHeartbeatIssue)
}

// handleClaimIssue claims a specific issue for the calling worker.
func (h *Handler) handleClaimIssue(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input claimIssueInput,
) (*gosdk.CallToolResult, any, error) {
	if input.Number <= 0 {
		return nil, nil, fmt.Errorf("number must be greater than 0")
	}
	if h.work == nil {
		return nil, nil, fmt.Errorf("work coordination not available (dispatcher not connected)")
	}

	workerID := input.WorkerID
	if workerID == "" {
		workerID = defaultWorkerID()
	}

	// Resolve owner/repo from the active project.
	owner, repo, err := h.activeOwnerRepo()
	if err != nil {
		return nil, nil, err
	}

	if result := h.checkTier(autonomy.ActionLabelIssue, "claim_issue", func() (*gosdk.CallToolResult, error) {
		return h.executeClaimIssue(ctx, owner, repo, input.Number, workerID)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeClaimIssue(ctx, owner, repo, input.Number, workerID)
	return r, nil, err
}

func (h *Handler) executeClaimIssue(ctx context.Context, owner, repo string, number int, workerID string) (*gosdk.CallToolResult, error) {
	claim, err := h.work.ClaimIssue(ctx, owner, repo, number, workerID)
	if err != nil {
		return nil, fmt.Errorf("claiming issue: %w", err)
	}

	// Enrich with project phase.
	if h.projects != nil {
		for _, p := range h.projects.List() {
			if p.Owner == owner && p.Repo == repo {
				claim.Phase = p.Phase
				break
			}
		}
	}

	// Also fetch the full issue for context.
	issue, issueErr := h.activeTracker().GetIssue(ctx, number)

	type claimResponse struct {
		*ClaimResult
		Issue any `json:"issue,omitempty"`
	}
	resp := claimResponse{ClaimResult: claim}
	if issueErr == nil {
		resp.Issue = issue
	}

	result, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		return nil, fmt.Errorf("marshalling claim result: %w", marshalErr)
	}

	h.recorder.recordToolCall(ctx, "claim_issue", number)

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil
}

// handleRequestWork asks the dispatcher for work.
func (h *Handler) handleRequestWork(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input requestWorkInput,
) (*gosdk.CallToolResult, any, error) {
	if h.work == nil {
		return nil, nil, fmt.Errorf("work coordination not available (dispatcher not connected)")
	}

	workerID := input.WorkerID
	if workerID == "" {
		workerID = defaultWorkerID()
	}

	opts := &WorkRequestOptions{
		Project:    input.Project,
		Labels:     input.Labels,
		Priority:   input.Priority,
		Complexity: input.Complexity,
	}

	claim, issue, err := h.work.RequestWork(ctx, workerID, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("requesting work: %w", err)
	}

	// Enrich with project phase.
	if h.projects != nil {
		for _, p := range h.projects.List() {
			if p.Owner == claim.Owner && p.Repo == claim.Repo {
				claim.Phase = p.Phase
				break
			}
		}
	}

	type workResponse struct {
		*ClaimResult
		Issue any `json:"issue"`
	}
	resp := workResponse{ClaimResult: claim, Issue: issue}

	result, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		return nil, nil, fmt.Errorf("marshalling work result: %w", marshalErr)
	}

	h.recorder.recordToolCall(ctx, "request_work", claim.IssueNumber)

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleCompleteIssue marks a claimed issue as done.
func (h *Handler) handleCompleteIssue(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input completeIssueInput,
) (*gosdk.CallToolResult, any, error) {
	if input.Number <= 0 {
		return nil, nil, fmt.Errorf("number must be greater than 0")
	}
	if h.work == nil {
		return nil, nil, fmt.Errorf("work coordination not available (dispatcher not connected)")
	}

	workerID := input.WorkerID
	if workerID == "" {
		workerID = defaultWorkerID()
	}

	owner, repo, err := h.activeOwnerRepo()
	if err != nil {
		return nil, nil, err
	}

	if result := h.checkTier(autonomy.ActionLabelIssue, "complete_issue", func() (*gosdk.CallToolResult, error) {
		return h.executeCompleteIssue(ctx, owner, repo, input, workerID)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeCompleteIssue(ctx, owner, repo, input, workerID)
	return r, nil, err
}

func (h *Handler) executeCompleteIssue(ctx context.Context, owner, repo string, input completeIssueInput, workerID string) (*gosdk.CallToolResult, error) {
	if err := h.work.CompleteIssue(ctx, owner, repo, input.Number, workerID, input.PRNumber, input.Summary); err != nil {
		return nil, fmt.Errorf("completing issue: %w", err)
	}

	h.recorder.recordToolCall(ctx, "complete_issue", input.Number)

	result, _ := json.Marshal(map[string]any{
		"status":       "completed",
		"issue_number": input.Number,
		"pr_number":    input.PRNumber,
	})

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil
}

// handleReleaseIssue returns a claimed issue to the queue.
func (h *Handler) handleReleaseIssue(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input releaseIssueInput,
) (*gosdk.CallToolResult, any, error) {
	if input.Number <= 0 {
		return nil, nil, fmt.Errorf("number must be greater than 0")
	}
	if input.Reason == "" {
		return nil, nil, fmt.Errorf("reason must not be empty")
	}
	if h.work == nil {
		return nil, nil, fmt.Errorf("work coordination not available (dispatcher not connected)")
	}

	workerID := input.WorkerID
	if workerID == "" {
		workerID = defaultWorkerID()
	}

	owner, repo, err := h.activeOwnerRepo()
	if err != nil {
		return nil, nil, err
	}

	if result := h.checkTier(autonomy.ActionLabelIssue, "release_issue", func() (*gosdk.CallToolResult, error) {
		return h.executeReleaseIssue(ctx, owner, repo, input, workerID)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeReleaseIssue(ctx, owner, repo, input, workerID)
	return r, nil, err
}

func (h *Handler) executeReleaseIssue(ctx context.Context, owner, repo string, input releaseIssueInput, workerID string) (*gosdk.CallToolResult, error) {
	if err := h.work.ReleaseIssue(ctx, owner, repo, input.Number, workerID, input.Reason); err != nil {
		return nil, fmt.Errorf("releasing issue: %w", err)
	}

	h.recorder.recordToolCall(ctx, "release_issue", input.Number)

	result, _ := json.Marshal(map[string]any{
		"status":       "released",
		"issue_number": input.Number,
		"reason":       input.Reason,
	})

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil
}

// handleHeartbeatIssue resets the heartbeat timer for a claimed issue.
func (h *Handler) handleHeartbeatIssue(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input heartbeatIssueInput,
) (*gosdk.CallToolResult, any, error) {
	if input.Number <= 0 {
		return nil, nil, fmt.Errorf("number must be greater than 0")
	}
	if h.work == nil {
		return nil, nil, fmt.Errorf("work coordination not available (dispatcher not connected)")
	}

	workerID := input.WorkerID
	if workerID == "" {
		workerID = defaultWorkerID()
	}

	owner, repo, err := h.activeOwnerRepo()
	if err != nil {
		return nil, nil, err
	}

	hb, err := h.work.HeartbeatIssue(ctx, owner, repo, input.Number, workerID)
	if err != nil {
		return nil, nil, fmt.Errorf("heartbeat: %w", err)
	}

	result, _ := json.Marshal(hb)

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}
