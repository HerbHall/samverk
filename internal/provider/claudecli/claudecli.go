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
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/provider"
)

// Compile-time check that Client satisfies provider.Provider.
var _ provider.Provider = (*Client)(nil)

const (
	defaultTimeout = 300 * time.Second
	// maxErrOutputBytes caps the CLI output included in error messages to prevent
	// oversized GitHub issue comments and avoid leaking unrelated terminal output.
	maxErrOutputBytes = 2048

	// startupTimeout is how long Chat waits for the first byte of output after
	// spawning the process. If nothing arrives within this window, the process
	// is assumed to have failed to start (e.g. OAuth hang, binary crash).
	startupTimeout = 30 * time.Second

	// staleOutputTimeout is how long Chat waits for new bytes before treating
	// the process as hung. Must be shorter than the dispatcher heartbeat timeout.
	// 30s provides fast hang detection while still allowing brief pauses between
	// tool calls (typically <15s).
	staleOutputTimeout = 30 * time.Second

	// streamBufSize is the read buffer for pipe-based streaming.
	streamBufSize = 4096
)

// Client invokes the claude CLI binary for chat completions.
type Client struct {
	claudeBin    string
	model        string
	timeout      time.Duration
	allowedTools string // comma-separated tool list; empty means no --allowedTools flag
	maxTurns     int    // max agentic turns; 0 means no limit
	baseURL      string // when set, redirects CLI to this base URL (e.g. Ollama)
	onActivity   func() // called when output bytes arrive; may be nil
	logger       *zap.Logger
}

// Options configures optional Client parameters.
type Options struct {
	AllowedTools string      // comma-separated tool list (e.g. "Bash,Read,Edit,Write,Glob,Grep")
	MaxTurns     int         // max agentic turns per session; 0 means no limit
	BaseURL      string      // override ANTHROPIC_BASE_URL (e.g. Ollama endpoint)
	Logger       *zap.Logger // structured logger; nil uses nop logger
}

// New creates a claude-cli provider with the default timeout.
// If model is empty, the CLI uses its default.
func New(model string, opts ...Options) *Client {
	return NewWithTimeout(model, defaultTimeout, opts...)
}

// NewWithTimeout creates a claude-cli provider with a custom timeout.
// Use this when the provider config specifies timeout_seconds.
func NewWithTimeout(model string, timeout time.Duration, opts ...Options) *Client {
	c := &Client{
		claudeBin: "claude",
		model:     model,
		timeout:   timeout,
		logger:    zap.NewNop(),
	}
	if len(opts) > 0 {
		c.allowedTools = opts[0].AllowedTools
		c.maxTurns = opts[0].MaxTurns
		c.baseURL = opts[0].BaseURL
		if opts[0].Logger != nil {
			c.logger = opts[0].Logger
		}
	}
	return c
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
// --allowedTools pre-approves tools so the CLI never prompts in headless context.
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

	start := time.Now()
	log := c.logger
	if log == nil {
		log = zap.NewNop()
	}

	args := []string{"--print", "--dangerously-skip-permissions", "--no-session-persistence"}
	if c.allowedTools != "" {
		args = append(args, "--allowedTools", c.allowedTools)
	}
	if c.maxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(c.maxTurns))
	}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}

	log.Info("claude-cli chat request",
		zap.String("model", c.model),
		zap.Int("message_count", len(req.Messages)),
		zap.Int("prompt_bytes", prompt.Len()),
		zap.String("allowed_tools", c.allowedTools),
		zap.Int("max_turns", c.maxTurns),
		zap.Duration("timeout", c.timeout),
	)

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.claudeBin, args...) //nolint:gosec // G204: claudeBin is set internally
	cmd.Stdin = strings.NewReader(prompt.String())        // prompt via stdin — argument mode hangs
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}
	// Place the subprocess in its own process group so that signals sent to
	// the parent's process group (e.g. from a misconfigured KillMode) do not
	// propagate to claude-cli. Belt-and-suspenders alongside KillMode=process.
	setProcessGroup(cmd)

	cmd.Env = c.buildEnv()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("claude-cli: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("claude-cli: stderr pipe: %w", err)
	}

	if err = cmd.Start(); err != nil {
		log.Error("claude-cli start failed",
			zap.Error(err),
		)
		return nil, fmt.Errorf("claude-cli: start: %w", err)
	}
	log.Info("claude-cli process started",
		zap.Int("pid", cmd.Process.Pid),
	)

	// Read stdout and stderr concurrently, merging into a single buffer.
	var output strings.Builder
	streamErr := c.streamOutput(ctx, cancel, &output, stdout, stderr)

	// Wait for the process to exit after pipes are drained.
	waitErr := cmd.Wait()

	// Prefer stream errors (hung detection) over wait errors.
	// ErrProviderTimeout is returned unwrapped so the pool can detect retryable errors.
	if streamErr != nil {
		snippet := output.String()
		if len(snippet) > maxErrOutputBytes {
			snippet = snippet[len(snippet)-maxErrOutputBytes:]
		}
		log.Error("claude-cli stream error",
			zap.String("model", c.model),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Int("output_bytes", output.Len()),
			zap.Error(streamErr),
		)
		// Preserve ErrProviderTimeout as the root so errors.As works for failover.
		if provider.IsRetryable(streamErr) {
			return nil, streamErr
		}
		return nil, fmt.Errorf("claude-cli: %w: output: %s", streamErr, strings.TrimSpace(snippet))
	}
	if waitErr != nil {
		snippet := output.String()
		if len(snippet) > maxErrOutputBytes {
			snippet = snippet[len(snippet)-maxErrOutputBytes:]
		}
		log.Error("claude-cli exec error",
			zap.String("model", c.model),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Int("output_bytes", output.Len()),
			zap.Error(waitErr),
		)
		return nil, fmt.Errorf("claude-cli: exec: %w: output: %s", waitErr, strings.TrimSpace(snippet))
	}

	log.Info("claude-cli chat response",
		zap.String("model", c.model),
		zap.Int("output_bytes", output.Len()),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	)

	return &provider.ChatResponse{
		Model: c.model,
		Message: provider.Message{
			Role:    provider.RoleAssistant,
			Content: strings.TrimSpace(output.String()),
		},
	}, nil
}

