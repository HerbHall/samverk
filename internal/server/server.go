package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
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
	Addr               string           // listen address, e.g. ":8080"
	AuthToken          string           //nolint:gosec // G117: config field, not a hardcoded secret
	KeyStore           *KeyStore        // YAML-backed API key store (may be nil)
	SessionFile        string           // optional path for persistent session storage (survive restarts)
	MCPHandler         http.Handler     // optional MCP protocol handler; nil keeps the 501 placeholder
	APIHandler         APIRegistrar     // optional REST API handler; nil keeps the 501 placeholder
	PressureProvider   PressureProvider // optional; /healthz omits pressure field when nil
	VanityResolver      VanityImportResolver // optional; serves go-import meta tags for vanity import paths
	CFAccessTeamDomain  string               // Cloudflare Access team domain; empty = CF auto-login disabled
	CFAccessMCPClientID string               // CF Access client_id for /mcp JWT enforcement; empty = disabled
	SecureCookies       bool                 // set Secure flag on session cookies (false when behind Cloudflare Tunnel)
}

// Server is the main HTTP server.
type Server struct {
	cfg      Config
	mux      *http.ServeMux
	server   *http.Server
	logger   *zap.Logger
	sessions *SessionManager
	hub      *Hub

	mu         sync.Mutex
	listenAddr string
}

