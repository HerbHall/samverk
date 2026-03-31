package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// chatRateLimit tracks per-session request counts for rate limiting.
type chatRateLimit struct {
	mu       sync.Mutex
	requests map[string][]time.Time // session token -> request timestamps
}

func newChatRateLimit() *chatRateLimit {
	return &chatRateLimit{
		requests: make(map[string][]time.Time),
	}
}

// chatMaxRequestsPerMinute is the maximum number of chat requests per minute per session.
const chatMaxRequestsPerMinute = 10

// allow checks whether the given session is within the rate limit.
// It prunes expired timestamps and returns true if the request is allowed.
func (rl *chatRateLimit) allow(session string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Minute)

	// Prune old entries.
	timestamps := rl.requests[session]
	pruned := make([]time.Time, 0, len(timestamps))
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}

	if len(pruned) >= chatMaxRequestsPerMinute {
		rl.requests[session] = pruned
		return false
	}

	rl.requests[session] = append(pruned, now)
	return true
}

// chatMessage is a single message in the chat conversation.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the JSON body accepted by POST /api/v1/chat.
type chatRequest struct {
	Messages []chatMessage `json:"messages"`
	System   string        `json:"system"`
}

// anthropicMessage mirrors the Anthropic API message format.
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicRequest is the request body sent to the Anthropic Messages API.
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

// anthropicContentBlock represents a content block in the Anthropic response.
type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// anthropicResponse is the response from the Anthropic Messages API.
type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Role    string                  `json:"role"`
	Model   string                  `json:"model"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// chatResponse is the JSON body returned by POST /api/v1/chat.
type chatResponse struct {
	Content     string `json:"content"`
	Model       string `json:"model"`
	InputTokens int    `json:"input_tokens"`
	OutputTokens int   `json:"output_tokens"`
}

const (
	anthropicAPIURL     = "https://api.anthropic.com/v1/messages"
	anthropicAPIVersion = "2023-06-01"
	chatModel           = "claude-sonnet-4-20250514"
	chatMaxTokens       = 1024
)

// SetChatAPIKey configures the Anthropic API key for the chat proxy.
// If not called, the handler falls back to the ANTHROPIC_API_KEY env var.
func (a *API) SetChatAPIKey(key string) {
	a.chatAPIKey = key
}

// handleChat proxies chat requests to the Anthropic Messages API.
// The API key is kept server-side and never exposed to the browser.
func (a *API) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	// Resolve API key: explicit config only. No env var fallback --
	// the fleet uses OAuth (Max plan) for Claude CLI, not API keys.
	apiKey := a.chatAPIKey
	if apiKey == "" {
		writeError(w, http.StatusServiceUnavailable, "chat not configured: no API key")
		return
	}

	// Rate limit by Authorization header (or remote address as fallback).
	session := r.Header.Get("Authorization")
	if session == "" {
		session = r.RemoteAddr
	}
	if a.chatRateLimit != nil && !a.chatRateLimit.allow(session) {
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded: max 10 requests/minute")
		return
	}

	// Parse request body.
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages array is required")
		return
	}

	// Build Anthropic API request.
	messages := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, anthropicMessage(m))
	}

	anthropicReq := anthropicRequest{
		Model:     chatModel,
		MaxTokens: chatMaxTokens,
		System:    req.System,
		Messages:  messages,
	}

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		a.logger.Error("chat: marshal anthropic request", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Call Anthropic API.
	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		a.logger.Error("chat: build upstream request", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq) //nolint:gosec // G704: URL is a constant (anthropicAPIURL), not user input
	if err != nil {
		a.logger.Warn("chat: anthropic request failed", zap.Error(err))
		writeError(w, http.StatusBadGateway, fmt.Sprintf("upstream request failed: %v", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body.
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		a.logger.Warn("chat: read upstream response", zap.Error(err))
		writeError(w, http.StatusBadGateway, "failed to read upstream response")
		return
	}

	// If upstream returned an error, forward the status code.
	if resp.StatusCode != http.StatusOK {
		a.logger.Warn("chat: upstream error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(respBody)))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(respBody)
		return
	}

	// Parse the Anthropic response.
	var anthropicResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		a.logger.Error("chat: unmarshal anthropic response", zap.Error(err))
		writeError(w, http.StatusBadGateway, "invalid upstream response")
		return
	}

	// Extract text content from the response.
	var content string
	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	chatResp := chatResponse{
		Content:      content,
		Model:        anthropicResp.Model,
		InputTokens:  anthropicResp.Usage.InputTokens,
		OutputTokens: anthropicResp.Usage.OutputTokens,
	}

	writeJSON(w, http.StatusOK, chatResp)
}
