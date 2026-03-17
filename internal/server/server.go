package server

import (
	"context"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"net"
	"net/http"
	"sync"
	"time"
)

// APIRegistrar can register REST API routes on an http.ServeMux.
type APIRegistrar interface {
	RegisterRoutes(mux *http.ServeMux)
}

// PressureProvider returns the current resource pressure level string
// ("low", "moderate", "high", or "critical"). Used by /healthz.
type PressureProvider interface {
	Pressure() string
}

// Config holds server configuration.
type Config struct {
	Addr             string          // listen address, e.g. ":8080"
	AuthToken        string          //nolint:gosec // G117: config field, not a hardcoded secret
	KeyStore         *KeyStore       // YAML-backed API key store (may be nil)
	MCPHandler       http.Handler    // optional MCP protocol handler; nil keeps the 501 placeholder
	APIHandler       APIRegistrar    // optional REST API handler; nil keeps the 501 placeholder
	PressureProvider PressureProvider // optional; /healthz omits pressure field when nil
}

// Server is the main HTTP server.
type Server struct {
	cfg    Config
	mux    *http.ServeMux
	server *http.Server
	logger *zap.Logger

	mu         sync.Mutex
	listenAddr string
}

// New creates a new Server with routes registered.
func New(cfg Config, logger *zap.Logger) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	s := &Server{
		cfg:        cfg,
		mux:        http.NewServeMux(),
		logger:     logger,
		listenAddr: cfg.Addr,
	}

	s.server = &http.Server{
		Addr:              cfg.Addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	s.registerRoutes()
	return s
}

// Addr returns the address the server is listening on.
// This is useful in tests when Addr is ":0" (random port).
// Safe for concurrent use.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listenAddr
}

// Handler returns the underlying http.Handler for use in httptest.NewServer.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Start begins listening. It blocks until ctx is cancelled or an error occurs.
// When ctx is cancelled, Start calls Shutdown and returns nil.
func (s *Server) Start(ctx context.Context) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.cfg.Addr, err)
	}

	// Store the actual bound address (matters when port is 0).
	actual := ln.Addr().String()
	s.mu.Lock()
	s.listenAddr = actual
	s.mu.Unlock()

	s.logger.Info("server listening", zap.String("addr", actual))

	serveErr := make(chan error, 1)
	go func() {
		err := s.server.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			serveErr <- err
		} else {
			serveErr <- nil
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx := context.Background()
		if err := s.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return <-serveErr
	case err := <-serveErr:
		return err
	}
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("server shutting down")
	return s.server.Shutdown(ctx)
}

// registerRoutes wires all HTTP endpoints.
func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)

	// Return 404 for /.well-known/ paths so the SPA catch-all doesn't
	// serve HTML with a 200 status. Claude.ai probes this endpoint and
	// interprets a 200 as "OAuth is available", breaking MCP connectivity.
	s.mux.HandleFunc("GET /.well-known/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
	})

	if s.cfg.MCPHandler != nil {
		// MCP endpoint is unauthenticated -- security is handled by
		// Cloudflare Tunnel (same model as Synapset MCP).
		// Register without method prefix so StreamableHTTPHandler receives
		// POST (messages), GET (SSE streams), and DELETE (session teardown).
		//
		// Cloudflare AI protection blocks paths it recognizes as AI
		// endpoints (/mcp, /sse) with 403 "invalid Host header".
		// Use /connect for external access through Cloudflare Tunnel.
		// Custom Connector URL: https://samverk.herbhall.net/connect
		// Keep /mcp for LAN/Tailscale and existing configs.
		s.mux.Handle("/connect", s.cfg.MCPHandler)
		s.mux.Handle("/mcp", s.cfg.MCPHandler)
	} else {
		s.mux.HandleFunc("/connect", s.handleNotImplemented)
		s.mux.HandleFunc("/mcp", s.handleNotImplemented)
	}

	if s.cfg.APIHandler != nil {
		apiMux := http.NewServeMux()
		s.cfg.APIHandler.RegisterRoutes(apiMux)
		if s.cfg.AuthToken != "" || s.cfg.KeyStore != nil {
			s.mux.Handle("/api/", BearerAuth(s.cfg.AuthToken, s.cfg.KeyStore)(apiMux))
		} else {
			s.mux.Handle("/api/", apiMux)
		}
	} else {
		s.mux.HandleFunc("/api/v1/", s.handleNotImplemented)
	}

	// Serve embedded SPA for all unmatched routes.
	s.mux.Handle("/", spaHandler(s.cfg.AuthToken))
}

// healthResponse is the JSON body returned by /healthz.
// Pressure is omitted when no PressureProvider is configured.
type healthResponse struct {
	Status   string `json:"status"`
	Pressure string `json:"pressure,omitempty"`
}

// errorResponse is the JSON body returned for error responses.
type errorResponse struct {
	Error string `json:"error"`
}

// handleHealth returns 200 {"status":"ok"} with an optional pressure field.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	resp := healthResponse{Status: "ok"}
	if s.cfg.PressureProvider != nil {
		resp.Pressure = s.cfg.PressureProvider.Pressure()
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleNotImplemented returns 501 {"error":"not implemented"}.
func (s *Server) handleNotImplemented(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, errorResponse{Error: "not implemented"})
}

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		zap.L().Error("failed to encode JSON response", zap.Error(err))
	}
}
