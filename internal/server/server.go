package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Config holds server configuration.
type Config struct {
	Addr string // listen address, e.g. ":8080"
}

// Server is the main HTTP server.
type Server struct {
	cfg    Config
	mux    *http.ServeMux
	server *http.Server
}

// New creates a new Server with routes registered.
func New(cfg Config) *Server {
	s := &Server{
		cfg: cfg,
		mux: http.NewServeMux(),
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
func (s *Server) Addr() string {
	return s.server.Addr
}

// Handler returns the underlying http.Handler for use in httptest.NewServer.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Start begins listening. It blocks until ctx is cancelled or an error occurs.
// When ctx is cancelled, Start calls Shutdown and returns nil.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.cfg.Addr, err)
	}

	// Update Addr to the actual bound address (matters when port is 0).
	s.server.Addr = ln.Addr().String()

	slog.Info("server listening", "addr", s.server.Addr)

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
	slog.Info("server shutting down")
	return s.server.Shutdown(ctx)
}

// registerRoutes wires all HTTP endpoints.
func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /mcp", s.handleNotImplemented)
	s.mux.HandleFunc("/api/v1/", s.handleNotImplemented)
	s.mux.HandleFunc("/", s.handleNotImplemented)
}

// healthResponse is the JSON body returned by /healthz.
type healthResponse struct {
	Status string `json:"status"`
}

// errorResponse is the JSON body returned for error responses.
type errorResponse struct {
	Error string `json:"error"`
}

// handleHealth returns 200 {"status":"ok"}.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
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
		slog.Error("failed to encode JSON response", "err", err)
	}
}
