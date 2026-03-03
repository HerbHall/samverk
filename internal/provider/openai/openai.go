// Package openai implements the provider.Provider interface using
// the OpenAI Chat Completions API over raw net/http (no SDK dependency).
package openai

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

// Client is an HTTP client for the OpenAI Chat Completions API.
type Client struct {
	baseURL    string
	apiKey     string
	model      string
	maxTokens  int
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the default OpenAI API base URL.
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

// WithMaxTokens overrides the default max_tokens value.
func WithMaxTokens(n int) Option {
	return func(c *Client) {
		c.maxTokens = n
	}
}

// New creates a new OpenAI client with the given API key and model.
func New(apiKey, model string, opts ...Option) *Client {
	c := &Client{
		baseURL:   "https://api.openai.com",
		apiKey:    apiKey,
		model:     model,
		maxTokens: 4096,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// chatRequest is the OpenAI Chat Completions request body.
type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens"`
}

// chatMessage is a single message in the OpenAI format.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the OpenAI Chat Completions response body.
type chatResponse struct {
	Choices []choice `json:"choices"`
	Model   string   `json:"model"`
	Usage   usage    `json:"usage"`
}

// choice is a single completion choice.
type choice struct {
	Message chatMessage `json:"message"`
}

// usage contains token usage statistics.
type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// Chat sends a chat completion request to POST /v1/chat/completions.
func (c *Client) Chat(ctx context.Context, req provider.ChatRequest) (resp *provider.ChatResponse, err error) {
	// Convert provider messages to OpenAI format (role maps 1:1).
	msgs := make([]chatMessage, 0, len(req.Messages))
	for i := range req.Messages {
		msgs = append(msgs, chatMessage{
			Role:    string(req.Messages[i].Role),
			Content: req.Messages[i].Content,
		})
	}

	oaiReq := chatRequest{
		Model:     c.model,
		Messages:  msgs,
		MaxTokens: c.maxTokens,
	}

	var oaiResp chatResponse
	if err = c.doJSON(ctx, http.MethodPost, "/v1/chat/completions", oaiReq, &oaiResp); err != nil {
		return nil, fmt.Errorf("openai: chat: %w", err)
	}

	if len(oaiResp.Choices) == 0 {
		return nil, fmt.Errorf("openai: chat: empty choices array")
	}

	resp = &provider.ChatResponse{
		Model: oaiResp.Model,
		Message: provider.Message{
			Role:    provider.Role(oaiResp.Choices[0].Message.Role),
			Content: oaiResp.Choices[0].Message.Content,
		},
		PromptTokens:     oaiResp.Usage.PromptTokens,
		CompletionTokens: oaiResp.Usage.CompletionTokens,
	}
	return resp, nil
}

// Healthy returns true if the OpenAI API is reachable.
// It checks GET /v1/models which returns 200 with a list of available models.
func (c *Client) Healthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", http.NoBody)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL is from trusted baseURL config
	if err != nil {
		return false
	}
	_ = resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// Name returns the provider identifier.
func (c *Client) Name() string {
	return "openai"
}

// doJSON is the generic HTTP helper. It marshals body to JSON (if non-nil),
// makes a request with the given context, sets auth and content-type headers,
// checks the status code, and unmarshals the response into result (if non-nil).
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

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
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
