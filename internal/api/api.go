package api

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/advisor"
	"github.com/herbhall/samverk/internal/digest"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/hostmetrics"
	"github.com/herbhall/samverk/internal/loganalyst"
	"github.com/herbhall/samverk/internal/logstore"
	internalmcp "github.com/herbhall/samverk/internal/mcp"
	"github.com/herbhall/samverk/internal/metrics"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/internal/status"
	"github.com/herbhall/samverk/internal/store"
)

// poolMetricsSource provides a snapshot of the agent pool.
type poolMetricsSource interface {
	Snapshot() metrics.PoolSnapshot
}

// dispatcherMetricsSource provides a snapshot of the dispatcher.
type dispatcherMetricsSource interface {
	Snapshot() metrics.DispatcherSnapshot
}

// systemMetricsSource provides a system-level metrics snapshot.
type systemMetricsSource interface {
	Collect() metrics.SystemSnapshot
}

// API provides REST endpoints for the web dashboard.
type API struct {
	tracker        forge.IssueTracker      // may be nil if no forge credentials
	store          store.Store             // may be nil if no database
	costs          digest.CostSource       // may be nil if no cost tracking
	pool           poolMetricsSource       // may be nil if pool not yet started
	dispatcher     dispatcherMetricsSource // may be nil if dispatcher not started
	system         systemMetricsSource     // may be nil; created via SetMetrics
	hostMetrics    *hostmetrics.Collector   // may be nil if collector not started
	infra          hostmetrics.InfraContext // Proxmox context for action generation
	capacity       *capacityDTO            // may be nil if no providers configured
	logAnalyst     *loganalyst.Analyst      // may be nil if logstore not configured
	healthMonitor    *provider.HealthMonitor  // may be nil if health monitor not started
	providerRegistry *provider.Registry       // may be nil if no provider registry configured
	workers          *workerRegistry          // in-memory registry of PC agent workers
	scalingEnabled bool                    // true when autoscaler was configured
	scalingMin     int
	scalingMax     int
	advisor         *advisor.Advisor             // may be nil; set via SetAdvisor
	projectRegistry *internalmcp.ProjectRegistry // may be nil; single-project mode
	synapsetProxy   *SynapsetProxy               // may be nil; proxies Synapset API requests
	logStore        *logstore.LogStore            // may be nil; for log query API
	toolCount          int                           // number of MCP tools registered; set via SetToolCount
	chatAPIKey         string                        // Anthropic API key for chat proxy; empty = use env
	chatRateLimit      *chatRateLimit                // rate limiter for chat endpoint
	history            []historyEntry                // ring buffer of recent snapshots; guarded by historyMu
	primaryDBPath      string                        // file-system path to the primary SQLite database
	logsDBPath         string                        // file-system path to the logs SQLite database
	configReloadSrc    configReloadErrorSource       // may be nil; reports project.yaml reload errors
	subsystems         *status.Registry              // may be nil; subsystem liveness registry
	logger             *zap.Logger
}

// New creates an API handler with the given dependencies.
// Any dependency may be nil; endpoints that require a nil dependency
// return 503 Service Unavailable.
func New(tracker forge.IssueTracker, s store.Store, costs digest.CostSource, logger *zap.Logger) *API {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &API{
		tracker:       tracker,
		store:         s,
		costs:         costs,
		workers:       newWorkerRegistry(),
		chatRateLimit: newChatRateLimit(),
		logger:        logger,
	}
}

// SetMetrics attaches metrics sources to the API handler. Call before serving.
func (a *API) SetMetrics(pool poolMetricsSource, disp dispatcherMetricsSource, sys systemMetricsSource) {
	a.pool = pool
	a.dispatcher = disp
	a.system = sys
}

// SetCapacity configures the static provider capacity info displayed on the
// metrics page. Call from the serve command after parsing providers.yaml.
func (a *API) SetCapacity(providers []ProviderDTO, routing map[string][]string) {
	a.capacity = &capacityDTO{
		Providers:     providers,
		RoutingChains: routing,
	}
}

// SetLogStore attaches the log store for the log query API endpoint.
func (a *API) SetLogStore(ls *logstore.LogStore) {
	a.logStore = ls
}

// SetScalingConfig records the active scaling policy limits so the metrics endpoint
// can expose them alongside events read from the store. Call before serving.
// If not called, scaling_events and scaling_config in /api/v1/metrics will be null.
func (a *API) SetScalingConfig(enabled bool, minW, maxW int) {
	a.scalingEnabled = enabled
	a.scalingMin = minW
	a.scalingMax = maxW
}

// SetLogAnalyst attaches the log analyst for AI-powered log summaries.
// Call from the serve command after creating the logstore and Ollama client.
func (a *API) SetLogAnalyst(la *loganalyst.Analyst) {
	a.logAnalyst = la
}

// SetProviderRegistry attaches the provider registry so the health endpoint
// can include routing_chains in its response. Call before serving. May be nil.
func (a *API) SetProviderRegistry(reg *provider.Registry) {
	a.providerRegistry = reg
}

