// Package infra implements infrastructure probing for known endpoints
// (Ollama instances, Samverk services, Gitea) and synchronises the
// results with Synapset machine memories.
package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"samverk.dev/samverk/internal/synapset"
	"go.uber.org/zap"
)

// defaultProbeTimeout is the per-endpoint HTTP timeout.
const defaultProbeTimeout = 10 * time.Second

// Endpoint describes a single infrastructure endpoint to probe.
type Endpoint struct {
	Name   string // human-readable name (e.g. "vm-300")
	URL    string // base URL (e.g. "http://192.168.1.207:11434")
	Kind   string // "ollama", "samverk", "gitea", "gitea-runners"
	Source string // Synapset source key for query_memory
}

// ProbeResult holds the state captured from a single endpoint probe.
type ProbeResult struct {
	Endpoint  Endpoint
	Reachable bool
	Error     string            // non-empty when Reachable is false
	Fields    map[string]string // key-value pairs of probed state
}

// Prober polls infrastructure endpoints and syncs findings to Synapset.
type Prober struct {
	synapset   *synapset.Client
	httpClient *http.Client
	logger     *zap.Logger
	endpoints  []Endpoint
}

// NewProber creates a Prober with the given Synapset client and endpoints.
func NewProber(sc *synapset.Client, logger *zap.Logger, endpoints []Endpoint) *Prober {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Prober{
		synapset: sc,
		httpClient: &http.Client{
			Timeout: defaultProbeTimeout,
		},
		logger:    logger,
		endpoints: endpoints,
	}
}

// DefaultEndpoints returns the standard set of infrastructure endpoints.
func DefaultEndpoints() []Endpoint {
	return []Endpoint{
		{Name: "vm-300", URL: "http://192.168.1.207:11434", Kind: "ollama", Source: "vm-300"},
		{Name: "hdh-nzxt", URL: "http://192.168.1.202:11434", Kind: "ollama", Source: "hdh-nzxt"},
		{Name: "cm-asus", URL: "http://100.88.37.47:11434", Kind: "ollama", Source: "cm-asus"},
		{Name: "samverk-ct202", URL: "http://192.168.1.162:8080", Kind: "samverk", Source: "samverk-ct202"},
		{Name: "gitea-ct200", URL: "http://192.168.1.160:3000", Kind: "gitea", Source: "gitea-ct200"},
		{Name: "gitea-runners", URL: "http://192.168.1.160:9090", Kind: "gitea-runners", Source: "gitea-runners"},
	}
}

// Run probes all endpoints and syncs meaningful changes to Synapset.
// It returns the results for each endpoint. Unreachable hosts are logged
// and skipped without aborting the remaining probes.
func (p *Prober) Run(ctx context.Context) []ProbeResult {
	results := make([]ProbeResult, 0, len(p.endpoints))

	for i := range p.endpoints {
		ep := &p.endpoints[i]
		p.logger.Info("probing endpoint", zap.String("name", ep.Name), zap.String("kind", ep.Kind))

		result := p.probeEndpoint(ctx, *ep)
		results = append(results, result)

		if !result.Reachable {
			p.logger.Warn("endpoint unreachable",
				zap.String("name", ep.Name),
				zap.String("error", result.Error))
			continue
		}

		p.logger.Info("endpoint reachable",
			zap.String("name", ep.Name),
			zap.Int("fields", len(result.Fields)))

		if err := p.syncToSynapset(ctx, result); err != nil {
			p.logger.Warn("synapset sync failed",
				zap.String("name", ep.Name),
				zap.Error(err))
		}
	}

	return results
}

// probeEndpoint dispatches to the appropriate probe method based on Kind.
func (p *Prober) probeEndpoint(ctx context.Context, ep Endpoint) ProbeResult {
	switch ep.Kind {
	case "ollama":
		return p.probeOllama(ctx, ep)
	case "samverk":
		return p.probeSamverk(ctx, ep)
	case "gitea":
		return p.probeGitea(ctx, ep)
	case "gitea-runners":
		return p.probeGiteaRunners(ctx, ep)
	default:
		return ProbeResult{
			Endpoint: ep,
			Error:    fmt.Sprintf("unknown endpoint kind: %s", ep.Kind),
		}
	}
}

