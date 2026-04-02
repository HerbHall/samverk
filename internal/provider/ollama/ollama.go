// Package ollama implements the provider.Provider interface using
// the Ollama REST API over raw net/http (no SDK dependency).
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"

	"samverk.dev/samverk/internal/provider"
)

// Compile-time checks.
var (
	_ provider.Provider       = (*Client)(nil)
	_ provider.HealthDetailer = (*Client)(nil)
)

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
		dur := time.Since(start)
		c.logger.Error("ollama chat failed",
			zap.String("model", req.Model),
			zap.String("base_url", c.baseURL),
			zap.Int64("duration_ms", dur.Milliseconds()),
			zap.Error(err),
		)
		// Wrap context deadline/cancellation as ErrProviderTimeout so the
		// pool's IsRetryable check triggers failover to the next provider.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, &provider.ErrProviderTimeout{
				Provider:    "ollama",
				Model:       req.Model,
				TimeoutType: provider.TimeoutStale,
				Duration:    dur,
			}
		}
		// Wrap transport-level failures (connection refused, EOF, network
		// errors) as ErrProviderUnavailable so the pool triggers failover.
		// These occur when Ollama is down, not yet started, or unreachable.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return nil, &provider.ErrProviderUnavailable{
				Provider: "ollama",
				Cause:    err,
			}
		}
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

// healthTimeout is the maximum time for a health probe. Shorter than the
// default HTTP client timeout (30s) to avoid blocking the health monitor.
const healthTimeout = 5 * time.Second

// Healthy returns true if the Ollama server is reachable.
// It checks GET / which returns "Ollama is running" on success.
// Uses a 5-second timeout independent of the main HTTP client timeout.
func (c *Client) Healthy(ctx context.Context) bool {
	hctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(hctx, http.MethodGet, c.baseURL+"/", http.NoBody)
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

// OllamaHealth contains detailed health information from an Ollama instance.
type OllamaHealth struct {
	ModelLoaded bool   `json:"model_loaded"`
	VRAMFree    int64  `json:"vram_free"`
	VRAMTotal   int64  `json:"vram_total"`
	ModelName   string `json:"model_name,omitempty"`
}

// tagsResponse wraps the /api/tags response.
type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// HealthDetail returns extended health information: whether the configured
// model is available and VRAM usage from running models. Implements
// provider.HealthDetailer.
func (c *Client) HealthDetail(ctx context.Context) (*provider.HealthDetail, error) {
	hctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()

	// Check available models via /api/tags.
	var tags tagsResponse
	if err := c.doJSON(hctx, http.MethodGet, "/api/tags", nil, &tags); err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}

	// Check running models via /api/ps for VRAM usage.
	running, err := c.ListRunning(hctx)
	if err != nil {
		return nil, fmt.Errorf("list running: %w", err)
	}

	detail := &provider.HealthDetail{}

	// Check if any model is loaded (tags lists all pulled models).
	if len(tags.Models) > 0 {
		detail.ModelLoaded = true
	}

	// Sum VRAM usage from running models and check GPU offloading.
	var totalVRAM int64
	for i := range running {
		totalVRAM += running[i].SizeVRAM
	}
	// VRAMTotal is the total VRAM consumed by loaded models.
	// VRAMFree cannot be determined from Ollama's API alone, so we
	// report total VRAM in use. The dashboard can interpret this.
	detail.VRAMTotal = totalVRAM

	// GPUOffloaded is true if any running model has VRAM allocated,
	// meaning inference is at least partially on GPU. SizeVRAM==0 on a
	// running model means the model is fully CPU-resident.
	detail.GPUOffloaded = len(running) > 0 && totalVRAM > 0

	return detail, nil
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
