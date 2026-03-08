// Package claudecli implements provider.Provider by shelling out to the
// Claude Code CLI (`claude --print`). This uses the user's Max plan
// authentication from ~/.claude instead of requiring separate API credits.
package claudecli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/herbhall/samverk/internal/provider"
)

// Compile-time check that Client satisfies provider.Provider.
var _ provider.Provider = (*Client)(nil)

const defaultTimeout = 300 * time.Second

// Client invokes the claude CLI binary for chat completions.
type Client struct {
	claudeBin string
	model     string
	timeout   time.Duration
}

// New creates a claude-cli provider. If model is empty, the CLI uses its default.
func New(model string) *Client {
	return &Client{
		claudeBin: "claude",
		model:     model,
		timeout:   defaultTimeout,
	}
}

// Chat builds a prompt from the request messages, invokes `claude --print`,
// and returns the CLI output as the assistant response.
//
// IMPORTANT: The prompt MUST be sent via stdin, not as a CLI argument.
// Passing the prompt as an argument causes the CLI to hang indefinitely.
func (c *Client) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	var prompt strings.Builder
	for _, m := range req.Messages {
		if m.Role == provider.RoleSystem {
			fmt.Fprintf(&prompt, "%s\n\n", m.Content)
		}
	}
	for _, m := range req.Messages {
		if m.Role == provider.RoleUser {
			fmt.Fprintf(&prompt, "%s", m.Content)
		}
	}

	args := []string{"--print"}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.claudeBin, args...) //nolint:gosec // G204: claudeBin is set internally
	cmd.Stdin = strings.NewReader(prompt.String())        // prompt via stdin — argument mode hangs
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("claude-cli: exec: %w: stderr: %s", err, strings.TrimSpace(stderr.String()))
	}

	return &provider.ChatResponse{
		Model: c.model,
		Message: provider.Message{
			Role:    provider.RoleAssistant,
			Content: strings.TrimSpace(string(out)),
		},
	}, nil
}

// Healthy returns true if the claude binary is found and responds to --version.
func (c *Client) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := exec.CommandContext(ctx, c.claudeBin, "--version").Run() //nolint:gosec // G204: claudeBin is set internally
	return err == nil
}

// Name returns the provider identifier.
func (c *Client) Name() string { return "claude-cli" }
