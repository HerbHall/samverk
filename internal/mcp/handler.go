package mcp

import (
	"net/http"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/herbhall/samverk/internal/digest"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/version"
)

// Handler holds dependencies for MCP tool handlers.
type Handler struct {
	tracker forge.IssueTracker
	costs   digest.CostSource // may be nil
}

// NewHandler creates a new MCP tool handler with its dependencies.
func NewHandler(tracker forge.IssueTracker, costs digest.CostSource) *Handler {
	return &Handler{
		tracker: tracker,
		costs:   costs,
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
