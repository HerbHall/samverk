package claudecli

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/provider"
)

func TestNew(t *testing.T) {
	c := New("claude-sonnet-4-6")
	if c.claudeBin != "claude" {
		t.Errorf("claudeBin = %q, want 'claude'", c.claudeBin)
	}
	if c.model != "claude-sonnet-4-6" {
		t.Errorf("model = %q, want 'claude-sonnet-4-6'", c.model)
	}
	if c.timeout != defaultTimeout {
		t.Errorf("timeout = %v, want %v", c.timeout, defaultTimeout)
	}
}

func TestName(t *testing.T) {
	c := New("")
	if c.Name() != "claude-cli" {
		t.Errorf("Name() = %q, want 'claude-cli'", c.Name())
	}
}

func TestChatBuildsPrompt(t *testing.T) {
	// Use a binary that echoes input to verify prompt construction.
	// On systems without 'echo', this test verifies structure only.
	c := &Client{
		claudeBin: "echo",
		model:     "",
		timeout:   defaultTimeout,
	}

	resp, err := c.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "You are helpful."},
			{Role: provider.RoleUser, Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if resp.Message.Role != provider.RoleAssistant {
		t.Errorf("role = %q, want 'assistant'", resp.Message.Role)
	}
	// echo outputs the args: "--print <prompt>" or similar.
	// We just verify it didn't error.
	if resp.Message.Content == "" {
		t.Error("expected non-empty content from echo")
	}
}

func TestChatWithModel(t *testing.T) {
	c := &Client{
		claudeBin: "echo",
		model:     "claude-sonnet-4-6",
		timeout:   defaultTimeout,
	}

	resp, err := c.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "test"},
		},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if resp.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want 'claude-sonnet-4-6'", resp.Model)
	}
}

func TestHealthyWithMissingBinary(t *testing.T) {
	c := &Client{
		claudeBin: "nonexistent-binary-that-does-not-exist",
		timeout:   defaultTimeout,
	}
	if c.Healthy(context.Background()) {
		t.Error("Healthy() should return false for missing binary")
	}
}

func TestChatExecError(t *testing.T) {
	c := &Client{
		claudeBin: "nonexistent-binary-that-does-not-exist",
		timeout:   defaultTimeout,
	}
	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "test"},
		},
	})
	if err == nil {
		t.Fatal("Chat() should return error for missing binary")
	}
}

// TestActivityNotifierInterface verifies Client implements provider.ActivityNotifier.
func TestActivityNotifierInterface(t *testing.T) {
	var _ provider.ActivityNotifier = (*Client)(nil)
}

// TestSetOnActivity verifies the callback is stored and can be cleared.
func TestSetOnActivity(t *testing.T) {
	c := New("")
	if c.onActivity != nil {
		t.Fatal("onActivity should be nil initially")
	}

	called := false
	c.SetOnActivity(func() { called = true })
	if c.onActivity == nil {
		t.Fatal("onActivity should be set after SetOnActivity")
	}
	c.onActivity()
	if !called {
		t.Error("onActivity callback was not invoked")
	}

	c.SetOnActivity(nil)
	if c.onActivity != nil {
		t.Error("onActivity should be nil after SetOnActivity(nil)")
	}
}

// TestChatFiresOnActivity verifies the onActivity callback fires during
// streaming output from a subprocess. Uses `echo` which produces output
// that should trigger at least one callback invocation.
func TestChatFiresOnActivity(t *testing.T) {
	var calls atomic.Int32
	c := &Client{
		claudeBin:  "echo",
		model:      "",
		timeout:    10 * time.Second,
		onActivity: func() { calls.Add(1) },
	}

	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "hello world"},
		},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if calls.Load() == 0 {
		t.Error("onActivity was never called during streaming output")
	}
}

// TestNewWithTimeout verifies custom timeout is applied.
func TestNewWithTimeout(t *testing.T) {
	c := NewWithTimeout("model", 42*time.Second)
	if c.timeout != 42*time.Second {
		t.Errorf("timeout = %v, want 42s", c.timeout)
	}
}