// ollamaTagsResponse is the response from GET /api/tags.
type ollamaTagsResponse struct {
	Models []ollamaModel `json:"models"`
}

// ollamaModel is a single model entry from the Ollama API.
type ollamaModel struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// ollamaPsResponse is the response from GET /api/ps.
type ollamaPsResponse struct {
	Models []ollamaPsModel `json:"models"`
}

// ollamaPsModel is a running model entry from Ollama.
type ollamaPsModel struct {
	Name string `json:"name"`
}

// probeOllama queries an Ollama instance for installed and running models.
func (p *Prober) probeOllama(ctx context.Context, ep Endpoint) ProbeResult {
	result := ProbeResult{Endpoint: ep, Fields: make(map[string]string)}

	// GET /api/tags — installed models.
	tagsBody, err := p.httpGet(ctx, ep.URL+"/api/tags")
	if err != nil {
		return ProbeResult{Endpoint: ep, Error: fmt.Sprintf("tags: %v", err)}
	}

	var tags ollamaTagsResponse
	if err = json.Unmarshal(tagsBody, &tags); err != nil {
		return ProbeResult{Endpoint: ep, Error: fmt.Sprintf("parse tags: %v", err)}
	}

	models := make([]string, 0, len(tags.Models))
	for i := range tags.Models {
		models = append(models, tags.Models[i].Name)
	}
	sort.Strings(models)
	result.Fields["models"] = strings.Join(models, ", ")
	result.Reachable = true

	// GET /api/ps — running models (best-effort).
	psBody, err := p.httpGet(ctx, ep.URL+"/api/ps")
	if err != nil {
		p.logger.Debug("ollama /api/ps failed", zap.String("name", ep.Name), zap.Error(err))
		return result
	}

	var ps ollamaPsResponse
	if err = json.Unmarshal(psBody, &ps); err != nil {
		p.logger.Debug("ollama /api/ps parse failed", zap.String("name", ep.Name), zap.Error(err))
		return result
	}

	running := make([]string, 0, len(ps.Models))
	for i := range ps.Models {
		running = append(running, ps.Models[i].Name)
	}
	sort.Strings(running)
	result.Fields["running_models"] = strings.Join(running, ", ")

	return result
}

// samverkHealthResponse is the response from GET /healthz.
type samverkHealthResponse struct {
	Status string `json:"status"`
}

// samverkStatusResponse is the response from GET /api/v1/status.
type samverkStatusResponse struct {
	Version string `json:"version"`
	Workers int    `json:"workers"`
}

// probeSamverk queries the Samverk HTTP server for health and version.
func (p *Prober) probeSamverk(ctx context.Context, ep Endpoint) ProbeResult {
	result := ProbeResult{Endpoint: ep, Fields: make(map[string]string)}

	// GET /healthz.
	healthBody, err := p.httpGet(ctx, ep.URL+"/healthz")
	if err != nil {
		return ProbeResult{Endpoint: ep, Error: fmt.Sprintf("healthz: %v", err)}
	}
	var health samverkHealthResponse
	if err = json.Unmarshal(healthBody, &health); err != nil {
		// healthz may return plain text "ok".
		health.Status = strings.TrimSpace(string(healthBody))
	}
	result.Fields["health"] = health.Status
	result.Reachable = true

	// GET /api/v1/status.
	statusBody, err := p.httpGet(ctx, ep.URL+"/api/v1/status")
	if err != nil {
		p.logger.Debug("samverk /api/v1/status failed", zap.String("name", ep.Name), zap.Error(err))
		return result
	}
	var status samverkStatusResponse
	if err = json.Unmarshal(statusBody, &status); err == nil {
		if status.Version != "" {
			result.Fields["version"] = status.Version
		}
		result.Fields["workers"] = fmt.Sprintf("%d", status.Workers)
	}

	return result
}

// giteaVersionResponse is the response from GET /api/v1/version.
type giteaVersionResponse struct {
	Version string `json:"version"`
}

