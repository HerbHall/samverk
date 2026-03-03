package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/herbhall/samverk/internal/provider"
)

// newTestClient creates a Client pointed at a test HTTP server.
func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New("test-api-key", "claude-sonnet-4-20250514", WithBaseURL(srv.URL))
	return c, srv
}

// writeJSON is a test helper that writes a JSON response.
func writeJSON(t *testing.T, w http.ResponseWriter, status int, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}

func TestChat(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, r *http.Request) {
		// Verify headers.
		if got := r.Header.Get("x-api-key"); got != "test-api-key" {
			t.Errorf("x-api-key = %q, want %q", got, "test-api-key")
		}
		if got := r.Header.Get("anthropic-version"); got != apiVersion {
			t.Errorf("anthropic-version = %q, want %q", got, apiVersion)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}

		var req claudeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if req.Model != "claude-sonnet-4-20250514" {
			t.Errorf("Model = %q, want %q", req.Model, "claude-sonnet-4-20250514")
		}
		if req.MaxTokens != defaultMaxTokens {
			t.Errorf("MaxTokens = %d, want %d", req.MaxTokens, defaultMaxTokens)
		}

		writeJSON(t, w, http.StatusOK, claudeResponse{
			Content: []contentBlock{
				{Type: "text", Text: "Hello from Claude!"},
			},
			Model: req.Model,
			Usage: claudeUsage{
				InputTokens:  15,
				OutputTokens: 8,
			},
		})
	})

	c, _ := newTestClient(t, mux)

	resp, err := c.Chat(context.Background(), provider.ChatRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if resp.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want %q", resp.Model, "claude-sonnet-4-20250514")
	}
	if resp.Message.Role != provider.RoleAssistant {
		t.Errorf("Message.Role = %q, want %q", resp.Message.Role, provider.RoleAssistant)
	}
	if resp.Message.Content != "Hello from Claude!" {
		t.Errorf("Message.Content = %q, want %q", resp.Message.Content, "Hello from Claude!")
	}
	if resp.PromptTokens != 15 {
		t.Errorf("PromptTokens = %d, want 15", resp.PromptTokens)
	}
	if resp.CompletionTokens != 8 {
		t.Errorf("CompletionTokens = %d, want 8", resp.CompletionTokens)
	}
}

func TestChatSystemMessageExtraction(t *testing.T) {
	var gotReq claudeRequest

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeJSON(t, w, http.StatusOK, claudeResponse{
			Content: []contentBlock{
				{Type: "text", Text: "response"},
			},
			Model: gotReq.Model,
			Usage: claudeUsage{InputTokens: 10, OutputTokens: 5},
		})
	})

	c, _ := newTestClient(t, mux)

	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "You are a helpful assistant."},
			{Role: provider.RoleUser, Content: "Hi"},
			{Role: provider.RoleAssistant, Content: "Hello!"},
			{Role: provider.RoleUser, Content: "How are you?"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	// System message should be extracted into the top-level system field.
	if gotReq.System != "You are a helpful assistant." {
		t.Errorf("System = %q, want %q", gotReq.System, "You are a helpful assistant.")
	}

	// Only non-system messages should remain in the messages array.
	if len(gotReq.Messages) != 3 {
		t.Fatalf("len(Messages) = %d, want 3", len(gotReq.Messages))
	}
	if gotReq.Messages[0].Role != "user" {
		t.Errorf("Messages[0].Role = %q, want %q", gotReq.Messages[0].Role, "user")
	}
	if gotReq.Messages[1].Role != "assistant" {
		t.Errorf("Messages[1].Role = %q, want %q", gotReq.Messages[1].Role, "assistant")
	}
	if gotReq.Messages[2].Role != "user" {
		t.Errorf("Messages[2].Role = %q, want %q", gotReq.Messages[2].Role, "user")
	}
}

func TestChatMultipleSystemMessages(t *testing.T) {
	var gotReq claudeRequest

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeJSON(t, w, http.StatusOK, claudeResponse{
			Content: []contentBlock{{Type: "text", Text: "ok"}},
			Model:   gotReq.Model,
			Usage:   claudeUsage{InputTokens: 5, OutputTokens: 1},
		})
	})

	c, _ := newTestClient(t, mux)

	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "You are helpful."},
			{Role: provider.RoleSystem, Content: "Be concise."},
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	// Multiple system messages should be joined with newline.
	want := "You are helpful.\nBe concise."
	if gotReq.System != want {
		t.Errorf("System = %q, want %q", gotReq.System, want)
	}

	if len(gotReq.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(gotReq.Messages))
	}
}

func TestChatDefaultModel(t *testing.T) {
	var gotReq claudeRequest

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeJSON(t, w, http.StatusOK, claudeResponse{
			Content: []contentBlock{{Type: "text", Text: "ok"}},
			Model:   gotReq.Model,
			Usage:   claudeUsage{InputTokens: 5, OutputTokens: 1},
		})
	})

	c, _ := newTestClient(t, mux)

	// Send request without model -- should use client default.
	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if gotReq.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want %q", gotReq.Model, "claude-sonnet-4-20250514")
	}
}

func TestChatError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"type":"invalid_request_error","message":"bad request"}}`, http.StatusBadRequest)
	})

	c, _ := newTestClient(t, mux)

	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err == nil {
		t.Fatal("expected error for 400 response, got nil")
	}
}

func TestChatServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	c, _ := newTestClient(t, mux)

	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestHealthy(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, claudeResponse{
			Content: []contentBlock{{Type: "text", Text: "p"}},
			Model:   "claude-sonnet-4-20250514",
			Usage:   claudeUsage{InputTokens: 1, OutputTokens: 1},
		})
	})

	c, _ := newTestClient(t, mux)

	if !c.Healthy(context.Background()) {
		t.Error("Healthy = false, want true")
	}
}

func TestHealthyDown(t *testing.T) {
	// Point at an invalid URL to simulate unreachable server.
	c := New("test-key", "test-model", WithBaseURL("http://127.0.0.1:1"))

	if c.Healthy(context.Background()) {
		t.Error("Healthy = true, want false for unreachable server")
	}
}

func TestHealthyNon200(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})

	c, _ := newTestClient(t, mux)

	if c.Healthy(context.Background()) {
		t.Error("Healthy = true, want false for 401 response")
	}
}

func TestName(t *testing.T) {
	c := New("key", "model")

	if got := c.Name(); got != "claude" {
		t.Errorf("Name() = %q, want %q", got, "claude")
	}
}

func TestWithMaxTokens(t *testing.T) {
	var gotReq claudeRequest

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeJSON(t, w, http.StatusOK, claudeResponse{
			Content: []contentBlock{{Type: "text", Text: "ok"}},
			Model:   gotReq.Model,
			Usage:   claudeUsage{InputTokens: 5, OutputTokens: 1},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := New("test-key", "test-model", WithBaseURL(srv.URL), WithMaxTokens(8192))

	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if gotReq.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, want 8192", gotReq.MaxTokens)
	}
}

func TestCompileTimeInterfaceCheck(t *testing.T) {
	// This test verifies the compile-time check at the top of claude.go.
	// If the var _ provider.Provider = (*Client)(nil) line compiles,
	// Client satisfies the Provider interface.
	var _ provider.Provider = (*Client)(nil)
}
