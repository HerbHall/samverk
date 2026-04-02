package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"samverk.dev/samverk/pkg/models"
)

// getFailureSummaryInput is the typed input for the get_failure_summary tool.
type getFailureSummaryInput struct {
	Since string `json:"since" jsonschema:"duration like 24h or 168h for the lookback window (default: 24h)"`
}

// handleGetFailureSummary returns an aggregated failure analysis summary.
func (h *Handler) handleGetFailureSummary(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input getFailureSummaryInput,
) (*gosdk.CallToolResult, any, error) {
	if h.store == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{&gosdk.TextContent{Text: "Failure tracking not available (no store configured)"}},
		}, nil, nil
	}

	h.touchCheckIn(ctx)

	dur := 24 * time.Hour
	if input.Since != "" {
		parsed, err := time.ParseDuration(input.Since)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid since duration %q: %w", input.Since, err)
		}
		dur = parsed
	}

	since := time.Now().Add(-dur)
	summary, err := h.store.GetFailureSummary(ctx, since)
	if err != nil {
		return nil, nil, fmt.Errorf("getting failure summary: %w", err)
	}

	text := formatFailureSummary(summary, dur)

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{&gosdk.TextContent{Text: text}},
	}, nil, nil
}

// handleResetFailureCounts clears all persisted failure counts so the
// dispatcher retries previously-escalated issues.
func (h *Handler) handleResetFailureCounts(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	_ struct{},
) (*gosdk.CallToolResult, any, error) {
	if h.store == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{&gosdk.TextContent{Text: "Failure tracking not available (no store configured)"}},
		}, nil, nil
	}

	cleared, err := h.store.ResetAllFailureCounts(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("resetting failure counts: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{&gosdk.TextContent{
			Text: fmt.Sprintf("Reset %d issue failure count(s). Dispatcher will retry previously-escalated issues on next poll.", cleared),
		}},
	}, nil, nil
}

// formatFailureSummary renders the failure summary as human-readable text.
func formatFailureSummary(s *models.FailureSummary, window time.Duration) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## Failure Analysis (last %s)\n\n", window)

	if s.TotalFailures == 0 {
		b.WriteString("No failures recorded.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "**Total failures:** %d\n\n", s.TotalFailures)

	// By class.
	if len(s.ByClass) > 0 {
		b.WriteString("### By Class\n\n")
		for fc, count := range s.ByClass {
			retryable := ""
			if fc.IsRetryable() {
				retryable = " (retryable)"
			} else if fc.IsPermanent() {
				retryable = " (permanent)"
			}
			fmt.Fprintf(&b, "- **%s**: %d%s\n", fc, count, retryable)
		}
		b.WriteString("\n")
	}

	// Looping issues (5+ failures).
	if len(s.LoopingIssues) > 0 {
		b.WriteString("### Looping Issues (5+ failures)\n\n")
		for _, ifc := range s.LoopingIssues {
			fmt.Fprintf(&b, "- #%d: %d failures\n", ifc.IssueNumber, ifc.Count)
		}
		b.WriteString("\n")
	}

	// Top failing issues.
	if len(s.TopIssues) > 0 {
		b.WriteString("### Top Failing Issues\n\n")
		for _, ifc := range s.TopIssues {
			fmt.Fprintf(&b, "- #%d: %d failures\n", ifc.IssueNumber, ifc.Count)
		}
		b.WriteString("\n")
	}

	// Provider health.
	if len(s.ProviderHealth) > 0 {
		b.WriteString("### Provider Health\n\n")
		for prov, count := range s.ProviderHealth {
			fmt.Fprintf(&b, "- **%s**: %d failures\n", prov, count)
		}
		b.WriteString("\n")
	}

	return b.String()
}
