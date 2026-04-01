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
	Model      string    `json:"model"`
	Messages   []Message `json:"messages"`
	Stream     bool      `json:"stream"`      // always false for now
	WorkingDir string    `json:"-"`            // workspace directory for CLI providers; not serialized
	MaxTurns   int       `json:"-"`            // per-request max agentic turns; 0 = use provider default
}

// ChatResponse contains the result of a chat completion.
type ChatResponse struct {
	Model   string  `json:"model"`
	Message Message `json:"message"`

	// Token usage (if reported by provider).
	PromptTokens     int `json:"prompt_eval_count,omitempty"`
	CompletionTokens int `json:"eval_count,omitempty"`

	// MaxTurnsHit is true when the session ended because it reached the
	// --max-turns limit. The runner can use this to decide whether to
	// increase max_turns on retry.
	MaxTurnsHit bool `json:"-"`
	// TurnsUsed is the number of assistant turns completed in this session.
	TurnsUsed int `json:"-"`
}

// ActivityNotifier is an optional interface that providers may implement to
// support streaming progress detection. When a provider supports this, the
// runner sets a callback that fires whenever the provider produces output
// bytes, allowing the dispatcher heartbeat to reset on active output.
type ActivityNotifier interface {
	SetOnActivity(fn func())
}

// ToolAccessProvider is an optional interface that providers may implement to
// indicate they have local filesystem access via tools (Read, Write, Glob, etc.).
// CLI-based providers (e.g., claude-cli) implement this returning true.
// API-based providers (e.g., Ollama) do not implement this; HasToolAccess
// returns false by default.
type ToolAccessProvider interface {
	HasToolAccess() bool
}

// CanAccessTools checks whether a provider supports local filesystem access.
// Returns true only if the provider implements ToolAccessProvider and reports
// tool access. Used by the prompt builder to tailor file context instructions.
func CanAccessTools(p Provider) bool {
	if ta, ok := p.(ToolAccessProvider); ok {
		return ta.HasToolAccess()
	}
	return false
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
