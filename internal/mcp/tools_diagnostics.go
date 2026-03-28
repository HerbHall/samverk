package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/herbhall/samverk/internal/version"
)

// diagnosticsInput is the typed input for the get_diagnostics tool.
type diagnosticsInput struct{}

// diagnosticsSource provides a full system diagnostic snapshot.
// Implemented by the API layer which has access to all subsystems.
type diagnosticsSource interface {
	CollectDiagnostics(r *http.Request) any
}

// SetDiagnosticsSource attaches a diagnostics source for the get_diagnostics tool.
func (h *Handler) SetDiagnosticsSource(ds diagnosticsSource) {
	h.diagnostics = ds
}

// registerDiagnosticsTools adds the get_diagnostics MCP tool to the server.
func registerDiagnosticsTools(srv *gosdk.Server, h *Handler) {
	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "get_diagnostics",
		Description: "Get full system diagnostics: server status, subsystem health, provider connectivity, worker status, MCP endpoint reachability, and recent failure summary. Use this to verify the entire toolchain is connected and working.",
	}, h.handleGetDiagnostics)
}

// handleGetDiagnostics returns a comprehensive system diagnostic snapshot.
func (h *Handler) handleGetDiagnostics(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	_ diagnosticsInput,
) (*gosdk.CallToolResult, any, error) {
	h.touchCheckIn(ctx)

	if h.diagnostics != nil {
		// Delegate to the API layer which has full subsystem access.
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/diagnostics", http.NoBody)
		data := h.diagnostics.CollectDiagnostics(req)
		result, err := json.Marshal(data)
		if err != nil {
			return nil, nil, fmt.Errorf("marshalling diagnostics: %w", err)
		}
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{&gosdk.TextContent{Text: string(result)}},
		}, nil, nil
	}

	// Fallback: minimal diagnostics from handler-local dependencies.
	diag := h.minimalDiagnostics(ctx)
	result, err := json.Marshal(diag)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling diagnostics: %w", err)
	}
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{&gosdk.TextContent{Text: string(result)}},
	}, nil, nil
}

// minimalDiagnostics builds a diagnostic snapshot from handler-local deps only.
// Used when no API diagnosticsSource is wired.
func (h *Handler) minimalDiagnostics(ctx context.Context) map[string]any {
	diag := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"server": map[string]any{
			"version":    version.Version,
			"git_commit": version.GitCommit,
		},
	}

	// Subsystem wiring
	wiring := map[string]bool{
		"store":              h.store != nil,
		"tracker":            h.tracker != nil,
		"costs":              h.costs != nil,
		"work_coordinator":   h.work != nil,
		"provider_registry":  h.provReg != nil,
		"health_monitor":     h.healthM != nil,
		"logs":               h.logs != nil,
		"workers":            h.workersM != nil,
		"diagnostics_source": h.diagnostics != nil,
	}
	diag["subsystem_wiring"] = wiring

	// Provider health if available
	if h.healthM != nil {
		allHealth := h.healthM.AllHealth()
		providers := make([]map[string]any, 0, len(allHealth))
		for i := range allHealth {
			ph := &allHealth[i]
			entry := map[string]any{
				"name":         ph.Name,
				"healthy":      ph.Healthy,
				"model_loaded": ph.ModelLoaded,
			}
			if ph.Error != "" {
				entry["error"] = ph.Error
			}
			providers = append(providers, entry)
		}
		diag["providers"] = providers
	}

	// Pool metrics if available
	if h.poolM != nil {
		snap := h.poolM.Snapshot()
		diag["pool"] = map[string]any{
			"total_workers":  snap.TotalWorkers,
			"active_workers": snap.ActiveWorkers,
			"idle_workers":   snap.IdleWorkers,
			"queue_depth":    snap.QueueDepth,
		}
	}

	// Dispatcher paused state
	if h.store != nil {
		ctrl, err := h.store.GetScalingControl(ctx)
		if err == nil && ctrl != nil {
			diag["dispatcher_paused"] = ctrl.Paused
		}
	}

	// Worker status
	if h.workersM != nil {
		workers := h.workersM.ListWorkers()
		workerList := make([]map[string]any, 0, len(workers))
		for i := range workers {
			w := &workers[i]
			workerList = append(workerList, map[string]any{
				"agent_id":       w.AgentID,
				"status":         w.Status,
				"cpu_percent":    w.CPUPercent,
				"memory_percent": w.MemoryPercent,
			})
		}
		diag["workers"] = workerList
	}

	return diag
}