// TestNewWithOptions verifies AllowedTools and MaxTurns are stored.
func TestNewWithOptions(t *testing.T) {
	opts := Options{
		AllowedTools: "Bash,Read,Edit,Write,Glob,Grep",
		MaxTurns:     25,
	}
	c := New("model", opts)
	if c.allowedTools != opts.AllowedTools {
		t.Errorf("allowedTools = %q, want %q", c.allowedTools, opts.AllowedTools)
	}
	if c.maxTurns != opts.MaxTurns {
		t.Errorf("maxTurns = %d, want %d", c.maxTurns, opts.MaxTurns)
	}
}

// TestNewWithoutOptions verifies defaults when no options are passed.
func TestNewWithoutOptions(t *testing.T) {
	c := New("model")
	if c.allowedTools != "" {
		t.Errorf("allowedTools = %q, want empty", c.allowedTools)
	}
	if c.maxTurns != 0 {
		t.Errorf("maxTurns = %d, want 0", c.maxTurns)
	}
}

// TestChatIncludesAllowedTools verifies --allowedTools appears in CLI args.
// Uses echo which outputs the args, so we can verify they are present.
func TestChatIncludesAllowedTools(t *testing.T) {
	c := &Client{
		claudeBin:    "echo",
		model:        "",
		timeout:      defaultTimeout,
		allowedTools: "Bash,Read,Edit",
		maxTurns:     10,
	}

	resp, err := c.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "test"},
		},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	content := resp.Message.Content
	if !strings.Contains(content, "--allowedTools") {
		t.Errorf("output missing --allowedTools flag, got: %s", content)
	}
	if !strings.Contains(content, "Bash,Read,Edit") {
		t.Errorf("output missing tool list, got: %s", content)
	}
	if !strings.Contains(content, "--max-turns") {
		t.Errorf("output missing --max-turns flag, got: %s", content)
	}
	if !strings.Contains(content, "10") {
		t.Errorf("output missing max-turns value, got: %s", content)
	}
}

// TestChatOmitsOptionalFlags verifies flags are omitted when not configured.
func TestChatOmitsOptionalFlags(t *testing.T) {
	c := &Client{
		claudeBin: "echo",
		model:     "",
		timeout:   defaultTimeout,
	}

	resp, err := c.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "test"},
		},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	content := resp.Message.Content
	if strings.Contains(content, "--allowedTools") {
		t.Errorf("output should not contain --allowedTools when not configured, got: %s", content)
	}
	if strings.Contains(content, "--max-turns") {
		t.Errorf("output should not contain --max-turns when not configured, got: %s", content)
	}
}

// TestChatIncludesStreamJSON verifies --output-format stream-json --verbose
// are always present in CLI args.
func TestChatIncludesStreamJSON(t *testing.T) {
	c := &Client{
		claudeBin: "echo",
		model:     "",
		timeout:   defaultTimeout,
	}

	resp, err := c.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "test"},
		},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	content := resp.Message.Content
	if !strings.Contains(content, "--output-format") {
		t.Errorf("output missing --output-format flag, got: %s", content)
	}
	if !strings.Contains(content, "stream-json") {
		t.Errorf("output missing stream-json value, got: %s", content)
	}
	if !strings.Contains(content, "--verbose") {
		t.Errorf("output missing --verbose flag, got: %s", content)
	}
}