// New creates a new Server with routes registered.
func New(cfg Config, logger *zap.Logger) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}

	var sessions *SessionManager
	if cfg.AuthToken != "" || cfg.KeyStore != nil || cfg.CFAccessTeamDomain != "" {
		if cfg.SessionFile != "" {
			sessions = NewSessionManagerWithFile(cfg.SessionFile)
		} else {
			sessions = NewSessionManager()
		}
		sessions.secureCookies = cfg.SecureCookies
	}

	s := &Server{
		cfg:        cfg,
		mux:        http.NewServeMux(),
		logger:     logger,
		sessions:   sessions,
		hub:        NewHub(logger),
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

// Sessions returns the session manager (may be nil if auth is not configured).
// Exposed for the MCP-only listener to share sessions with the main server.
func (s *Server) Sessions() *SessionManager {
	return s.sessions
}

// Hub returns the WebSocket hub. Callers can call Hub().Broadcast to push
// events to all connected dashboard clients.
func (s *Server) Hub() *Hub {
	return s.hub
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

	// Run the WebSocket hub in the background for the lifetime of the server.
	go s.hub.Run(ctx)

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

// Shutdown gracefully stops the server and cleans up sessions.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("server shutting down")
	if s.sessions != nil {
		s.sessions.Stop()
	}
	return s.server.Shutdown(ctx)
}

// authEnabled returns true when any form of authentication is configured.
func (s *Server) authEnabled() bool {
	return s.cfg.AuthToken != "" || s.cfg.KeyStore != nil
}

// registerRoutes wires all HTTP endpoints.
func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)

	// Return 404 for /.well-known/ paths so the SPA catch-all doesn't
	// serve HTML with a 200 status. The specific OAuth discovery path below
	// takes priority via Go 1.22+ ServeMux exact-pattern matching.
	s.mux.HandleFunc("GET /.well-known/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
	})

	// OAuth 2.0 Authorization Server Metadata + proxy endpoints for Claude.ai
	// custom connector authentication via Cloudflare Access OIDC.
	// No-op when CFAccessTeamDomain or CFAccessMCPClientID is not configured.
	RegisterMCPOAuthRoutes(s.mux, MCPOAuthConfig{
		TeamDomain: s.cfg.CFAccessTeamDomain,
		ClientID:   s.cfg.CFAccessMCPClientID,
	})

	// Login / logout routes (unauthenticated).
	if s.authEnabled() {
		// Wrap login page with CF Access auto-login when configured.
		loginPageHandler := http.Handler(http.HandlerFunc(s.handleLoginPage))
		if s.cfg.CFAccessTeamDomain != "" && s.sessions != nil {
			loginPageHandler = CFAccessMiddleware(s.cfg.CFAccessTeamDomain, s.sessions)(loginPageHandler)
		}
		s.mux.Handle("GET /login", loginPageHandler)
		s.mux.HandleFunc("POST /login", s.handleLoginSubmit)
		s.mux.HandleFunc("POST /logout", s.handleLogout)
	}

	if s.cfg.MCPHandler != nil {
		// MCP endpoints: JWT enforcement takes priority over Bearer auth.
		// Register without method prefix so StreamableHTTPHandler receives
		// POST (messages), GET (SSE streams), and DELETE (session teardown).
		//
		// When CF_ACCESS_TEAM_DOMAIN + CF_ACCESS_MCP_CLIENT_ID are set, the
		// Cloudflare Tunnel routes external traffic to port 8080 (this server),
		// so JWT enforcement must live here — not on the MCP-only port 8081.
		jwtEnabled := s.cfg.CFAccessTeamDomain != "" && s.cfg.CFAccessMCPClientID != ""

		var mcpAuth func(http.Handler) http.Handler
		switch {
		case jwtEnabled && s.authEnabled():
			// Accept either CF Access JWT (external/CF-tunnel) or Bearer token
			// (internal/LAN). JWT is checked first; Bearer is the fallback.
			mcpAuth = CFAccessOrBearerAuth(s.cfg.CFAccessTeamDomain, s.cfg.CFAccessMCPClientID, s.cfg.AuthToken, s.cfg.KeyStore)
		case jwtEnabled:
			// JWT-only: no Bearer configured.
			mcpAuth = CFAccessEnforceMiddleware(s.cfg.CFAccessTeamDomain, s.cfg.CFAccessMCPClientID)
		case s.authEnabled():
			// Bearer-only: no CF Access configured.
			mcpAuth = BearerAuth(s.cfg.AuthToken, s.cfg.KeyStore)
		}

		if mcpAuth != nil {
			s.mux.Handle("/connect", mcpAuth(s.cfg.MCPHandler))
			s.mux.Handle("/mcp", mcpAuth(s.cfg.MCPHandler))
		} else {
			s.mux.Handle("/connect", s.cfg.MCPHandler)
			s.mux.Handle("/mcp", s.cfg.MCPHandler)
		}
	} else {
		s.mux.HandleFunc("/connect", s.handleNotImplemented)
		s.mux.HandleFunc("/mcp", s.handleNotImplemented)
	}

	// WebSocket event hub — requires Bearer token or session auth when configured.
	if s.authEnabled() {
		wsAuth := BearerOrSessionAuth(s.cfg.AuthToken, s.cfg.KeyStore, s.sessions)
		s.mux.Handle("GET /ws", wsAuth(http.HandlerFunc(s.handleWS)))
	} else {
		s.mux.HandleFunc("GET /ws", s.handleWS)
	}

	if s.cfg.APIHandler != nil {
		apiMux := http.NewServeMux()
		s.cfg.APIHandler.RegisterRoutes(apiMux)
		if s.authEnabled() {
			// API accepts either Bearer token or session cookie.
			s.mux.Handle("/api/", BearerOrSessionAuth(s.cfg.AuthToken, s.cfg.KeyStore, s.sessions)(apiMux))
		} else {
			s.mux.Handle("/api/", apiMux)
		}
	} else {
		s.mux.HandleFunc("/api/v1/", s.handleNotImplemented)
	}

	// Serve embedded SPA for all unmatched routes.
	// When auth is enabled, require a valid session cookie.
	// CF Access middleware runs first: if a valid JWT is present, it auto-issues
	// a session and redirects to /, bypassing the token login form.
	spa := http.Handler(spaHandler())
	if s.authEnabled() {
		spaWithAuth := requireSession(s.sessions)(spa)
		if s.cfg.CFAccessTeamDomain != "" && s.sessions != nil {
			spaWithAuth = CFAccessMiddleware(s.cfg.CFAccessTeamDomain, s.sessions)(spaWithAuth)
		}
		spa = spaWithAuth
	}

	// Vanity Go import handler: intercepts ?go-get=1 requests BEFORE auth
	// so Go tooling can resolve module paths without credentials.
	if s.cfg.VanityResolver != nil {
		vanity := handleVanityImport(s.cfg.VanityResolver)
		authedSPA := spa
		s.mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("go-get") == "1" {
				vanity.ServeHTTP(w, r)
				return
			}
			authedSPA.ServeHTTP(w, r)
		}))
	} else {
		s.mux.Handle("/", spa)
	}
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
