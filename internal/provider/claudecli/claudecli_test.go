package claudecli

import (
	"context"
	"testing"

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
