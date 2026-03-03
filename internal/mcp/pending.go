package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// PendingAction represents an MCP tool call queued for user confirmation (Tier 3).
type PendingAction struct {
	ID        string
	ToolName  string
	Execute   func() (*gosdk.CallToolResult, error) // closure captures handler + input
	CreatedAt time.Time
}

// pendingActions is a thread-safe queue for Tier 3 actions awaiting confirmation.
type pendingActions struct {
	mu      sync.Mutex
	actions map[string]*PendingAction
	ttl     time.Duration
}

// newPendingActions creates a pendingActions queue with a 1-hour default TTL.
func newPendingActions() *pendingActions {
	return &pendingActions{
		actions: make(map[string]*PendingAction),
		ttl:     time.Hour,
	}
}

// add queues a tool call for user confirmation and returns the action ID.
func (p *pendingActions) add(toolName string, execute func() (*gosdk.CallToolResult, error)) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := generateActionID()
	p.actions[id] = &PendingAction{
		ID:        id,
		ToolName:  toolName,
		Execute:   execute,
		CreatedAt: time.Now(),
	}
	return id
}

// get retrieves a pending action by ID. Returns nil, false if not found or expired.
func (p *pendingActions) get(id string) (*PendingAction, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	action, ok := p.actions[id]
	if !ok {
		return nil, false
	}
	if time.Since(action.CreatedAt) > p.ttl {
		delete(p.actions, id)
		return nil, false
	}
	return action, true
}

// remove deletes a pending action by ID.
func (p *pendingActions) remove(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.actions, id)
}

// generateActionID creates a random hex ID for pending actions.
func generateActionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