// TestStreamOutputParsesResultEvent verifies that streamOutput extracts
// the result text from a stream-json {"type":"result"} event.
func TestStreamOutputParsesResultEvent(t *testing.T) {
	// Simulate a stream-json session with init + assistant + result events.
	jsonLines := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"test-123"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"thinking..."}]}}`,
		`{"type":"result","subtype":"success","result":"The answer is 42.","is_error":false}`,
	}, "\n") + "\n"

	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()

	go func() {
		_, _ = stdoutW.Write([]byte(jsonLines))
		_ = stdoutW.Close()
	}()
	go func() {
		_ = stderrW.Close()
	}()

	var output strings.Builder
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := &Client{}
	_, err := c.streamOutput(ctx, cancel, &output, stdoutR, stderrR)
	if err != nil {
		t.Fatalf("streamOutput() error = %v", err)
	}

	result := strings.TrimSpace(output.String())
	if result != "The answer is 42." {
		t.Errorf("result = %q, want %q", result, "The answer is 42.")
	}
}

// TestStreamOutputHandlesNonJSON verifies fallback behavior when the CLI
// produces non-JSON output (e.g. error messages or old CLI versions).
func TestStreamOutputHandlesNonJSON(t *testing.T) {
	plainText := "This is plain text output\nfrom the CLI\n"

	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()

	go func() {
		_, _ = stdoutW.Write([]byte(plainText))
		_ = stdoutW.Close()
	}()
	go func() {
		_ = stderrW.Close()
	}()

	var output strings.Builder
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := &Client{}
	_, err := c.streamOutput(ctx, cancel, &output, stdoutR, stderrR)
	if err != nil {
		t.Fatalf("streamOutput() error = %v", err)
	}

	// Non-JSON lines accumulate as raw text.
	if !strings.Contains(output.String(), "This is plain text output") {
		t.Errorf("expected raw text in output, got: %q", output.String())
	}
}

// TestBaseURLStoredInClient verifies that BaseURL from Options is stored.
func TestBaseURLStoredInClient(t *testing.T) {
	c := New("model", Options{BaseURL: "http://192.168.1.207:11434"})
	if c.baseURL != "http://192.168.1.207:11434" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "http://192.168.1.207:11434")
	}
}

// TestOllamaEnvInjection verifies that when baseURL is set, the subprocess
// environment includes Ollama-specific env vars.
func TestOllamaEnvInjection(t *testing.T) {
	// buildEnv extracts the env slice that Chat would construct.
	// We test the env-building logic by inspecting the Client's buildEnv helper.
	c := &Client{
		claudeBin: "echo", // won't actually run in this test path
		model:     "qwen2.5-coder:14b",
		timeout:   defaultTimeout,
		baseURL:   "http://192.168.1.207:11434",
	}

	env := c.buildEnv()

	envMap := make(map[string]string, len(env))
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Verify Ollama env vars are injected.
	if got, ok := envMap["ANTHROPIC_BASE_URL"]; !ok || got != "http://192.168.1.207:11434" {
		t.Errorf("ANTHROPIC_BASE_URL = %q, want %q", got, "http://192.168.1.207:11434")
	}
	if got, ok := envMap["ANTHROPIC_AUTH_TOKEN"]; !ok || got != "ollama" {
		t.Errorf("ANTHROPIC_AUTH_TOKEN = %q, want %q", got, "ollama")
	}
	if got, ok := envMap["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"]; !ok || got != "1" {
		t.Errorf("CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC = %q, want %q", got, "1")
	}
	if got, ok := envMap["CLAUDE_CODE_SKIP_TOKEN_COUNTING"]; !ok || got != "1" {
		t.Errorf("CLAUDE_CODE_SKIP_TOKEN_COUNTING = %q, want %q", got, "1")
	}

	// Verify ANTHROPIC_API_KEY is stripped.
	if _, ok := envMap["ANTHROPIC_API_KEY"]; ok {
		t.Error("ANTHROPIC_API_KEY should be stripped when baseURL is set")
	}
}

// TestNoOllamaEnvWithoutBaseURL verifies that without a baseURL,
// no Ollama-specific env vars are injected.
func TestNoOllamaEnvWithoutBaseURL(t *testing.T) {
	// Set vars that buildEnv must strip even without baseURL.
	t.Setenv("ANTHROPIC_BASE_URL", "http://leftover-ollama:11434")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "stale-token")
	t.Setenv("CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1")
	t.Setenv("CLAUDE_CODE_SKIP_TOKEN_COUNTING", "1")

	c := &Client{
		claudeBin: "echo",
		model:     "claude-sonnet-4-6",
		timeout:   defaultTimeout,
	}

	env := c.buildEnv()

	for _, e := range env {
		if strings.HasPrefix(e, "ANTHROPIC_BASE_URL=") {
			t.Errorf("ANTHROPIC_BASE_URL should not be set without baseURL, got: %s", e)
		}
		if strings.HasPrefix(e, "ANTHROPIC_AUTH_TOKEN=") {
			t.Errorf("ANTHROPIC_AUTH_TOKEN should not be set without baseURL, got: %s", e)
		}
		if strings.HasPrefix(e, "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=") {
			t.Errorf("CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC should not be set without baseURL, got: %s", e)
		}
		if strings.HasPrefix(e, "CLAUDE_CODE_SKIP_TOKEN_COUNTING=") {
			t.Errorf("CLAUDE_CODE_SKIP_TOKEN_COUNTING should not be set without baseURL, got: %s", e)
		}
	}

	// ANTHROPIC_API_KEY should still be stripped (OAuth mode).
	for _, e := range env {
		if strings.HasPrefix(e, "ANTHROPIC_API_KEY=") {
			t.Error("ANTHROPIC_API_KEY should always be stripped")
		}
	}
}

// TestOllamaEnvStripsExistingAnthropicVars verifies that pre-existing
// ANTHROPIC_BASE_URL and ANTHROPIC_AUTH_TOKEN in the environment are
// replaced (not duplicated) when baseURL is set.
func TestOllamaEnvStripsExistingAnthropicVars(t *testing.T) {
	// Set env vars that should be replaced.
	t.Setenv("ANTHROPIC_BASE_URL", "https://api.anthropic.com")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "real-token")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")

	c := &Client{
		claudeBin: "echo",
		model:     "qwen2.5-coder:14b",
		timeout:   defaultTimeout,
		baseURL:   "http://192.168.1.207:11434",
	}

	env := c.buildEnv()

	// Count occurrences of each key.
	counts := map[string]int{
		"ANTHROPIC_BASE_URL":  0,
		"ANTHROPIC_AUTH_TOKEN": 0,
		"ANTHROPIC_API_KEY":   0,
	}
	for _, e := range env {
		for key := range counts {
			if strings.HasPrefix(e, key+"=") {
				counts[key]++
			}
		}
	}

	if counts["ANTHROPIC_BASE_URL"] != 1 {
		t.Errorf("ANTHROPIC_BASE_URL appears %d times, want 1", counts["ANTHROPIC_BASE_URL"])
	}
	if counts["ANTHROPIC_AUTH_TOKEN"] != 1 {
		t.Errorf("ANTHROPIC_AUTH_TOKEN appears %d times, want 1", counts["ANTHROPIC_AUTH_TOKEN"])
	}
	if counts["ANTHROPIC_API_KEY"] != 0 {
		t.Errorf("ANTHROPIC_API_KEY appears %d times, want 0", counts["ANTHROPIC_API_KEY"])
	}

	// Verify the injected values are correct (not the original env values).
	for _, e := range env {
		if strings.HasPrefix(e, "ANTHROPIC_BASE_URL=") {
			val := strings.TrimPrefix(e, "ANTHROPIC_BASE_URL=")
			if val != "http://192.168.1.207:11434" {
				t.Errorf("ANTHROPIC_BASE_URL value = %q, want Ollama URL", val)
			}
		}
		if strings.HasPrefix(e, "ANTHROPIC_AUTH_TOKEN=") {
			val := strings.TrimPrefix(e, "ANTHROPIC_AUTH_TOKEN=")
			if val != "ollama" {
				t.Errorf("ANTHROPIC_AUTH_TOKEN value = %q, want 'ollama'", val)
			}
		}
	}
}
