// Package claudecli implements provider.Provider by shelling out to the
// Claude Code CLI (`claude --print`). This uses the user's Max plan
// authentication from ~/.claude instead of requiring separate API credits.
package claudecli

import (
	"bufio"
	"context"
	"encoding/json"
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

	// maxOutputBufferBytes caps the raw output buffer that accumulates stream
	// lines for error diagnostics. The actual result is extracted from the
	// {"type":"result"} event and stored separately, so the raw buffer only
	// needs to hold enough context for debugging failures. Previously this
	// was unbounded, causing the dispatch process to grow to 9.9 GB RSS when
	// running multiple long agent sessions (each accumulating full stream
	// output including all tool calls and results).
	maxOutputBufferBytes = 64 * 1024 // 64 KB per session

	// startupTimeout is how long Chat waits for the first JSON event after
	// spawning the process. With --output-format stream-json --verbose, the
	// CLI emits a {"type":"system","subtype":"init"} event within 1-3 seconds
	// of startup. 30s is generous enough for slow machines while catching
	// true startup failures (OAuth hang, binary crash, missing .git dir).
	//
	// Previously 300s when using plain --print (which buffers ALL output).
	startupTimeout = 30 * time.Second

	// staleOutputTimeout is how long Chat waits between JSON events before
	// treating the process as hung. With stream-json, the CLI emits events
	// for each assistant turn, tool call, and tool result, so gaps between
	// events correspond to actual API round-trip time (typically 5-30s).
	// 120s accommodates long-running tool calls and slow API responses
	// while still detecting genuine hangs.
	staleOutputTimeout = 120 * time.Second

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

// Chat builds a prompt from the request messages, invokes `claude --print`
// with `--output-format stream-json --verbose`, and returns the final result.
//
// The stream-json format emits one JSON object per line as events occur
// (init, assistant turns, tool calls, tool results, rate limits, final result).
// This enables real-time activity detection: each JSON line resets the stale
// timer, preventing false "hung process" kills during long agentic sessions
// where plain --print mode would buffer ALL output until the session ends.
//
// The final result text is extracted from the {"type":"result"} event.
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

	args := []string{
		"--print",
		"--dangerously-skip-permissions",
		"--no-session-persistence",
		"--output-format", "stream-json",
		"--verbose",
	}
	if c.allowedTools != "" {
		args = append(args, "--allowedTools", c.allowedTools)
	}
	// Per-request MaxTurns overrides client default when > 0.
	effectiveMaxTurns := c.maxTurns
	if req.MaxTurns > 0 {
		effectiveMaxTurns = req.MaxTurns
	}
	if effectiveMaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(effectiveMaxTurns))
	}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}

	log.Info("claude-cli chat request",
		zap.String("model", c.model),
		zap.Int("message_count", len(req.Messages)),
		zap.Int("prompt_bytes", prompt.Len()),
		zap.String("allowed_tools", c.allowedTools),
		zap.Int("max_turns", effectiveMaxTurns),
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
	meta, streamErr := c.streamOutput(ctx, cancel, &output, stdout, stderr)

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

	// output now contains the result text extracted from the stream-json
	// {"type":"result"} event. If stream-json parsing failed or the CLI
	// produced plain text, output contains the raw accumulated text.
	resultText := strings.TrimSpace(output.String())

	log.Info("claude-cli chat response",
		zap.String("model", c.model),
		zap.Int("result_bytes", len(resultText)),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	)

	return &provider.ChatResponse{
		Model: c.model,
		Message: provider.Message{
			Role:    provider.RoleAssistant,
			Content: resultText,
		},
		MaxTurnsHit: meta.ResultError && meta.NumTurns >= effectiveMaxTurns,
		TurnsUsed:   meta.NumTurns,
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
		case strings.HasPrefix(e, "ANTHROPIC_BASE_URL="):
			continue
		case strings.HasPrefix(e, "ANTHROPIC_AUTH_TOKEN="):
			continue
		case strings.HasPrefix(e, "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="):
			continue
		case strings.HasPrefix(e, "CLAUDE_CODE_SKIP_TOKEN_COUNTING="):
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

// streamEvent is the minimal structure of a stream-json event from the CLI.
// Only fields needed for activity detection and result extraction are decoded.
type streamEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`
	Result  string `json:"result,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
}

// streamMeta carries metadata extracted from the stream-json session.
type streamMeta struct {
	NumTurns    int  // number of assistant turns completed
	ResultError bool // true if the result event had is_error=true (e.g., max-turns reached)
}

// streamOutput reads stream-json events from stdout, line by line.
// Each JSON line resets the activity timer so the dispatcher knows the
// session is alive. The final result text is extracted from the
// {"type":"result"} event and written to output.
//
// Stderr is drained separately to prevent pipe deadlocks but is not
// used for activity detection (the CLI produces no stderr in --print mode).
//
// Two timeout phases:
//   - Startup: waits startupTimeout for the first JSON line (~2s expected)
//   - Stale: after first line, waits staleOutputTimeout between lines
//
// Returns *provider.ErrProviderTimeout on timeout so the pool can retry
// with the next provider in the routing chain.
func (c *Client) streamOutput(ctx context.Context, cancel context.CancelFunc, output *strings.Builder, stdout, stderr io.ReadCloser) (streamMeta, error) {
	log := c.logger
	if log == nil {
		log = zap.NewNop()
	}

	// Drain stderr in background to prevent pipe deadlock.
	// Capped to maxOutputBufferBytes to prevent unbounded growth.
	var stderrBuf strings.Builder
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stderrBuf, io.LimitReader(stderr, maxOutputBufferBytes))
		// Drain remaining stderr to prevent pipe deadlock.
		_, _ = io.Copy(io.Discard, stderr)
	}()

	scanner := bufio.NewScanner(stdout)
	// stream-json events can be large (full assistant messages with tool results).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	gotFirstLine := false
	timer := time.NewTimer(startupTimeout)
	defer timer.Stop()

	// Use a channel to make Scan interruptible by the timer.
	type scanResult struct {
		line string
		ok   bool
	}
	lineCh := make(chan scanResult, 1)

	scanNext := func() {
		go func() {
			ok := scanner.Scan()
			if ok {
				lineCh <- scanResult{line: scanner.Text(), ok: true}
			} else {
				lineCh <- scanResult{ok: false}
			}
		}()
	}

	var resultText string
	var numTurns int
	var resultIsError bool

	scanNext()
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return streamMeta{NumTurns: numTurns, ResultError: resultIsError}, ctx.Err()

		case <-timer.C:
			cancel()
			timeoutType := provider.TimeoutStartup
			dur := startupTimeout
			if gotFirstLine {
				timeoutType = provider.TimeoutStale
				dur = staleOutputTimeout
			}
			log.Error("claude-cli timeout",
				zap.String("provider", c.Name()),
				zap.String("model", c.model),
				zap.String("timeout_type", string(timeoutType)),
				zap.Duration("duration", dur),
				zap.Int("turns_completed", numTurns),
			)
			wg.Wait()
			return streamMeta{NumTurns: numTurns}, &provider.ErrProviderTimeout{
				Provider:    c.Name(),
				Model:       c.model,
				TimeoutType: timeoutType,
				Duration:    dur,
			}

		case res := <-lineCh:
			if !res.ok {
				// Scanner finished (EOF or error).
				wg.Wait()
				if err := scanner.Err(); err != nil {
					return streamMeta{NumTurns: numTurns, ResultError: resultIsError}, fmt.Errorf("read stream: %w", err)
				}
				// If we got a result from the stream, use it.
				// Otherwise fall back to raw output (non-json CLI or error).
				if resultText != "" {
					output.Reset()
					output.WriteString(resultText)
				} else if output.Len() == 0 && stderrBuf.Len() > 0 {
					// No stdout at all -- surface stderr as the output.
					output.WriteString(stderrBuf.String())
				}
				return streamMeta{NumTurns: numTurns, ResultError: resultIsError}, nil
			}

			line := res.line

			// Reset timer on every line -- the process is alive.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			if !gotFirstLine {
				gotFirstLine = true
				log.Debug("claude-cli first event received",
					zap.String("model", c.model),
				)
			}
			timer.Reset(staleOutputTimeout)

			// Signal activity to the dispatcher heartbeat.
			if c.onActivity != nil {
				c.onActivity()
			}

			// Parse the JSON event to extract type and result.
			var event streamEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				// Not valid JSON -- accumulate as raw output (fallback),
				// capped to prevent unbounded growth.
				if output.Len() < maxOutputBufferBytes {
					output.WriteString(line)
					output.WriteByte('\n')
				}
				scanNext()
				continue
			}

			switch event.Type {
			case "assistant":
				numTurns++
			case "result":
				resultText = event.Result
				resultIsError = event.IsError
				if event.IsError {
					log.Warn("claude-cli session ended with error",
						zap.String("model", c.model),
						zap.String("result", truncate(resultText, 200)),
					)
				}
			}

			// Accumulate raw lines for error diagnostics, but cap to prevent
			// unbounded memory growth. The actual result is extracted above
			// into resultText; this buffer is only used when the session
			// fails and we need context for the error message.
			if output.Len() < maxOutputBufferBytes {
				output.WriteString(line)
				output.WriteByte('\n')
			}

			scanNext()
		}
	}
}

// truncate returns s truncated to n bytes with "..." appended if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
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

// HasToolAccess returns true because claude-cli runs as a subprocess with
// local filesystem access via tools (Read, Write, Edit, Glob, Grep).
func (c *Client) HasToolAccess() bool { return true }
