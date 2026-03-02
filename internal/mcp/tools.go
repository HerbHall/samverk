package mcp

import (
	"context"
	"fmt"
	"time"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/herbhall/samverk/internal/digest"
)

// getDigestInput is the typed input for the get_digest tool.
type getDigestInput struct {
	Since string `json:"since" jsonschema:"duration like 24h or 168h for the lookback window"`
}

// getCostSummaryInput is the typed input for the get_cost_summary tool.
type getCostSummaryInput struct {
	Since string `json:"since" jsonschema:"duration like 24h or 168h for the lookback window"`
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
