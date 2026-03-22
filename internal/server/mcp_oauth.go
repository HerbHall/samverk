package server

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"
)

// mcpTokenClient is a dedicated HTTP client for the /token proxy that does NOT
// follow redirects. CF Access returns 302 to the redirect_uri on error; we
// convert those to JSON errors instead of blindly following them to Claude.ai.
var mcpTokenClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// MCPOAuthConfig holds the Cloudflare Access OIDC parameters needed to proxy
// OAuth requests from Claude.ai to the CF Access authorization server.
type MCPOAuthConfig struct {
	TeamDomain       string // e.g. herbhall.cloudflareaccess.com
	ClientID         string // CF Access self-hosted app aud_tag (for JWT validation on /mcp)
	SAASClientID     string // CF Access SAAS app client_id (used in OIDC proxy path for /authorize and /token)
	SAASClientSecret string // CF Access SAAS app client_secret (injected server-side into token exchange)
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
// any required field is empty.
//
// Route specificity: Go 1.22+ ServeMux gives exact patterns priority over subtree
// patterns, so "GET /.well-known/oauth-authorization-server" wins over the existing
// "GET /.well-known/" catch-all 404.
func RegisterMCPOAuthRoutes(mux *http.ServeMux, cfg MCPOAuthConfig) {
	if cfg.TeamDomain == "" || cfg.ClientID == "" || cfg.SAASClientID == "" || cfg.SAASClientSecret == "" {
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

	// Forward all query params but translate scope: Claude.ai sends scope=claudeai,
	// CF Access requires openid in the scope (per its OIDC discovery document).
	q := r.URL.Query()
	q.Set("scope", "openid email")
	target += "?" + q.Encode()

	http.Redirect(w, r, target, http.StatusFound)
}

// handleToken proxies Claude.ai's token exchange POST to the Cloudflare Access
// OIDC token endpoint, injecting the client_secret server-side.
// Claude.ai uses PKCE and does not send the client_secret; CF Access requires it.
// The secret is held only on the server — never exposed to clients.
func (cfg MCPOAuthConfig) handleToken(w http.ResponseWriter, r *http.Request) {
	target := "https://" + cfg.TeamDomain + "/cdn-cgi/access/sso/oidc/" + cfg.SAASClientID + "/token"

	// Read and parse the incoming form body so we can inject the client_secret.
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to read request body"})
		return
	}

	params, err := url.ParseQuery(string(bodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	// Inject correct client_id and client_secret server-side.
	// The connector's configured client_id may differ from the SAAS app's actual client_id;
	// the secret is held server-only and never exposed to clients.
	params.Set("client_id", cfg.SAASClientID)
	params.Set("client_secret", cfg.SAASClientSecret) //nolint:gosec // G101: value is a runtime config secret, not hardcoded

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, strings.NewReader(params.Encode()))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to build token request"})
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := mcpTokenClient.Do(req) //nolint:gosec // G704: URL is constructed from the trusted CF_ACCESS_TEAM_DOMAIN config value
	if err != nil {
		zap.L().Error("mcp_oauth: token exchange HTTP error", zap.Error(err))
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "token exchange failed"})
		return
	}
	defer func() { _ = resp.Body.Close() }()

	zap.L().Info("mcp_oauth: CF Access token response", zap.Int("status", resp.StatusCode))

	// CF Access returns 302 to redirect_uri on error (non-standard but observed behavior).
	// Convert to a JSON error so Claude.ai receives a proper OAuth error response.
	if resp.StatusCode == http.StatusFound {
		loc := resp.Header.Get("Location")
		oauthErr := "token_exchange_error"
		if loc != "" {
			if u, parseErr := url.Parse(loc); parseErr == nil {
				if e := u.Query().Get("error"); e != "" {
					oauthErr = e
				}
				if desc := u.Query().Get("error_description"); desc != "" {
					oauthErr += ": " + desc
				}
			}
		}
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: oauthErr})
		return
	}

	// For non-200 responses, log the body to help diagnose failures.
	// For 200, pipe directly without reading (contains the access token — do not log).
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		zap.L().Warn("mcp_oauth: CF Access token non-200",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(body)),
		)
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, bytes.NewReader(body))
		return
	}

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
