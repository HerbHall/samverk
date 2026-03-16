package api

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// SynapsetProxy forwards dashboard API requests to the Synapset service.
type SynapsetProxy struct {
	baseURL string
	client  *http.Client
	logger  *zap.Logger
}

// NewSynapsetProxy creates a proxy that forwards requests to the given Synapset base URL.
func NewSynapsetProxy(baseURL string, logger *zap.Logger) *SynapsetProxy {
	return &SynapsetProxy{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// SetSynapsetProxy configures the Synapset proxy for dashboard API requests.
func (a *API) SetSynapsetProxy(baseURL string) {
	a.synapsetProxy = NewSynapsetProxy(baseURL, a.logger)
}

// RegisterSynapsetRoutes registers all Synapset proxy routes on the given mux.
func (a *API) RegisterSynapsetRoutes(mux *http.ServeMux) {
	if a.synapsetProxy == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/synapset/status", a.synapsetProxy.handle("/api/status"))
	mux.HandleFunc("GET /api/v1/synapset/tools/summary", a.synapsetProxy.handle("/api/tools/summary"))
	mux.HandleFunc("GET /api/v1/synapset/tools/history", a.synapsetProxy.handle("/api/tools/history"))
	mux.HandleFunc("GET /api/v1/synapset/search/stats", a.synapsetProxy.handle("/api/search/stats"))
	mux.HandleFunc("GET /api/v1/synapset/memories/trend", a.synapsetProxy.handle("/api/memories/trend"))
	mux.HandleFunc("GET /api/v1/synapset/pools/stats", a.synapsetProxy.handle("/api/pools/stats"))
	mux.HandleFunc("GET /api/v1/synapset/metrics/history", a.synapsetProxy.handle("/api/metrics/history"))
	mux.HandleFunc("GET /api/v1/synapset/logs", a.synapsetProxy.handle("/api/logs"))
}

// handle returns an HTTP handler that proxies requests to the given upstream path.
func (p *SynapsetProxy) handle(upstreamPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetURL := p.baseURL + upstreamPath
		if q := r.URL.RawQuery; q != "" {
			targetURL += "?" + q
		}

		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, http.NoBody)
		if err != nil {
			p.logger.Error("synapset proxy: build request", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to build upstream request")
			return
		}
		req.Header.Set("Accept", "application/json")

		resp, err := p.client.Do(req) //nolint:gosec // G107: URL is built from configured baseURL, not user input
		if err != nil {
			p.logger.Warn("synapset proxy: upstream request failed",
				zap.String("url", targetURL), zap.Error(err))
			writeError(w, http.StatusBadGateway, fmt.Sprintf("synapset unreachable: %v", err))
			return
		}
		defer func() { _ = resp.Body.Close() }()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		if _, copyErr := io.Copy(w, resp.Body); copyErr != nil {
			p.logger.Warn("synapset proxy: copy response body", zap.Error(copyErr))
		}
	}
}
