package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/herbhall/samverk/internal/logstore"
)

// listWorkersInput is the typed input for the list_workers tool.
type listWorkersInput struct{}

// getSessionLogInput is the typed input for the get_session_log tool.
type getSessionLogInput struct {
	IssueNumber int `json:"issue_number" jsonschema:"required,the issue number to get logs for"`
	LastNLines  int `json:"last_n_lines,omitempty" jsonschema:"number of log lines to return (default: 50, max: 500)"`
}

// getProviderHealthInput is the typed input for the get_provider_health tool.
type getProviderHealthInput struct{}

// registerObservabilityTools adds agent observability tools to the MCP server.
func registerObservabilityTools(srv *gosdk.Server, h *Handler) {
	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "list_workers",
		Description: "List all currently active worker sessions with live status (worker_id, issue, provider, status, elapsed time)",
	}, h.handleListWorkers)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "get_session_log",
		Description: "Get recent log output for the agent session working on a given issue",
	}, h.handleGetSessionLog)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "get_provider_health",
		Description: "Get current health status of all configured AI providers (healthy, last_checked, last_error, VRAM)",
	}, h.handleGetProviderHealth)
}

// handleListWorkers returns all currently active worker sessions.
func (h *Handler) handleListWorkers(
	_ context.Context,
	_ *gosdk.CallToolRequest,
	_ listWorkersInput,
) (*gosdk.CallToolResult, any, error) {
	if h.poolM == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "worker pool not available (pool not running in this process)"},
			},
		}, nil, nil
	}

	snap := h.poolM.Snapshot()

	// Build a summary from pool snapshot and worker lister if available.
	type workerSummary struct {
		TotalWorkers  int    `json:"total_workers"`
		ActiveWorkers int    `json:"active_workers"`
		IdleWorkers   int    `json:"idle_workers"`
		QueueDepth    int    `json:"queue_depth"`
		Workers       []any  `json:"workers,omitempty"`
	}

	summary := workerSummary{
		TotalWorkers:  snap.TotalWorkers,
		ActiveWorkers: snap.ActiveWorkers,
		IdleWorkers:   snap.IdleWorkers,
		QueueDepth:    snap.QueueDepth,
	}

	// Add PC worker details if available.
	if h.workersM != nil {
		workers := h.workersM.ListWorkers()
		details := make([]any, 0, len(workers))
		for i := range workers {
			w := &workers[i]
			entry := map[string]any{
				"agent_id": w.AgentID,
				"hostname": w.Hostname,
				"status":   w.Status,
			}
			if w.CurrentTask != nil {
				entry["current_issue"] = *w.CurrentTask
			}
			entry["active_worktrees"] = w.ActiveWorktrees
			entry["cpu_percent"] = w.CPUPercent
			entry["memory_percent"] = w.MemoryPercent
			details = append(details, entry)
		}
		summary.Workers = details
	}

	result, err := json.Marshal(summary)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling workers: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleGetSessionLog returns recent log output for an agent session by issue number.
func (h *Handler) handleGetSessionLog(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input getSessionLogInput,
) (*gosdk.CallToolResult, any, error) {
	if h.logs == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "session logs not available: no log store configured"},
			},
		}, nil, nil
	}

	if input.IssueNumber <= 0 {
		return nil, nil, fmt.Errorf("issue_number must be greater than 0")
	}

	limit := input.LastNLines
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	entries, err := h.logs.Query(ctx, logstore.QueryFilter{
		Issue: input.IssueNumber,
		Limit: limit,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("querying logs: %w", err)
	}

	if len(entries) == 0 {
		text := fmt.Sprintf("No log entries found for issue #%d", input.IssueNumber)
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{&gosdk.TextContent{Text: text}},
		}, nil, nil
	}

	// Return entries in chronological order (query returns DESC).
	reversed := make([]logstore.LogEntry, len(entries))
	for i, e := range entries {
		reversed[len(entries)-1-i] = e
	}

	type logLine struct {
		Timestamp string `json:"ts"`
		Level     string `json:"level"`
		Message   string `json:"msg"`
		Component string `json:"component,omitempty"`
		SessionID string `json:"session_id,omitempty"`
	}

	lines := make([]logLine, 0, len(reversed))
	for i := range reversed {
		e := &reversed[i]
		lines = append(lines, logLine{
			Timestamp: e.Timestamp.UTC().Format(time.RFC3339),
			Level:     e.Level,
			Message:   e.Message,
			Component: e.Component,
			SessionID: e.SessionID,
		})
	}

	result, marshalErr := json.Marshal(map[string]any{
		"issue_number": input.IssueNumber,
		"count":        len(lines),
		"lines":        lines,
	})
	if marshalErr != nil {
		return nil, nil, fmt.Errorf("marshalling logs: %w", marshalErr)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleGetProviderHealth returns health status of all configured providers.
func (h *Handler) handleGetProviderHealth(
	_ context.Context,
	_ *gosdk.CallToolRequest,
	_ getProviderHealthInput,
) (*gosdk.CallToolResult, any, error) {
	if h.healthM == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "provider health not available: no health monitor configured"},
			},
		}, nil, nil
	}

	allHealth := h.healthM.AllHealth()

	type healthEntry struct {
		Name        string `json:"name"`
		Healthy     bool   `json:"healthy"`
		LastChecked string `json:"last_checked,omitempty"`
		LastHealthy string `json:"last_healthy,omitempty"`
		Error       string `json:"error,omitempty"`
		ModelLoaded bool   `json:"model_loaded,omitempty"`
		VRAMFree    int64  `json:"vram_free_bytes,omitempty"`
		VRAMTotal   int64  `json:"vram_total_bytes,omitempty"`
	}

	entries := make([]healthEntry, 0, len(allHealth))
	for i := range allHealth {
		ph := &allHealth[i]
		entry := healthEntry{
			Name:        ph.Name,
			Healthy:     ph.Healthy,
			Error:       ph.Error,
			ModelLoaded: ph.ModelLoaded,
			VRAMFree:    ph.VRAMFree,
			VRAMTotal:   ph.VRAMTotal,
		}
		if !ph.LastChecked.IsZero() {
			entry.LastChecked = ph.LastChecked.UTC().Format(time.RFC3339)
		}
		if !ph.LastHealthy.IsZero() {
			entry.LastHealthy = ph.LastHealthy.UTC().Format(time.RFC3339)
		}
		entries = append(entries, entry)
	}

	result, err := json.Marshal(map[string]any{
		"providers": entries,
		"count":     len(entries),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling provider health: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}
