// Package claudecli implements provider.Provider by shelling out to the
// Claude Code CLI (`claude --print`). This uses the user's Max plan
// authentication from ~/.claude instead of requiring separate API credits.
package claudecli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/herbhall/samverk/internal/provider"
)

// Compile-time check that Client satisfies provider.Provider.
var _ provider.Provider = (*Client)(nil)

const (
	defaultTimeout = 300 * time.Second
	// maxErrOutputBytes caps the CLI output included in error messages to prevent
	// oversized GitHub issue comments and avoid leaking unrelated terminal output.
	maxErrOutputBytes = 2048

	// staleOutputTimeout is how long Chat waits for new bytes before treating
	// the process as hung. Must be shorter than the dispatcher heartbeat timeout.
	staleOutputTimeout = 3 * time.Minute

	// streamBufSize is the read buffer for pipe-based streaming.
	streamBufSize = 4096
)

// Client invokes the claude CLI binary for chat completions.
type Client struct {
	claudeBin  string
	model      string
	timeout    time.Duration
	onActivity func() // called when output bytes arrive; may be nil
}

// New creates a claude-cli provider with the default timeout.
// If model is empty, the CLI uses its default.
func New(model string) *Client {
	return NewWithTimeout(model, defaultTimeout)
}

// NewWithTimeout creates a claude-cli provider with a custom timeout.
// Use this when the provider config specifies timeout_seconds.
func NewWithTimeout(model string, timeout time.Duration) *Client {
	return &Client{
		claudeBin: "claude",
		model:     model,
		timeout:   timeout,
	}
}

// SetOnActivity registers a callback invoked whenever the CLI process
// produces output bytes. The runner uses this to reset the dispatcher
// heartbeat timer, proving the session is actively producing tokens.
func (c *Client) SetOnActivity(fn func()) {
	c.onActivity = fn
}

// Chat builds a prompt from the request messages, invokes `claude --print`,
// and returns the CLI output as the assistant response.
//
// Output is read incrementally via pipes instead of CombinedOutput so that
// the onActivity callback fires as bytes arrive and hung processes are
// detected early (no output for staleOutputTimeout → cancel).
//
// IMPORTANT: The prompt MUST be sent via stdin, not as a CLI argument.
// Passing the prompt as an argument causes the CLI to hang indefinitely.
// --dangerously-skip-permissions is required for headless/non-interactive use.
// ANTHROPIC_API_KEY must be unset so the CLI uses OAuth (~/.claude) not API credits.
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

	args := []string{"--print", "--dangerously-skip-permissions"}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.claudeBin, args...) //nolint:gosec // G204: claudeBin is set internally
	cmd.Stdin = strings.NewReader(prompt.String())        // prompt via stdin — argument mode hangs
	// Place the subprocess in its own process group so that signals sent to
	// the parent's process group (e.g. from a misconfigured KillMode) do not
	// propagate to claude-cli. Belt-and-suspenders alongside KillMode=process.
	setProcessGroup(cmd)

	// Inherit environment but strip ANTHROPIC_API_KEY so the CLI uses
	// OAuth credentials (~/.claude) instead of API credits.
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "ANTHROPIC_API_KEY=") {
			env = append(env, e)
		}
	}
	cmd.Env = env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("claude-cli: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("claude-cli: stderr pipe: %w", err)
	}

	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude-cli: start: %w", err)
	}

	// Read stdout and stderr concurrently, merging into a single buffer.
	var output strings.Builder
	streamErr := c.streamOutput(ctx, cancel, &output, stdout, stderr)

	// Wait for the process to exit after pipes are drained.
	waitErr := cmd.Wait()

	// Prefer stream errors (hung detection) over wait errors.
	if streamErr != nil {
		snippet := output.String()
		if len(snippet) > maxErrOutputBytes {
			snippet = snippet[len(snippet)-maxErrOutputBytes:]
		}
		return nil, fmt.Errorf("claude-cli: %w: output: %s", streamErr, strings.TrimSpace(snippet))
	}
	if waitErr != nil {
		snippet := output.String()
		if len(snippet) > maxErrOutputBytes {
			snippet = snippet[len(snippet)-maxErrOutputBytes:]
		}
		return nil, fmt.Errorf("claude-cli: exec: %w: output: %s", waitErr, strings.TrimSpace(snippet))
	}

	return &provider.ChatResponse{
		Model: c.model,
		Message: provider.Message{
			Role:    provider.RoleAssistant,
			Content: strings.TrimSpace(output.String()),
		},
	}, nil
}

// streamOutput reads from stdout and stderr concurrently, writing to output.
// It fires onActivity on each read and cancels the context if no bytes arrive
// within staleOutputTimeout (hung detection).
func (c *Client) streamOutput(ctx context.Context, cancel context.CancelFunc, output *strings.Builder, stdout, stderr io.ReadCloser) error {
	// Merge stdout and stderr via a pipe so we can read from a single source.
	pr, pw := io.Pipe()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(pw, stdout)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(pw, stderr)
	}()
	go func() {
		wg.Wait()
		_ = pw.Close()
	}()

	buf := make([]byte, streamBufSize)
	staleTimer := time.NewTimer(staleOutputTimeout)
	defer staleTimer.Stop()

	for {
		// Use a channel to make Read interruptible by the stale timer.
		type readResult struct {
			n   int
			err error
		}
		ch := make(chan readResult, 1)
		go func() {
			n, readErr := pr.Read(buf)
			ch <- readResult{n, readErr}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-staleTimer.C:
			cancel() // kill the subprocess
			return fmt.Errorf("hung: no output for %v", staleOutputTimeout)
		case res := <-ch:
			if res.n > 0 {
				output.Write(buf[:res.n])
				// Reset stale timer — process is alive.
				if !staleTimer.Stop() {
					select {
					case <-staleTimer.C:
					default:
					}
				}
				staleTimer.Reset(staleOutputTimeout)
				// Signal activity to the dispatcher heartbeat.
				if c.onActivity != nil {
					c.onActivity()
				}
			}
			if res.err != nil {
				if res.err == io.EOF {
					return nil
				}
				return fmt.Errorf("read output: %w", res.err)
			}
		}
	}
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
