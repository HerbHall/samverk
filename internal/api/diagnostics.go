package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"samverk.dev/samverk/internal/version"
)

// diagnosticsResponse is the top-level response for GET /api/v1/diagnostics.
type diagnosticsResponse struct {
	Timestamp  string           `json:"timestamp"`
	Server     serverDiag       `json:"server"`
	Subsystems []subsystemDiag  `json:"subsystems"`
	Providers  []providerDiag   `json:"providers,omitempty"`
	Workers    []workerDiag     `json:"workers,omitempty"`
	MCP        mcpEndpointsDiag `json:"mcp_endpoints"`
	Failures   failuresDiag     `json:"recent_failures,omitempty"`
}

type serverDiag struct {
	Version          string  `json:"version"`
	GitCommit        string  `json:"git_commit"`
	UptimeS          float64 `json:"uptime_s"`
	DispatcherPaused bool    `json:"dispatcher_paused"`
	PoolWorkers      int     `json:"pool_workers"`
	PoolActive       int     `json:"pool_active"`
	QueueDepth       int     `json:"queue_depth"`
}

type subsystemDiag struct {
	Name       string `json:"name"`
	Healthy    bool   `json:"healthy"`
	LastActive string `json:"last_active,omitempty"`
	PollCount  int64  `json:"poll_count,omitempty"`
	Errors     int64  `json:"errors,omitempty"`
}

type providerDiag struct {
	Name        string `json:"name"`
	Healthy     bool   `json:"healthy"`
	ModelLoaded bool   `json:"model_loaded,omitempty"`
	GPUEnabled  bool   `json:"gpu_enabled,omitempty"`
	GPUDegraded bool   `json:"gpu_degraded,omitempty"`
	LastChecked string `json:"last_checked,omitempty"`
	Error       string `json:"error,omitempty"`
}