// probeGitea queries a Gitea instance for its version.
func (p *Prober) probeGitea(ctx context.Context, ep Endpoint) ProbeResult {
	result := ProbeResult{Endpoint: ep, Fields: make(map[string]string)}

	body, err := p.httpGet(ctx, ep.URL+"/api/v1/version")
	if err != nil {
		return ProbeResult{Endpoint: ep, Error: fmt.Sprintf("version: %v", err)}
	}

	var ver giteaVersionResponse
	if err = json.Unmarshal(body, &ver); err != nil {
		return ProbeResult{Endpoint: ep, Error: fmt.Sprintf("parse version: %v", err)}
	}

	result.Fields["version"] = ver.Version
	result.Reachable = true

	return result
}

// giteaRunner is a single runner entry from the runner health endpoint.
type giteaRunner struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	Version          string `json:"version"`
	Status           string `json:"status"`
	SecondsSinceOnline int  `json:"seconds_since_online"`
}

// probeGiteaRunners queries the runner health endpoint on CT 200 and reports
// total/online/offline counts. Logs a warning for each offline runner.
func (p *Prober) probeGiteaRunners(ctx context.Context, ep Endpoint) ProbeResult {
	result := ProbeResult{Endpoint: ep, Fields: make(map[string]string)}

	body, err := p.httpGet(ctx, ep.URL+"/runners")
	if err != nil {
		return ProbeResult{Endpoint: ep, Error: fmt.Sprintf("runners: %v", err)}
	}

	var runners []giteaRunner
	if err = json.Unmarshal(body, &runners); err != nil {
		return ProbeResult{Endpoint: ep, Error: fmt.Sprintf("parse runners: %v", err)}
	}

	var online, offline int
	offlineNames := make([]string, 0)
	for i := range runners {
		if runners[i].Status == "online" {
			online++
		} else {
			offline++
			offlineNames = append(offlineNames, runners[i].Name)
			p.logger.Warn("CI runner offline",
				zap.String("runner", runners[i].Name),
				zap.Int("id", runners[i].ID),
				zap.Int("seconds_since_online", runners[i].SecondsSinceOnline))
		}
	}

	result.Fields["total"] = fmt.Sprintf("%d", len(runners))
	result.Fields["online"] = fmt.Sprintf("%d", online)
	result.Fields["offline"] = fmt.Sprintf("%d", offline)
	if len(offlineNames) > 0 {
		result.Fields["offline_runners"] = strings.Join(offlineNames, ", ")
	}
	result.Reachable = true

	return result
}

// httpGet performs a GET request and returns the response body.
func (p *Prober) httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := p.httpClient.Do(req) //nolint:gosec // G704: URLs are from trusted internal config
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return body, nil
}

// syncToSynapset compares probed state against the existing Synapset memory
// for the endpoint's source and writes updates for meaningful changes.
func (p *Prober) syncToSynapset(ctx context.Context, result ProbeResult) error {
	if p.synapset == nil {
		return nil
	}

	pool := "machines"
	source := result.Endpoint.Source

	// Query existing memory for this source.
	existing, err := p.synapset.QueryMemory(ctx, pool, map[string]interface{}{
		"source": source,
	})
	if err != nil {
		p.logger.Warn("synapset query_memory failed, will store fresh",
			zap.String("source", source),
			zap.Error(err))
	}

	// Build a summary of probed state.
	summary := formatProbeFields(result)

	// If we found an existing memory, check for meaningful change.
	if len(existing) > 0 {
		mem := existing[0]
		if mem.Content == summary {
			p.logger.Info("no meaningful change, skipping update",
				zap.String("source", source))
			return nil
		}

		// Update the existing memory.
		p.logger.Info("updating synapset memory",
			zap.String("source", source),
			zap.String("memory_id", mem.ID.String()))
		return p.synapset.UpdateMemory(ctx, pool, mem.ID.String(), map[string]interface{}{
			"content": summary,
		})
	}

	// No existing memory — store a new one.
	p.logger.Info("storing new synapset memory", zap.String("source", source))
	return p.synapset.StoreMemory(ctx, pool, summary, "infra-probe", []string{"infrastructure", "probe"}, source)
}

// formatProbeFields builds a deterministic summary string from the probe fields.
func formatProbeFields(result ProbeResult) string {
	keys := make([]string, 0, len(result.Fields))
	for k := range result.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(result.Endpoint.Name)
	b.WriteString(" (")
	b.WriteString(result.Endpoint.Kind)
	b.WriteString("): ")

	for i, k := range keys {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(result.Fields[k])
	}
	return b.String()
}
