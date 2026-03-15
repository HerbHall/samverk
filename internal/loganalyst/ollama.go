package loganalyst

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaAnalystHTTP is a minimal HTTP client for the Ollama /api/generate
// endpoint. It does not stream; it waits for the full response.
type OllamaAnalystHTTP struct {
	url    string
	model  string
	client *http.Client
}

// NewOllamaAnalystClient creates a client that calls the Ollama generate API.
func NewOllamaAnalystClient(url, model string, timeout time.Duration) *OllamaAnalystHTTP {
	return &OllamaAnalystHTTP{
		url:   url,
		model: model,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// ollamaRequest is the JSON body sent to /api/generate.
type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// ollamaResponse is the JSON body received from /api/generate (non-streaming).
type ollamaResponse struct {
	Response string `json:"response"`
}

// Generate sends a prompt to Ollama and returns the generated text.
func (c *OllamaAnalystHTTP) Generate(ctx context.Context, prompt string) (text string, err error) {
	body, err := json.Marshal(ollamaRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false,
	})
	if err != nil {
		return "", fmt.Errorf("marshal ollama request: %w", err)
	}

	endpoint := c.url + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body)) //nolint:gosec // G107: URL is from trusted config
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result ollamaResponse
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}

	return result.Response, nil
}
