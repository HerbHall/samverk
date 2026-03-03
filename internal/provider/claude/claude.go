// Package claude implements the provider.Provider interface using
// the Anthropic Claude Messages API over raw net/http (no SDK dependency).
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/herbhall/samverk/internal/provider"
)

// Compile-time check that Client satisfies provider.Provider.
var _ provider.Provider = (*Client)(nil)

const (
	defaultBaseURL   = "https://api.anthropic.com"
	defaultMaxTokens = 4096
	defaultTimeout   = 120 * time.Second
	apiVersion       = "2023-06-01"
)

// Client is an HTTP client for the Anthropic Claude Messages API.
type Client struct {
	baseURL    string
	apiKey     string
	model      string
	maxTokens  int
	httpClient *http.Client
}

// Option is a functional option for the Client.
type Option func(*Client)

// WithBaseURL overrides the default API base URL.
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(url, "/")
	}
}

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// WithMaxTokens overrides the default max_tokens for requests.
func WithMaxTokens(n int) Option {
	return func(c *Client) {
		c.maxTokens = n
	}
}

// New creates a new Claude API client.
func New(apiKey, model string, opts ...Option) *Client {
	c := &Client{
		baseURL:   defaultBaseURL,
		apiKey:    apiKey,
		model:     model,
		maxTokens: defaultMaxTokens,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// claudeRequest is the request body for the Claude Messages API.
type claudeRequest struct {
	Model     string            `json:"model"`
	MaxTokens int               `json:"max_tokens"`
	System    string            `json:"system,omitempty"`
	Messages  []claudeMessage   `json:"messages"`
}

// claudeMessage is a message in the Claude Messages API format.
type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeResponse is the response body from the Claude Messages API.
type claudeResponse struct {
	Content []contentBlock `json:"content"`
	Model   string         `json:"model"`
	Usage   claudeUsage    `json:"usage"`
}

// contentBlock is a single content block in a Claude response.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// claudeUsage contains token counts from the Claude API.
type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Chat sends a chat completion request to POST /v1/messages.
// System messages are extracted from req.Messages and placed in the
// top-level "system" field as required by the Claude API.
func (c *Client) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = c.model
	}

	// Extract system messages and convert remaining to Claude format.
	var systemParts []string
	messages := make([]claudeMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == provider.RoleSystem {
			systemParts = append(systemParts, m.Content)
			continue
		}
		messages = append(messages, claudeMessage{
			Role:    string(m.Role),
			Content: m.Content,
		})
	}

	cr := claudeRequest{
		Model:     model,
		MaxTokens: c.maxTokens,
		System:    strings.Join(systemParts, "\n"),
		Messages:  messages,
	}

	var resp claudeResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/messages", cr, &resp); err != nil {
		return nil, fmt.Errorf("claude: chat: %w", err)
	}

	// Concatenate all text content blocks into a single response.
	var textParts []string
	for _, block := range resp.Content {
		if block.Type == "text" {
			textParts = append(textParts, block.Text)
		}
	}

	return &provider.ChatResponse{
		Model: resp.Model,
		Message: provider.Message{
			Role:    provider.RoleAssistant,
			Content: strings.Join(textParts, ""),
		},
		PromptTokens:     resp.Usage.InputTokens,
		CompletionTokens: resp.Usage.OutputTokens,
	}, nil
}

// Healthy returns true if the Claude API is reachable.
// It sends a minimal Chat request with 1 max_token.
func (c *Client) Healthy(ctx context.Context) bool {
	cr := claudeRequest{
		Model:     c.model,
		MaxTokens: 1,
		Messages: []claudeMessage{
			{Role: "user", Content: "ping"},
		},
	}

	var resp claudeResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/messages", cr, &resp); err != nil {
		return false
	}
	return true
}

// Name returns the provider identifier.
func (c *Client) Name() string {
	return "claude"
}

// doJSON is the generic HTTP helper. It marshals body to JSON (if non-nil),
// makes a request with the given context, sets Claude-specific headers,
// checks the status code, and unmarshals the response into result.
func (c *Client) doJSON(ctx context.Context, method, path string, body, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	if bodyReader == nil {
		bodyReader = http.NoBody
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", apiVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL is from trusted baseURL config
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err = json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}
