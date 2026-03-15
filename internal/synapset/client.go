// Package synapset provides a client for the Synapset MCP-based semantic
// memory service. It handles session initialization and exposes search/store
// operations via the MCP JSON-RPC 2.0 protocol over Streamable HTTP.
//
// The client degrades gracefully: if Synapset is unreachable or returns an
// error, operations log a warning and return empty results rather than
// blocking the caller.
package synapset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DefaultURL is used when neither config nor env var specifies a Synapset endpoint.
const DefaultURL = "https://synapset.herbhall.net/mcp"

// DefaultPool is the default pool name for agent memories.
const DefaultPool = "samverk"

// defaultTimeout is the HTTP request timeout for Synapset calls.
const defaultTimeout = 10 * time.Second

// postInitDelay is a small pause after session initialization to allow
// the MCP server (behind Cloudflare tunnels) to stabilize before the
// first tool call. The user explicitly prefers accuracy over speed.
const postInitDelay = 150 * time.Millisecond

// bodyPreviewLen is the max number of bytes to include from a response
// body when reporting parse errors, for diagnostic purposes.
const bodyPreviewLen = 200

// Memory represents a single memory entry returned by Synapset.
type Memory struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Category   string  `json:"category"`
	Tags       []string `json:"tags"`
	Source     string  `json:"source"`
	Similarity float64 `json:"similarity"`
}

// Config holds Synapset client configuration.
type Config struct {
	URL  string // MCP endpoint URL
	Pool string // default pool for agent memories
}

// ConfigFromEnv builds a Config from environment variables, falling back
// to the provided defaults for any unset value.
func ConfigFromEnv(defaults Config) Config {
	if env := os.Getenv("SYNAPSET_URL"); env != "" {
		defaults.URL = env
	}
	if defaults.URL == "" {
		defaults.URL = DefaultURL
	}
	if env := os.Getenv("SYNAPSET_POOL"); env != "" {
		defaults.Pool = env
	}
	if defaults.Pool == "" {
		defaults.Pool = DefaultPool
	}
	return defaults
}

// Client communicates with a Synapset MCP server over Streamable HTTP.
type Client struct {
	baseURL    string
	pool       string
	httpClient *http.Client
	logger     *zap.Logger

	mu        sync.Mutex
	sessionID string // MCP session ID from initialize handshake
	nextID    int    // JSON-RPC request ID counter
}

// New creates a Synapset client with the given configuration.
func New(cfg Config, logger *zap.Logger) *Client {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.URL == "" {
		cfg.URL = DefaultURL
	}
	if cfg.Pool == "" {
		cfg.Pool = DefaultPool
	}
	return &Client{
		baseURL: cfg.URL,
		pool:    cfg.Pool,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		logger: logger,
		nextID: 1,
	}
}

// Pool returns the configured default pool name.
func (c *Client) Pool() string {
	return c.pool
}

// jsonRPCRequest is a JSON-RPC 2.0 request envelope.
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      *int        `json:"id,omitempty"` // nil for notifications
}

// jsonRPCResponse is a JSON-RPC 2.0 response envelope.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
	ID      *int            `json:"id,omitempty"`
}

// jsonRPCError is a JSON-RPC 2.0 error object.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// toolCallParams wraps the tools/call method parameters.
type toolCallParams struct {
	Name      string      `json:"name"`
	Arguments interface{} `json:"arguments"`
}

// toolResult is the outer result of a tools/call response.
type toolResult struct {
	Content []toolContent `json:"content"`
}

// toolContent is a single content block in a tool result.
type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// initializeParams are sent with the MCP initialize method.
type initializeParams struct {
	ProtocolVersion string           `json:"protocolVersion"`
	Capabilities    json.RawMessage  `json:"capabilities"`
	ClientInfo      initializeClient `json:"clientInfo"`
}

