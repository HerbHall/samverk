package api

import (
	"net/http"
	"time"
)

// mcpServerInfo describes one MCP server known to the system.
type mcpServerInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`     // "hosted" or "consumed"
	Endpoint    string `json:"endpoint"`
	Healthy     bool   `json:"healthy"`
	ToolCount   int    `json:"tool_count"`
	Description string `json:"description"`
}

// handleMCPServers handles GET /api/v1/mcp/servers.
// Returns health and tool inventory for Samverk MCP (self) and Synapset MCP (consumed).
func (a *API) handleMCPServers(w http.ResponseWriter, r *http.Request) {
	servers := []mcpServerInfo{
		a.samverkMCPInfo(),
	}
	if a.synapsetProxy != nil {
		servers = append(servers, a.synapsetMCPInfo(r))
	}
	writeJSON(w, http.StatusOK, servers)
}

// samverkMCPInfo builds the entry for Samverk's own hosted MCP server.
// It is always reachable internally; health is derived from overall system health.
func (a *API) samverkMCPInfo() mcpServerInfo {
	healthy := a.tracker != nil && a.store != nil
	return mcpServerInfo{
		Name:        "Samverk MCP",
		Type:        "hosted",
		Endpoint:    "/mcp",
		Healthy:     healthy,
		ToolCount:   a.toolCount,
		Description: "Samverk's own MCP server — issue management, digest, cost, scaling, and observability tools.",
	}
}

// synapsetMCPInfo pings the Synapset proxy status endpoint to determine reachability.
func (a *API) synapsetMCPInfo(r *http.Request) mcpServerInfo {
	info := mcpServerInfo{
		Name:        "Synapset MCP",
		Type:        "consumed",
		Endpoint:    a.synapsetProxy.baseURL + "/mcp",
		Description: "Synapset semantic memory server — store, search, and manage cross-session knowledge.",
	}

	client := &http.Client{Timeout: 3 * time.Second}
	statusURL := a.synapsetProxy.baseURL + "/api/status"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, statusURL, http.NoBody) //nolint:gosec // G704: URL is from trusted baseURL config
	if err != nil {
		return info
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req) //nolint:gosec // G107: URL is built from configured baseURL, not user input
	if err != nil {
		return info
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		info.Healthy = true
	}
	return info
}
