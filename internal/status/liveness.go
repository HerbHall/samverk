// Package status provides runtime status monitoring for subsystems and deployments.
package status

import (
	"sync"
	"time"
)

// SubsystemStatus represents the runtime status of a background subsystem.
type SubsystemStatus struct {
	Name       string    `json:"name"`
	Running    bool      `json:"running"`
	LastActive time.Time `json:"last_active"`
	PollCount  int64     `json:"poll_count"`
	Errors     int64     `json:"errors"`
}

// Registry tracks the status of all background subsystems.
// It is thread-safe and can be shared across goroutines.
type Registry struct {
	mu        sync.RWMutex
	started   time.Time
	subsystem map[string]*SubsystemStatus
}

// NewRegistry creates a new subsystem registry.
func NewRegistry() *Registry {
	return &Registry{
		started:   time.Now(),
		subsystem: make(map[string]*SubsystemStatus),
	}
}

// Register adds a subsystem to the registry with initial status.
// If the subsystem already exists, it updates the status.
func (r *Registry) Register(name string, running bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subsystem[name] = &SubsystemStatus{
		Name:       name,
		Running:    running,
		LastActive: time.Now(),
		PollCount:  0,
		Errors:     0,
	}
}

// RecordActivity updates the last activity timestamp for a subsystem.
// Call this regularly from a subsystem's main loop to indicate it's alive.
func (r *Registry) RecordActivity(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.subsystem[name]; ok {
		s.LastActive = time.Now()
		s.PollCount++
		s.Running = true
	}
}

// RecordError increments the error count for a subsystem.
func (r *Registry) RecordError(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.subsystem[name]; ok {
		s.Errors++
	}
}

// GetStatus returns the current status of a subsystem by name.
func (r *Registry) GetStatus(name string) *SubsystemStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.subsystem[name]; ok {
		// Return a copy to avoid data races
		cpy := *s
		return &cpy
	}
	return nil
}

// All returns a snapshot of all subsystem statuses.
func (r *Registry) All() []SubsystemStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]SubsystemStatus, 0, len(r.subsystem))
	for _, s := range r.subsystem {
		result = append(result, *s)
	}
	return result
}

// Uptime returns the duration since the registry was created (server uptime).
func (r *Registry) Uptime() time.Duration {
	return time.Since(r.started)
}