// initializeClient identifies the MCP client.
type initializeClient struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ensureSession initializes an MCP session if one is not already active.
// On success it stores the session ID for subsequent requests.
func (c *Client) ensureSession(ctx context.Context) error {
	c.mu.Lock()
	if c.sessionID != "" {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	// Step 1: Send initialize request.
	id := c.allocID()
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params: initializeParams{
			ProtocolVersion: "2025-03-26",
			Capabilities:    json.RawMessage(`{}`),
			ClientInfo: initializeClient{
				Name:    "samverk-agent",
				Version: "1.0.0",
			},
		},
		ID: &id,
	}

	resp, sessionID, err := c.doRequest(ctx, req)
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("initialize: %w", resp.Error)
	}
	if sessionID == "" {
		return fmt.Errorf("initialize: no Mcp-Session-Id in response")
	}

	c.mu.Lock()
	c.sessionID = sessionID
	c.mu.Unlock()

	// Step 2: Send initialized notification (no ID = notification).
	notif := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	if _, _, err = c.doRequestWithSession(ctx, notif, sessionID); err != nil {
		// Non-fatal: session may still work.
		c.logger.Warn("synapset: initialized notification failed", zap.Error(err))
	}

	// Step 3: Brief pause to let the server (and any tunnel layer) stabilize.
	select {
	case <-time.After(postInitDelay):
	case <-ctx.Done():
		return ctx.Err()
	}

	// Step 4: Validate session with a tools/list ping before real work.
	listID := c.allocID()
	listReq := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/list",
		ID:      &listID,
	}
	if listResp, _, listErr := c.doRequestWithSession(ctx, listReq, sessionID); listErr != nil {
		c.logger.Warn("synapset: tools/list validation ping failed", zap.Error(listErr))
	} else if listResp != nil && listResp.Error != nil {
		c.logger.Warn("synapset: tools/list returned error", zap.String("error", listResp.Error.Message))
	}

	return nil
}

// resetSession clears the cached session ID so the next call re-initializes.
func (c *Client) resetSession() {
	c.mu.Lock()
	c.sessionID = ""
	c.mu.Unlock()
}

// allocID returns the next JSON-RPC request ID.
func (c *Client) allocID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID
	c.nextID++
	return id
}

// getSessionID returns the current session ID (thread-safe).
func (c *Client) getSessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

// doRequest sends a JSON-RPC request and returns the response plus the
// Mcp-Session-Id header (if present).
func (c *Client) doRequest(ctx context.Context, rpcReq jsonRPCRequest) (rpcResp *jsonRPCResponse, sessionID string, err error) {
	return c.doRequestWithSession(ctx, rpcReq, "")
}

// doRequestWithSession sends a JSON-RPC request with an optional session header.
func (c *Client) doRequestWithSession(ctx context.Context, rpcReq jsonRPCRequest, session string) (rpcResp *jsonRPCResponse, sessionID string, err error) {
	body, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if session != "" {
		httpReq.Header.Set("Mcp-Session-Id", session)
	}

	httpResp, err := c.httpClient.Do(httpReq) //nolint:gosec // G704: URL is from trusted baseURL config
	if err != nil {
		return nil, "", fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	sessionID = httpResp.Header.Get("Mcp-Session-Id")

	// Notifications may return 202/204 with no body.
	if rpcReq.ID == nil {
		return nil, sessionID, nil
	}

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, sessionID, fmt.Errorf("http %d: %s", httpResp.StatusCode, bodyPreview(respBody))
	}

	// Read the full body so we can include a preview in error messages.
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, sessionID, fmt.Errorf("read response body: %w", err)
	}

	// Check Content-Type: if not JSON, report the body rather than
	// letting json.Decode produce a cryptic "invalid character" error.
	ct := httpResp.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(ct, "application/json") {
		return nil, sessionID, fmt.Errorf("unexpected content-type %q: %s", ct, bodyPreview(respBody))
	}

	var result jsonRPCResponse
	if err = json.Unmarshal(respBody, &result); err != nil {
		return nil, sessionID, fmt.Errorf("decode response: %w (body: %s)", err, bodyPreview(respBody))
	}
	return &result, sessionID, nil
}

