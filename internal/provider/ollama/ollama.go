// Package ollama implements the provider.Provider interface using
// the Ollama REST API over raw net/http (no SDK dependency).
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/provider"
)

// Compile-time check that Client satisfies provider.Provider.
var _ provider.Provider = (*Client)(nil)

// Client is an HTTP client for the Ollama REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
}

// Option configures a Client.
type Option func(*Client)

// WithLogger sets a structured logger for request/response logging.
func WithLogger(l *zap.Logger) Option {
	return func(c *Client) {
		c.logger = l
	}
}

// New creates a new Ollama client targeting the given base URL
// (e.g., "http://localhost:11434") with a default 30-second timeout.
func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		logger:  zap.NewNop(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewWithTimeout creates a new Ollama client targeting the given base URL
// with a custom HTTP client timeout. Use this when models may be cold
// (evicted from VRAM) and require extra time to load before responding.
func NewWithTimeout(baseURL string, timeout time.Duration, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		logger:  zap.NewNop(),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Chat sends a chat completion request to POST /api/chat.
// Stream is always forced to false.
func (c *Client) Chat(ctx context.Context, req provider.ChatRequest) (resp *provider.ChatResponse, err error) {
	start := time.Now()

	c.logger.Info("ollama chat request",
		zap.String("model", req.Model),
		zap.Int("message_count", len(req.Messages)),
		zap.String("base_url", c.baseURL),
	)

	req.Stream = false

	resp = &provider.ChatResponse{}
	if err = c.doJSON(ctx, http.MethodPost, "/api/chat", req, resp); err != nil {
		c.logger.Error("ollama chat failed",
			zap.String("model", req.Model),
			zap.String("base_url", c.baseURL),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("ollama: chat: %w", err)
	}

	c.logger.Info("ollama chat response",
		zap.String("model", resp.Model),
		zap.Int("prompt_tokens", resp.PromptTokens),
		zap.Int("completion_tokens", resp.CompletionTokens),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	)

	return resp, nil
}

// Healthy returns true if the Ollama server is reachable.
// It checks GET / which returns "Ollama is running" on success.
func (c *Client) Healthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/", http.NoBody)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL is from trusted baseURL config
	if err != nil {
		return false
	}
	_ = resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// Name returns the provider identifier.
func (c *Client) Name() string {
	return "ollama"
}

// GenerateRequest contains the parameters for a text generation request.
type GenerateRequest struct {
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	Stream    bool   `json:"stream"`
	KeepAlive string `json:"keep_alive,omitempty"`
}

// GenerateResponse contains the result of a text generation request.
type GenerateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`

	PromptTokens     int `json:"prompt_eval_count,omitempty"`
	CompletionTokens int `json:"eval_count,omitempty"`
}

// Generate sends a text generation request to POST /api/generate.
// Stream is always forced to false.
func (c *Client) Generate(ctx context.Context, req GenerateRequest) (resp *GenerateResponse, err error) {
	req.Stream = false

	resp = &GenerateResponse{}
	if err = c.doJSON(ctx, http.MethodPost, "/api/generate", req, resp); err != nil {
		return nil, fmt.Errorf("ollama: generate: %w", err)
	}
	return resp, nil
}

// Preload loads a model into memory with the given keep-alive duration.
// It sends an empty prompt to /api/generate.
func (c *Client) Preload(ctx context.Context, model, keepAlive string) error {
	req := GenerateRequest{
		Model:     model,
		Prompt:    "",
		Stream:    false,
		KeepAlive: keepAlive,
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/generate", req, &GenerateResponse{}); err != nil {
		return fmt.Errorf("ollama: preload %s: %w", model, err)
	}
	return nil
}

// Unload evicts a model from memory by setting keep_alive to "0".
func (c *Client) Unload(ctx context.Context, model string) error {
	req := GenerateRequest{
		Model:     model,
		Prompt:    "",
		Stream:    false,
		KeepAlive: "0",
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/generate", req, &GenerateResponse{}); err != nil {
		return fmt.Errorf("ollama: unload %s: %w", model, err)
	}
	return nil
}

// RunningModel describes a model currently loaded in Ollama's memory.
type RunningModel struct {
	Name      string `json:"name"`
	Model     string `json:"model"`
	Size      int64  `json:"size"`
	SizeVRAM  int64  `json:"size_vram"`
	ExpiresAt string `json:"expires_at"`
}

// psResponse wraps the /api/ps response.
type psResponse struct {
	Models []RunningModel `json:"models"`
}

// ListRunning returns the models currently loaded in Ollama's memory
// via GET /api/ps.
func (c *Client) ListRunning(ctx context.Context) ([]RunningModel, error) {
	var ps psResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/ps", nil, &ps); err != nil {
		return nil, fmt.Errorf("ollama: list running: %w", err)
	}
	return ps.Models, nil
}

// pullRequest wraps the /api/pull request body.
type pullRequest struct {
	Name   string `json:"name"`
	Stream bool   `json:"stream"`
}

// Pull downloads a model from the Ollama registry.
// Stream is always false (waits for completion).
func (c *Client) Pull(ctx context.Context, model string) error {
	req := pullRequest{
		Name:   model,
		Stream: false,
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/pull", req, &json.RawMessage{}); err != nil {
		return fmt.Errorf("ollama: pull %s: %w", model, err)
	}
	return nil
}

// doJSON is the generic HTTP helper. It marshals body to JSON (if non-nil),
// makes a request with the given context, checks the status code, and
// unmarshals the response into result (if non-nil).
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
