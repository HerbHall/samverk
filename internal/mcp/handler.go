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
	tracker   forge.IssueTracker          // required
	costs     digest.CostSource           // may be nil
	store     store.Store                 // may be nil
	recorder  *sessionRecorder            // derived from store; nil when store is nil
	policy    autonomy.AutonomyPolicy     // may be nil (no enforcement)
	pending   *pendingActions              // Tier 3 action queue
	repo      forge.RepoReader            // may be nil (no repo browsing)
	prManager forge.PullRequestManager    // may be nil (no PR operations)
	projects  *ProjectRegistry            // may be nil (single-project mode)
	poolM         poolMetricsSource       // may be nil (pool not running here)
	dispM         dispatcherMetricsSource // may be nil (dispatcher not running here)
	sysM          systemMetricsSource     // may be nil
	scalingEvents scalingEventReader      // may be nil (reads from store)
}

// NewHandler creates a new MCP tool handler with its dependencies.
// The store, policy, and repo parameters are optional (may be nil) for graceful degradation.
func NewHandler(tracker forge.IssueTracker, costs digest.CostSource, s store.Store, policy autonomy.AutonomyPolicy, repo forge.RepoReader) *Handler {
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
		repo:     repo,
	}
}

// SetPRManager attaches a PullRequestManager to the handler.
func (h *Handler) SetPRManager(prm forge.PullRequestManager) {
	h.prManager = prm
}

// SetProjects attaches a project registry to the handler for multi-project support.
// When set, tools resolve tracker and repo through the active project.
func (h *Handler) SetProjects(reg *ProjectRegistry) {
	h.projects = reg
}

// activeTracker returns the IssueTracker to use for the current context.
// If a project registry is configured, it uses the active project's tracker.
// Otherwise, it falls back to the handler's directly-configured tracker.
func (h *Handler) activeTracker() forge.IssueTracker {
	if h.projects != nil {
		p, err := h.projects.Active()
		if err == nil && p.Tracker != nil {
			return p.Tracker
		}
	}
	return h.tracker
}

// activeReader returns the RepoReader to use for the current context.
// If a project registry is configured, it uses the active project's reader.
// Otherwise, it falls back to the handler's directly-configured reader.
func (h *Handler) activeReader() forge.RepoReader {
	if h.projects != nil {
		p, err := h.projects.Active()
		if err == nil && p.Reader != nil {
			return p.Reader
		}
	}
	return h.repo
}

// activePRManager returns the PullRequestManager to use for the current context.
// If a project registry is configured, it uses the active project's PRManager.
// Otherwise, it falls back to the handler's directly-configured prManager.
func (h *Handler) activePRManager() forge.PullRequestManager {
	if h.projects != nil {
		p, err := h.projects.Active()
		if err == nil && p.PRManager != nil {
			return p.PRManager
		}
	}
	return h.prManager
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
	registerRepoTools(srv, h)
	registerProjectTools(srv, h)
	registerPRTools(srv, h)
	registerScalingTools(srv, h)
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