// callTool sends a tools/call request with automatic session management.
// If the session is expired (HTTP 404/410), it re-initializes and retries once.
func (c *Client) callTool(ctx context.Context, toolName string, args interface{}) (*toolResult, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}

	result, err := c.doToolCall(ctx, toolName, args)
	if err != nil {
		// Session may have expired; retry with a fresh session.
		c.resetSession()
		if retryErr := c.ensureSession(ctx); retryErr != nil {
			return nil, fmt.Errorf("session re-init: %w", retryErr)
		}
		result, err = c.doToolCall(ctx, toolName, args)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// doToolCall performs a single tools/call request.
func (c *Client) doToolCall(ctx context.Context, toolName string, args interface{}) (*toolResult, error) {
	id := c.allocID()
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: toolCallParams{
			Name:      toolName,
			Arguments: args,
		},
		ID: &id,
	}

	resp, _, err := c.doRequestWithSession(ctx, req, c.getSessionID())
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}

	var result toolResult
	if err = json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal tool result: %w", err)
	}
	return &result, nil
}

// SearchMemory searches a specific pool for memories matching the query.
func (c *Client) SearchMemory(ctx context.Context, pool, query string, limit int) (memories []Memory, err error) {
	if limit <= 0 {
		limit = 5
	}
	args := map[string]interface{}{
		"pool": pool,
		"query":     query,
		"limit":     limit,
	}
	result, err := c.callTool(ctx, "search_memory", args)
	if err != nil {
		return nil, err
	}
	return parseMemories(result)
}

// SearchAll searches across all pools for memories matching the query.
func (c *Client) SearchAll(ctx context.Context, query string, limit int) (memories []Memory, err error) {
	if limit <= 0 {
		limit = 5
	}
	args := map[string]interface{}{
		"query": query,
		"limit": limit,
	}
	result, err := c.callTool(ctx, "search_all", args)
	if err != nil {
		return nil, err
	}
	return parseMemories(result)
}

// StoreMemory saves a memory entry to the specified pool.
func (c *Client) StoreMemory(ctx context.Context, pool, content, category string, tags []string, source string) error {
	args := map[string]interface{}{
		"pool": pool,
		"content":   content,
		"category":  category,
	}
	if len(tags) > 0 {
		args["tags"] = tags
	}
	if source != "" {
		args["source"] = source
	}
	_, err := c.callTool(ctx, "store_memory", args)
	return err
}

// parseMemories extracts Memory entries from a tool result's text content.
func parseMemories(result *toolResult) ([]Memory, error) {
	if result == nil || len(result.Content) == 0 {
		return nil, nil
	}
	for i := range result.Content {
		if result.Content[i].Type != "text" {
			continue
		}
		var memories []Memory
		if err := json.Unmarshal([]byte(result.Content[i].Text), &memories); err != nil {
			// Try parsing as a wrapper object with a "memories" field.
			var wrapper struct {
				Memories []Memory `json:"memories"`
			}
			if wrapErr := json.Unmarshal([]byte(result.Content[i].Text), &wrapper); wrapErr == nil && len(wrapper.Memories) > 0 {
				return wrapper.Memories, nil
			}
			// Try single-object response.
			var single Memory
			if singleErr := json.Unmarshal([]byte(result.Content[i].Text), &single); singleErr == nil && single.Content != "" {
				return []Memory{single}, nil
			}
			return nil, fmt.Errorf("parse memories: %w (body: %s)", err, bodyPreview([]byte(result.Content[i].Text)))
		}
		return memories, nil
	}
	return nil, nil
}

// bodyPreview returns the first bodyPreviewLen bytes of data as a string,
// trimmed of whitespace, for inclusion in error messages.
func bodyPreview(data []byte) string {
	s := strings.TrimSpace(string(data))
	if len(s) > bodyPreviewLen {
		return s[:bodyPreviewLen] + "..."
	}
	return s
}
