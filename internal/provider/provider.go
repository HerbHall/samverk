// Package provider defines the Provider interface and shared types for
// AI model inference backends (Ollama, Claude, OpenAI).
package provider

import "context"

// Role identifies the sender of a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single chat message.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// ChatRequest contains the parameters for a chat completion.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"` // always false for now
}

// ChatResponse contains the result of a chat completion.
type ChatResponse struct {
	Model   string  `json:"model"`
	Message Message `json:"message"`

	// Token usage (if reported by provider).
	PromptTokens     int `json:"prompt_eval_count,omitempty"`
	CompletionTokens int `json:"eval_count,omitempty"`
}

// ActivityNotifier is an optional interface that providers may implement to
// support streaming progress detection. When a provider supports this, the
// runner sets a callback that fires whenever the provider produces output
// bytes, allowing the dispatcher heartbeat to reset on active output.
type ActivityNotifier interface {
	SetOnActivity(fn func())
}

// Provider is the interface for AI model inference backends.
// Implementations exist for Ollama (local), Claude, and OpenAI (cloud).
type Provider interface {
	// Chat sends a chat completion request and returns the response.
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

	// Healthy returns true if the provider is reachable and ready.
	Healthy(ctx context.Context) bool

	// Name returns the provider identifier (e.g., "ollama", "claude", "openai").
	Name() string
}