type workerDiag struct {
	AgentID       string  `json:"agent_id"`
	Status        string  `json:"status"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	LastHeartbeat string  `json:"last_heartbeat"`
	Stale         bool    `json:"stale"`
}

type mcpEndpointsDiag struct {
	Samverk  endpointDiag `json:"samverk"`
	Synapset endpointDiag `json:"synapset"`
}

type endpointDiag struct {
	URL       string `json:"url"`
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
}

type failuresDiag struct {
	Last24h    int                `json:"last_24h"`
	ByClass    map[string]int     `json:"by_class,omitempty"`
	TopLooping []loopingIssueDiag `json:"top_looping,omitempty"`
}

type loopingIssueDiag struct {
	IssueNumber int `json:"issue_number"`
	Failures    int `json:"failures"`
}

// CollectDiagnostics returns a full diagnostic snapshot as a struct.
// Implements the diagnosticsSource interface used by the MCP handler.
func (a *API) CollectDiagnostics(r *http.Request) any {
	return diagnosticsResponse{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Server:     a.collectServerDiag(),
		Subsystems: a.collectSubsystemDiag(),
		Providers:  a.collectProviderDiag(),
		Workers:    a.collectWorkerDiag(),
		MCP:        a.collectMCPDiag(r),
		Failures:   a.collectFailureDiag(r.Context()),
	}
}

// handleDiagnostics handles GET /api/v1/diagnostics.
func (a *API) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.CollectDiagnostics(r))
}

func (a *API) collectServerDiag() serverDiag {
	diag := serverDiag{
		Version:   version.Version,
		GitCommit: version.GitCommit,
	}

	if a.subsystems != nil {
		diag.UptimeS = a.subsystems.Uptime().Seconds()
	}

	if a.pool != nil {
		snap := a.pool.Snapshot()
		diag.PoolWorkers = snap.TotalWorkers
		diag.PoolActive = snap.ActiveWorkers
		diag.QueueDepth = snap.QueueDepth
	}

	if a.store != nil {
		ctrl, err := a.store.GetScalingControl(context.Background())
		if err == nil && ctrl != nil {
			diag.DispatcherPaused = ctrl.Paused
		}
	}

	return diag
}

func (a *API) collectSubsystemDiag() []subsystemDiag {
	if a.subsystems == nil {
		return []subsystemDiag{
			{Name: "subsystem_registry", Healthy: false},
		}
	}

	statuses := a.subsystems.All()
	result := make([]subsystemDiag, 0, len(statuses)+4)

	for i := range statuses {
		s := &statuses[i]
		result = append(result, subsystemDiag{
			Name:       s.Name,
			Healthy:    s.Running,
			LastActive: s.LastActive.UTC().Format(time.RFC3339),
			PollCount:  s.PollCount,
			Errors:     s.Errors,
		})
	}

	// Core dependency wiring checks
	result = append(result,
		subsystemDiag{Name: "store", Healthy: a.store != nil},
		subsystemDiag{Name: "tracker", Healthy: a.tracker != nil},
		subsystemDiag{Name: "cost_tracking", Healthy: a.costs != nil},
		subsystemDiag{Name: "provider_registry", Healthy: a.providerRegistry != nil},
	)

	return result
}

func (a *API) collectProviderDiag() []providerDiag {
	if a.healthMonitor == nil {
		return nil
	}

	allHealth := a.healthMonitor.AllHealth()
	result := make([]providerDiag, 0, len(allHealth))
	for i := range allHealth {
		ph := &allHealth[i]
		entry := providerDiag{
			Name:        ph.Name,
			Healthy:     ph.Healthy,
			ModelLoaded: ph.ModelLoaded,
			GPUEnabled:  ph.GPUEnabled,
			GPUDegraded: ph.GPUDegraded,
			Error:       ph.Error,
		}
		if !ph.LastChecked.IsZero() {
			entry.LastChecked = ph.LastChecked.UTC().Format(time.RFC3339)
		}
		result = append(result, entry)
	}
	return result
}

func (a *API) collectWorkerDiag() []workerDiag {
	workers := a.workers.list()
	if len(workers) == 0 {
		return nil
	}

	cutoff := time.Now().Add(-workerStaleThreshold)
	result := make([]workerDiag, 0, len(workers))
	for i := range workers {
		w := &workers[i]
		result = append(result, workerDiag{
			AgentID:       w.AgentID,
			Status:        string(w.Status),
			CPUPercent:    w.CPUPercent,
			MemoryPercent: w.MemoryPercent,
			LastHeartbeat: w.LastHeartbeat.UTC().Format(time.RFC3339),
			Stale:         w.LastHeartbeat.Before(cutoff),
		})
	}
	return result
}

func (a *API) collectMCPDiag(r *http.Request) mcpEndpointsDiag {
	diag := mcpEndpointsDiag{
		Samverk: endpointDiag{
			URL:       "/mcp",
			Reachable: a.tracker != nil && a.store != nil,
		},
	}
	if !diag.Samverk.Reachable {
		diag.Samverk.Error = "tracker or store not wired"
	}

	if a.synapsetProxy != nil {
		diag.Synapset = a.probeSynapset(r)
	} else {
		diag.Synapset = endpointDiag{
			URL:   "not configured",
			Error: "synapset proxy not configured",
		}
	}

	return diag
}

func (a *API) probeSynapset(r *http.Request) endpointDiag {
	ep := endpointDiag{
		URL: a.synapsetProxy.baseURL + "/mcp",
	}

	client := &http.Client{Timeout: 3 * time.Second}
	statusURL := a.synapsetProxy.baseURL + "/api/status"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, statusURL, http.NoBody) //nolint:gosec // G704: URL is from trusted baseURL config
	if err != nil {
		ep.Error = fmt.Sprintf("request build: %v", err)
		return ep
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req) //nolint:gosec // G107: URL is built from configured baseURL, not user input
	if err != nil {
		ep.Error = fmt.Sprintf("connection: %v", err)
		return ep
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		ep.Reachable = true
	} else {
		ep.Error = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return ep
}

func (a *API) collectFailureDiag(ctx context.Context) failuresDiag {
	if a.store == nil {
		return failuresDiag{}
	}

	since := time.Now().Add(-24 * time.Hour)
	summary, err := a.store.GetFailureSummary(ctx, since)
	if err != nil {
		return failuresDiag{}
	}

	diag := failuresDiag{
		Last24h: summary.TotalFailures,
	}

	if len(summary.ByClass) > 0 {
		diag.ByClass = make(map[string]int, len(summary.ByClass))
		for class, count := range summary.ByClass {
			diag.ByClass[string(class)] = count
		}
	}

	if len(summary.LoopingIssues) > 0 {
		diag.TopLooping = make([]loopingIssueDiag, 0, len(summary.LoopingIssues))
		for i := range summary.LoopingIssues {
			li := &summary.LoopingIssues[i]
			diag.TopLooping = append(diag.TopLooping, loopingIssueDiag{
				IssueNumber: li.IssueNumber,
				Failures:    li.Count,
			})
		}
	}

	return diag
}