// buildEnv constructs the subprocess environment.
// Always strips ANTHROPIC_API_KEY so the CLI uses OAuth (~/.claude) not API credits.
// When baseURL is configured (e.g. Ollama), also replaces ANTHROPIC_BASE_URL and
// ANTHROPIC_AUTH_TOKEN with values appropriate for the custom backend.
func (c *Client) buildEnv() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		switch {
		case strings.HasPrefix(e, "ANTHROPIC_API_KEY="):
			continue
		case c.baseURL != "" && strings.HasPrefix(e, "ANTHROPIC_BASE_URL="):
			continue
		case c.baseURL != "" && strings.HasPrefix(e, "ANTHROPIC_AUTH_TOKEN="):
			continue
		default:
			env = append(env, e)
		}
	}
	// When a custom base URL is configured (e.g. Ollama), redirect the CLI
	// to that endpoint. ANTHROPIC_AUTH_TOKEN is set to a dummy value because
	// the CLI requires it but Ollama ignores it. Nonessential traffic
	// (usage/billing) is disabled to prevent 404 errors from non-Anthropic
	// backends. Token counting is skipped to avoid hangs on Ollama's missing
	// /v1/messages/count_tokens endpoint (see ollama/ollama#13949).
	if c.baseURL != "" {
		env = append(env,
			"ANTHROPIC_BASE_URL="+c.baseURL,
			"ANTHROPIC_AUTH_TOKEN=ollama",
			"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
			"CLAUDE_CODE_SKIP_TOKEN_COUNTING=1",
		)
	}
	return env
}

// streamOutput reads from stdout and stderr concurrently, writing to output.
// It fires onActivity on each read and cancels the context if no bytes arrive
// within the timeout window. Two phases:
//   - Startup: waits startupTimeout for the first byte (fast-fail on launch issues)
//   - Stale: after first byte, waits staleOutputTimeout between subsequent reads
//
// Returns *provider.ErrProviderTimeout on timeout so the pool can retry with
// the next provider in the routing chain.
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

	// Start with the startup timeout; switch to stale after first byte.
	gotFirstByte := false
	timer := time.NewTimer(startupTimeout)
	defer timer.Stop()

	for {
		// Use a channel to make Read interruptible by the timer.
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
		case <-timer.C:
			cancel() // kill the subprocess
			timeoutType := provider.TimeoutStartup
			dur := startupTimeout
			if gotFirstByte {
				timeoutType = provider.TimeoutStale
				dur = staleOutputTimeout
			}
			c.logger.Error("claude-cli timeout",
				zap.String("provider", c.Name()),
				zap.String("model", c.model),
				zap.String("timeout_type", string(timeoutType)),
				zap.Duration("duration", dur),
			)
			return &provider.ErrProviderTimeout{
				Provider:    c.Name(),
				Model:       c.model,
				TimeoutType: timeoutType,
				Duration:    dur,
			}
		case res := <-ch:
			if res.n > 0 {
				output.Write(buf[:res.n])
				// Reset timer — process is alive.
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				// After first byte, switch to stale output timeout.
				if !gotFirstByte {
					gotFirstByte = true
					timer.Reset(staleOutputTimeout)
				} else {
					timer.Reset(staleOutputTimeout)
				}
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
