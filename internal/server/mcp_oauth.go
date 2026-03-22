package server

import (
	"io"
	"net/http"
)

// MCPOAuthConfig holds the Cloudflare Access OIDC parameters needed to proxy
// OAuth requests from Claude.ai to the CF Access authorization server.
type MCPOAuthConfig struct {
	TeamDomain   string // e.g. herbhall.cloudflareaccess.com
	ClientID     string // CF Access self-hosted app aud_tag (for JWT validation on /mcp)
	SAASClientID string // CF Access SAAS app client_id (used in OIDC proxy path for /authorize and /token)
}

// oauthServerMeta is the JSON body for GET /.well-known/oauth-authorization-server.
type oauthServerMeta struct {
	Issuer                        string   `json:"issuer"`
	AuthorizationEndpoint         string   `json:"authorization_endpoint"`
	TokenEndpoint                 string   `json:"token_endpoint"`
	ResponseTypesSupported        []string `json:"response_types_supported"`
	GrantTypesSupported           []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
}

// RegisterMCPOAuthRoutes registers the three OAuth endpoints needed for Claude.ai
// custom connector authentication via Cloudflare Access OIDC. It is a no-op when
// either TeamDomain or ClientID is empty.
//
// Route specificity: Go 1.22+ ServeMux gives exact patterns priority over subtree
// patterns, so "GET /.well-known/oauth-authorization-server" wins over the existing
// "GET /.well-known/" catch-all 404.
func RegisterMCPOAuthRoutes(mux *http.ServeMux, cfg MCPOAuthConfig) {
	if cfg.TeamDomain == "" || cfg.ClientID == "" || cfg.SAASClientID == "" {
		return
	}
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", cfg.handleOAuthMeta)
	mux.HandleFunc("GET /authorize", cfg.handleAuthorize)
	mux.HandleFunc("POST /token", cfg.handleToken)
}

// handleOAuthMeta serves the OAuth 2.0 Authorization Server Metadata document.
// Claude.ai fetches this to discover the authorization and token endpoints.
func (cfg MCPOAuthConfig) handleOAuthMeta(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, oauthServerMeta{ //nolint:gosec // G101: fields contain OAuth endpoint URLs, not credentials
		Issuer:                        "https://samverk.herbhall.net",
		AuthorizationEndpoint:         "https://samverk.herbhall.net/authorize",
		TokenEndpoint:                 "https://samverk.herbhall.net/token",
		ResponseTypesSupported:        []string{"code"},
		GrantTypesSupported:           []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported: []string{"S256"},
	})
}

// handleAuthorize proxies Claude.ai's OAuth authorization request to the
// Cloudflare Access OIDC authorization endpoint. CF Access handles Google auth
// and redirects back to https://claude.ai/api/mcp/auth_callback.
func (cfg MCPOAuthConfig) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	// SAASClientID is the CF Access SAAS app client_id, used in the OIDC path.
	// ClientID is the self-hosted app AUD, used only for JWT validation — not here.
	target := "https://" + cfg.TeamDomain + "/cdn-cgi/access/sso/oidc/" + cfg.SAASClientID + "/authorization"
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// handleToken proxies Claude.ai's token exchange POST to the Cloudflare Access
// OIDC token endpoint and pipes the response back unchanged.
func (cfg MCPOAuthConfig) handleToken(w http.ResponseWriter, r *http.Request) {
	target := "https://" + cfg.TeamDomain + "/cdn-cgi/access/sso/oidc/" + cfg.SAASClientID + "/token"

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to build token request"})
		return
	}
	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))

	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G704: URL is constructed from the trusted CF_ACCESS_TEAM_DOMAIN config value
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "token exchange failed"})
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
