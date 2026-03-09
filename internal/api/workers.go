package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// workerStaleThreshold is the maximum time between heartbeats before a worker
// is considered offline.
const workerStaleThreshold = 5 * time.Minute

// WorkerStatus represents the operational state of a PC agent worker.
type WorkerStatus string

const (
	WorkerStatusIdle    WorkerStatus = "idle"
	WorkerStatusBusy    WorkerStatus = "busy"
	WorkerStatusOffline WorkerStatus = "offline"
)

// WorkerRecord holds registration and heartbeat state for a single PC agent.
type WorkerRecord struct {
	AgentID         string       `json:"agent_id"`
	Hostname        string       `json:"hostname"`
	Capabilities    []string     `json:"capabilities"`
	MaxConcurrent   int          `json:"max_concurrent"`
	WorkspaceRoot   string       `json:"workspace_root"`
	Status          WorkerStatus `json:"status"`
	CurrentTask     *int         `json:"current_task,omitempty"`
	CPUPercent      float64      `json:"cpu_percent"`
	MemoryPercent   float64      `json:"memory_percent"`
	ActiveWorktrees int          `json:"active_worktrees"`
	RegisteredAt    time.Time    `json:"registered_at"`
	LastHeartbeat   time.Time    `json:"last_heartbeat"`
}

// workerRegistry stores registered PC agent workers in memory.
type workerRegistry struct {
	mu      sync.RWMutex
	workers map[string]*WorkerRecord
}

func newWorkerRegistry() *workerRegistry {
	return &workerRegistry{
		workers: make(map[string]*WorkerRecord),
	}
}

// register adds or refreshes a worker record.
func (r *workerRegistry) register(req registerRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if existing, ok := r.workers[req.AgentID]; ok {
		existing.Hostname = req.Hostname
		existing.Capabilities = req.Capabilities
		existing.MaxConcurrent = req.MaxConcurrent
		existing.WorkspaceRoot = req.WorkspaceRoot
		existing.Status = WorkerStatusIdle
		existing.LastHeartbeat = now
		return
	}
	r.workers[req.AgentID] = &WorkerRecord{
		AgentID:       req.AgentID,
		Hostname:      req.Hostname,
		Capabilities:  req.Capabilities,
		MaxConcurrent: req.MaxConcurrent,
		WorkspaceRoot: req.WorkspaceRoot,
		Status:        WorkerStatusIdle,
		RegisteredAt:  now,
		LastHeartbeat: now,
	}
}

// heartbeat updates an existing worker's live metrics.
// Returns false if the agent_id is unknown.
func (r *workerRegistry) heartbeat(req heartbeatRequest) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.workers[req.AgentID]
	if !ok {
		return false
	}
	rec.Status = req.Status
	rec.CurrentTask = req.CurrentTask
	rec.CPUPercent = req.CPUPercent
	rec.MemoryPercent = req.MemoryPercent
	rec.ActiveWorktrees = req.ActiveWorktrees
	rec.LastHeartbeat = time.Now()
	return true
}

// list returns a snapshot of all workers. Workers that have not sent a heartbeat
// within workerStaleThreshold are reported as offline.
func (r *workerRegistry) list() []WorkerRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]WorkerRecord, 0, len(r.workers))
	cutoff := time.Now().Add(-workerStaleThreshold)
	for _, rec := range r.workers {
		snap := *rec
		if rec.LastHeartbeat.Before(cutoff) {
			snap.Status = WorkerStatusOffline
		}
		result = append(result, snap)
	}
	return result
}

// --- Request types ---

type registerRequest struct {
	AgentID       string   `json:"agent_id"`
	Hostname      string   `json:"hostname"`
	Capabilities  []string `json:"capabilities"`
	MaxConcurrent int      `json:"max_concurrent"`
	WorkspaceRoot string   `json:"workspace_root"`
}

type heartbeatRequest struct {
	AgentID         string       `json:"agent_id"`
	Status          WorkerStatus `json:"status"`
	CurrentTask     *int         `json:"current_task"`
	CPUPercent      float64      `json:"cpu_percent"`
	MemoryPercent   float64      `json:"memory_percent"`
	ActiveWorktrees int          `json:"active_worktrees"`
}

// --- HTTP handlers ---

// ListWorkers returns a snapshot of all registered workers for external consumers
// (e.g. the MCP digest). Stale workers are marked offline in the returned slice.
func (a *API) ListWorkers() []WorkerRecord {
	return a.workers.list()
}

// handleRegisterWorker handles POST /api/v1/workers/register.
func (a *API) handleRegisterWorker(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}
	a.workers.register(req)
	writeJSON(w, http.StatusOK, map[string]string{"status": "registered"})
}

// handleWorkerHeartbeat handles POST /api/v1/workers/heartbeat.
// Unknown agents are auto-registered to handle restart scenarios where
// the server restarted since the agent last registered.
func (a *API) handleWorkerHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req heartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}
	if !a.workers.heartbeat(req) {
		// Auto-register: server may have restarted since the agent last registered.
		a.workers.register(registerRequest{AgentID: req.AgentID, MaxConcurrent: 1})
		a.workers.heartbeat(req)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleListWorkers handles GET /api/v1/workers.
func (a *API) handleListWorkers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.workers.list())
}