// SetProjectRegistry attaches the MCP project registry so the projects endpoint
// can list registered projects. Call before serving. May be nil (single-project mode).
func (a *API) SetProjectRegistry(reg *internalmcp.ProjectRegistry) {
	a.projectRegistry = reg
}

// SetToolCount records the number of MCP tools exposed by this server
// so the /api/v1/status and /api/v1/mcp/servers endpoints can report it.
func (a *API) SetToolCount(n int) {
	a.toolCount = n
}

// SetSubsystemRegistry attaches the subsystem liveness registry.
func (a *API) SetSubsystemRegistry(r *status.Registry) {
	a.subsystems = r
}

// RegisterRoutes registers all API endpoints on the given mux.
// Routes use Go 1.22+ method+path patterns.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/issues", a.handleListIssues)
	mux.HandleFunc("GET /api/v1/issues/search", a.handleSearchIssues)
	mux.HandleFunc("POST /api/v1/issues/bulk", a.handleBulkIssues)
	mux.HandleFunc("GET /api/v1/issues/{number}", a.handleGetIssue)
	mux.HandleFunc("GET /api/v1/issues/{number}/cost", a.handleGetIssueCost)
	mux.HandleFunc("GET /api/v1/sessions", a.handleListSessions)
	mux.HandleFunc("GET /api/v1/sessions/{id}/log", a.handleGetSessionLog)
	mux.HandleFunc("GET /api/v1/costs", a.handleGetCosts)
	mux.HandleFunc("GET /api/v1/status", a.handleStatus)
	mux.HandleFunc("GET /api/v1/metrics", a.handleMetrics)
	mux.HandleFunc("GET /api/v1/metrics/history", a.handleMetricsHistory)
	mux.HandleFunc("GET /api/v1/scaling/control", a.handleGetScalingControl)
	mux.HandleFunc("POST /api/v1/scaling/pause", a.handleScalingPause)
	mux.HandleFunc("POST /api/v1/scaling/resume", a.handleScalingResume)
	mux.HandleFunc("POST /api/v1/scaling/set", a.handleScalingSet)
	mux.HandleFunc("GET /api/v1/workers/active", a.handleListActiveWorkers)
	mux.HandleFunc("GET /api/v1/workers", a.handleListWorkers)
	mux.HandleFunc("POST /api/v1/workers/register", a.handleRegisterWorker)
	mux.HandleFunc("POST /api/v1/workers/heartbeat", a.handleWorkerHeartbeat)
	mux.HandleFunc("GET /api/v1/metrics/host", a.handleHostMetrics)
	mux.HandleFunc("GET /api/v1/failures", a.handleFailureSummary)
	mux.HandleFunc("POST /api/v1/failures/reset", a.handleResetFailureCounts)
	mux.HandleFunc("GET /api/v1/kpis", a.handleGetKPIs)
	mux.HandleFunc("GET /api/v1/quality/recommendations", a.handleGetRecommendations)
	mux.HandleFunc("GET /api/v1/providers/health", a.handleProviderHealth)
	mux.HandleFunc("GET /api/v1/agents", a.handleAgents)
	mux.HandleFunc("GET /api/v1/logs", a.handleListLogs)
	mux.HandleFunc("GET /api/v1/logs/summary", a.handleLogSummary)
	mux.HandleFunc("POST /api/v1/chat", a.handleChat)
	mux.HandleFunc("GET /api/v1/projects", a.handleListProjects)
	mux.HandleFunc("GET /api/v1/forges", a.handleListForges)
	mux.HandleFunc("POST /api/v1/capacity/apply", a.handleCapacityApply)
	mux.HandleFunc("GET /api/v1/devkit/summary", a.handleDevkitSummary)
	mux.HandleFunc("GET /api/v1/data/sources", a.handleDataSources)
	mux.HandleFunc("GET /api/v1/mcp/servers", a.handleMCPServers)
	mux.HandleFunc("GET /api/v1/pipeline/events", a.handlePipelineEvents)
	mux.HandleFunc("GET /api/v1/pipeline/throughput", a.handlePipelineThroughput)
	mux.HandleFunc("GET /api/v1/pipeline/stages", a.handlePipelineStages)
	mux.HandleFunc("GET /api/v1/system/version", a.handleSystemVersion)
	mux.HandleFunc("GET /api/v1/system/subsystems", a.handleSystemSubsystems)
	mux.HandleFunc("GET /api/v1/diagnostics", a.handleDiagnostics)

	// Synapset proxy routes (no-op if proxy not configured).
	a.RegisterSynapsetRoutes(mux)
}

// errorResponse is the JSON body returned for error responses.
type errorResponse struct {
	Error string `json:"error"`
}

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		zap.L().Error("api: failed to encode JSON response", zap.Error(err))
	}
}

// writeError writes a JSON error response with the given status code.
func writeError(w http.ResponseWriter, statusCode int, msg string) {
	writeJSON(w, statusCode, errorResponse{Error: msg})
}
