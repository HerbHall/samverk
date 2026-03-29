package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Default thresholds for computed worker status.
const (
	defaultStaleThreshold   = 5 * time.Minute
	defaultOfflineThreshold = 15 * time.Minute
)

// workerStaleThreshold is retained for the legacy list() behavior.
const workerStaleThreshold = defaultStaleThreshold

// WorkerStatus represents the operational state of a PC agent worker.
type WorkerStatus string

const (
	WorkerStatusIdle    WorkerStatus = "idle"
	WorkerStatusBusy    WorkerStatus = "busy"
	WorkerStatusOffline WorkerStatus = "offline"
)

// ComputedWorkerStatus is the three-tier heartbeat-freshness classification.
type ComputedWorkerStatus string

const (
	ComputedWorkerStatusHealthy ComputedWorkerStatus = "healthy"
	ComputedWorkerStatusStale   ComputedWorkerStatus = "stale"
	ComputedWorkerStatusOffline ComputedWorkerStatus = "offline"
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

// workerListDTO extends WorkerRecord with a computed_status field.
type workerListDTO struct {
	WorkerRecord
	ComputedStatus ComputedWorkerStatus `json:"computed_status"`
}

// computeWorkerStatus returns the heartbeat-freshness status for a worker.
// A zero LastHeartbeat is treated as offline.
func computeWorkerStatus(lastHeartbeat time.Time, staleThreshold, offlineThreshold time.Duration) ComputedWorkerStatus {
	if lastHeartbeat.IsZero() {
		return ComputedWorkerStatusOffline
	}
	age := time.Since(lastHeartbeat)
	switch {
	case age < staleThreshold:
		return ComputedWorkerStatusHealthy
	case age < offlineThreshold:
		return ComputedWorkerStatusStale
	default:
		return ComputedWorkerStatusOffline
	}
}

// workerRegistry stores registered PC agent workers in memory.
type workerRegistry struct {
	mu               sync.RWMutex
	workers          map[string]*WorkerRecord
	staleThreshold   time.Duration
	offlineThreshold time.Duration
}

func newWorkerRegistry() *workerRegistry {
	return &workerRegistry{
		workers:          make(map[string]*WorkerRecord),
		staleThreshold:   defaultStaleThreshold,
		offlineThreshold: defaultOfflineThreshold,
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

// listWithStatus returns worker DTOs enriched with computed_status.
func (r *workerRegistry) listWithStatus() []workerListDTO {
	r.mu.RLock()
	stale := r.staleThreshold
	offline := r.offlineThreshold
	r.mu.RUnlock()

	records := r.list()
	result := make([]workerListDTO, 0, len(records))
	for i := range records {
		result = append(result, workerListDTO{
			WorkerRecord:   records[i],
			ComputedStatus: computeWorkerStatus(records[i].LastHeartbeat, stale, offline),
		})
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

// SetWorkerStaleThreshold configures the heartbeat-age thresholds used for computed_status.
// stale is the age after which a worker transitions healthy → stale.
// offline is the age after which a worker transitions stale → offline.
// Defaults are 5m and 15m respectively.
func (a *API) SetWorkerStaleThreshold(stale, offline time.Duration) {
	a.workers.mu.Lock()
	defer a.workers.mu.Unlock()
	a.workers.staleThreshold = stale
	a.workers.offlineThreshold = offline
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
// Each worker includes a computed_status field (healthy/stale/offline) derived
// from the age of the last heartbeat.
func (a *API) handleListWorkers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.workers.listWithStatus())
}

// activeWorkerResponse represents a currently running agent session.
type activeWorkerResponse struct {
	SessionID   string  `json:"session_id"`
	IssueNumber int     `json:"issue_number"`
	AgentType   string  `json:"agent_type"`
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	StartedAt   string  `json:"started_at"`
	ElapsedS    float64 `json:"elapsed_s"`
}

// handleListActiveWorkers handles GET /api/v1/workers/active.
// Returns sessions with status="active", computed elapsed_s.
func (a *API) handleListActiveWorkers(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeJSON(w, http.StatusOK, []activeWorkerResponse{})
		return
	}
	sessions, err := a.store.ListSessions(r.Context(), "active")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list active sessions")
		return
	}
	result := make([]activeWorkerResponse, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, activeWorkerResponse{
			SessionID:   s.ID,
			IssueNumber: s.IssueNumber,
			AgentType:   s.AgentType,
			Provider:    s.Provider,
			Model:       s.Model,
			StartedAt:   s.StartedAt.Format(time.RFC3339),
			ElapsedS:    time.Since(s.StartedAt).Seconds(),
		})
	}
	writeJSON(w, http.StatusOK, result)
}