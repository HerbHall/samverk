package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"samverk.dev/samverk/internal/autonomy"
	"samverk.dev/samverk/internal/digest"
	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/internal/provider"
	"samverk.dev/samverk/internal/store"
	"samverk.dev/samverk/internal/version"
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
	writer    forge.RepoWriter            // may be nil (no repo write operations)
	prManager forge.PullRequestManager    // may be nil (no PR operations)
	projects  *ProjectRegistry            // may be nil (single-project mode)
	poolM         poolMetricsSource       // may be nil (pool not running here)
	dispM         dispatcherMetricsSource // may be nil (dispatcher not running here)
	sysM          systemMetricsSource     // may be nil
	scalingEvents scalingEventReader      // may be nil (reads from store)
	workersM      workerLister            // may be nil (no PC agent workers registered)
	work          WorkCoordinator         // may be nil (dispatcher not connected)
	healthM       providerHealthSource   // may be nil (no health monitor)
	logs          logQuerier             // may be nil (no log store)
	provReg       *provider.Registry    // may be nil (no provider registry for routing chains)
	diagnostics   diagnosticsSource    // may be nil (no API diagnostics wired)
}

// NewHandler creates a new MCP tool handler with its dependencies.
// The store, policy, and repo parameters are optional (may be nil) for graceful degradation.
func NewHandler(tracker forge.IssueTracker, costs digest.CostSource, s store.Store, policy autonomy.AutonomyPolicy, repo forge.RepoReader) *Handler {
	var rec *sessionRecorder
	if s != nil {
		rec = &sessionRecorder{store: s, logger: zap.L()}
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

// touchCheckIn persists the current time as the last human check-in.
// Called from status-check tools (get_digest, get_failure_summary,
// get_project_summary, get_provider_health) so the "away" duration
// reflects the last time the user looked at the system, not just the
// last get_digest call.
func (h *Handler) touchCheckIn(ctx context.Context) {
	if h.store != nil {
		_ = h.store.SetLastCheckIn(ctx, time.Now())
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

// SetWorkerLister attaches a worker lister so the digest includes PC agent status.
func (h *Handler) SetWorkerLister(wl workerLister) {
	h.workersM = wl
}

// SetWorkCoordinator attaches a work coordinator (dispatcher) for interactive
// work checkout/checkin tools.
func (h *Handler) SetWorkCoordinator(wc WorkCoordinator) {
	h.work = wc
}

// SetWriter attaches a RepoWriter for write operations (create_branch, write_file).
func (h *Handler) SetWriter(w forge.RepoWriter) {
	h.writer = w
}

// SetProviderHealth attaches a provider health source for the get_provider_health tool.
func (h *Handler) SetProviderHealth(ph providerHealthSource) {
	h.healthM = ph
}

// SetLogQuerier attaches a log querier for the get_session_log tool.
func (h *Handler) SetLogQuerier(lq logQuerier) {
	h.logs = lq
}

// SetProviderRegistry attaches a provider registry for routing chain and last-success data.
func (h *Handler) SetProviderRegistry(reg *provider.Registry) {
	h.provReg = reg
}

// WiringGap describes an MCP handler dependency that is expected but not wired.
type WiringGap struct {
	Dependency  string // field name
	Tool        string // MCP tool that degrades without it
	Description string // what the user loses
}

// AuditWiring checks that all expected production dependencies are wired.
// Returns a list of gaps. An empty list means the handler is fully wired.
// Call this after all Set* calls during startup to surface missing wiring
// before it silently degrades a tool at runtime.
func (h *Handler) AuditWiring() []WiringGap {
	type check struct {
		ok          bool
		dep, tool   string
		description string
	}
	checks := []check{
		{h.store != nil, "store", "get_digest/get_failure_summary/scaling", "session recording, failure tracking, check-in persistence"},
		{h.costs != nil, "costs", "get_digest", "cost and token attribution in digest"},
		{h.repo != nil, "repo", "read_file/list_files/search_code", "repository browsing"},
		{h.writer != nil, "writer", "write_file/create_branch", "repository write operations"},
		{h.prManager != nil, "prManager", "create_pr/merge_pr/review_pr", "pull request operations"},
		{h.projects != nil, "projects", "set_project/list_projects", "multi-project routing"},
		{h.logs != nil, "logs", "get_session_log", "session log queries"},
		{h.sysM != nil, "sysM", "get_digest (runtime metrics)", "system metrics in digest"},
		{h.workersM != nil, "workersM", "get_digest (worker status)", "PC worker status in digest"},
		{h.work != nil, "work", "request_work/claim_issue/complete_issue", "work coordination"},
		{h.scalingEvents != nil, "scalingEvents", "scale_history", "scaling event history"},
		{h.policy != nil, "policy", "approve_action/reject_action", "autonomy policy enforcement"},
	}

	var gaps []WiringGap
	for _, c := range checks {
		if !c.ok {
			gaps = append(gaps, WiringGap{
				Dependency:  c.dep,
				Tool:        c.tool,
				Description: c.description,
			})
		}
	}
	return gaps
}

// activeOwnerRepo returns the owner and repo for the currently active project.
func (h *Handler) activeOwnerRepo() (owner, repo string, err error) {
	if h.projects != nil {
		p, pErr := h.projects.Active()
		if pErr == nil {
			return p.Owner, p.Repo, nil
		}
	}
	return "", "", fmt.Errorf("no active project configured")
}

// resolveTracker returns the IssueTracker for the named project, or the active
// tracker if name is empty. Returns an error if the named project is not found.
func (h *Handler) resolveTracker(name string) (forge.IssueTracker, error) {
	if name == "" {
		return h.activeTracker(), nil
	}
	if h.projects == nil {
		return nil, fmt.Errorf("multi-project support not configured")
	}
	p, ok := h.projects.Get(name)
	if !ok {
		return nil, fmt.Errorf("project %q not found", name)
	}
	return p.Tracker, nil
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

// activeWriter returns the RepoWriter to use for the current context.
// If a project registry is configured, it uses the active project's writer.
// Otherwise, it falls back to the handler's directly-configured writer.
func (h *Handler) activeWriter() forge.RepoWriter {
	if h.projects != nil {
		p, err := h.projects.Active()
		if err == nil && p.Writer != nil {
			return p.Writer
		}
	}
	return h.writer
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
	registerPRReviewTools(srv, h)
	registerScalingTools(srv, h)
	registerWorkerTools(srv, h)
	registerRepoWriteTools(srv, h)
	registerObservabilityTools(srv, h)
	registerDiagnosticsTools(srv, h)
	registerSearchTools(srv, h)
	registerBulkTools(srv, h)
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

// CountTools returns the number of MCP tools registered by this server.
// It spins up a temporary in-process handler and calls tools/list via
// the Streamable HTTP protocol to avoid reaching into SDK internals.
func CountTools() int {
	httpH := NewHTTPHandler(&Handler{})

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	w := httptest.NewRecorder()
	httpH.ServeHTTP(w, req)

	var resp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		return 0
	}
	return len(resp.Result.Tools)
}
