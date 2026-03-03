package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/digest"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/internal/version"
)

// Handler holds dependencies for MCP tool handlers.
type Handler struct {
	tracker  forge.IssueTracker
	costs    digest.CostSource      // may be nil
	store    store.Store             // may be nil
	recorder *sessionRecorder        // derived from store; nil when store is nil
	policy   autonomy.AutonomyPolicy // may be nil (no enforcement)
	pending  *pendingActions          // Tier 3 action queue
}

// NewHandler creates a new MCP tool handler with its dependencies.
// The store and policy parameters are optional (may be nil) for graceful degradation.
func NewHandler(tracker forge.IssueTracker, costs digest.CostSource, s store.Store, policy autonomy.AutonomyPolicy) *Handler {
	var rec *sessionRecorder
	if s != nil {
		rec = &sessionRecorder{store: s}
	}
	return &Handler{
		tracker:  tracker,
		costs:    costs,
		store:    s,
		recorder: rec,
		policy:   policy,
		pending:  newPendingActions(),
	}
}

// checkTier evaluates the autonomy tier for an action. Returns nil if the action
// should proceed, or a CallToolResult with confirmation_required if Tier 3.
func (h *Handler) checkTier(actionType autonomy.ActionType, toolName string, execute func() (*gosdk.CallToolResult, error)) *gosdk.CallToolResult {
	if h.policy == nil {
		return nil // no policy = no enforcement
	}
	tier := h.policy.TierFor(actionType)
	if tier < autonomy.Tier3 {
		return nil // Tier 1/2: proceed
	}
	// Tier 3: queue for confirmation.
	actionID := h.pending.add(toolName, execute)
	result := map[string]any{
		"status":    "confirmation_required",
		"action":    toolName,
		"tier":      3,
		"action_id": actionID,
		"reason":    fmt.Sprintf("%s requires user confirmation (Tier 3)", toolName),
	}
	resultJSON, _ := json.Marshal(result)
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{&gosdk.TextContent{Text: string(resultJSON)}},
	}
}

// newMCPServer creates the go-sdk MCP server with all tools registered.
func newMCPServer(h *Handler) *gosdk.Server {
	srv := gosdk.NewServer(
		&gosdk.Implementation{
			Name:    "samverk",
			Version: version.Version,
		},
		&gosdk.ServerOptions{},
	)

	registerTools(srv, h)
	return srv
}

// NewHTTPHandler creates an http.Handler that serves the MCP protocol
// over Streamable HTTP in stateless JSON response mode.
func NewHTTPHandler(h *Handler) http.Handler {
	mcpServer := newMCPServer(h)

	return gosdk.NewStreamableHTTPHandler(
		func(_ *http.Request) *gosdk.Server {
			return mcpServer
		},
		&gosdk.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
		},
	)
}
