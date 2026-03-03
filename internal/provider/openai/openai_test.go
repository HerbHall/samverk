package openai

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
	c := New("test-api-key", "gpt-4o", WithBaseURL(srv.URL))
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
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header.
		if got := r.Header.Get("Authorization"); got != "Bearer test-api-key" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-api-key")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeJSON(t, w, http.StatusOK, chatResponse{
			Model: req.Model,
			Choices: []choice{
				{Message: chatMessage{Role: "assistant", Content: "Hello from OpenAI!"}},
			},
			Usage: usage{
				PromptTokens:     15,
				CompletionTokens: 8,
			},
		})
	})

	c, _ := newTestClient(t, mux)

	resp, err := c.Chat(context.Background(), provider.ChatRequest{
		Model: "gpt-4o",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if resp.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", resp.Model, "gpt-4o")
	}
	if resp.Message.Role != provider.RoleAssistant {
		t.Errorf("Message.Role = %q, want %q", resp.Message.Role, provider.RoleAssistant)
	}
	if resp.Message.Content != "Hello from OpenAI!" {
		t.Errorf("Message.Content = %q, want %q", resp.Message.Content, "Hello from OpenAI!")
	}
	if resp.PromptTokens != 15 {
		t.Errorf("PromptTokens = %d, want 15", resp.PromptTokens)
	}
	if resp.CompletionTokens != 8 {
		t.Errorf("CompletionTokens = %d, want 8", resp.CompletionTokens)
	}
}

func TestChatMapsAllRoles(t *testing.T) {
	var gotMessages []chatMessage

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotMessages = req.Messages

		writeJSON(t, w, http.StatusOK, chatResponse{
			Model: req.Model,
			Choices: []choice{
				{Message: chatMessage{Role: "assistant", Content: "response"}},
			},
			Usage: usage{PromptTokens: 10, CompletionTokens: 5},
		})
	})

	c, _ := newTestClient(t, mux)

	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Model: "gpt-4o",
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

	if len(gotMessages) != 4 {
		t.Fatalf("len(messages) = %d, want 4", len(gotMessages))
	}

	wantRoles := []string{"system", "user", "assistant", "user"}
	for i, want := range wantRoles {
		if gotMessages[i].Role != want {
			t.Errorf("messages[%d].Role = %q, want %q", i, gotMessages[i].Role, want)
		}
	}

	if gotMessages[0].Content != "You are a helpful assistant." {
		t.Errorf("messages[0].Content = %q, want system prompt", gotMessages[0].Content)
	}
}

func TestChatUsesClientModel(t *testing.T) {
	var gotModel string

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel = req.Model

		writeJSON(t, w, http.StatusOK, chatResponse{
			Model:   req.Model,
			Choices: []choice{{Message: chatMessage{Role: "assistant", Content: "ok"}}},
			Usage:   usage{PromptTokens: 1, CompletionTokens: 1},
		})
	})

	c, _ := newTestClient(t, mux)

	// Request specifies a different model -- client should use its configured model.
	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Model: "gpt-3.5-turbo",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if gotModel != "gpt-4o" {
		t.Errorf("sent model = %q, want client model %q", gotModel, "gpt-4o")
	}
}

func TestChatErrorResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"message":"rate limit exceeded"}}`, http.StatusTooManyRequests)
	})

	c, _ := newTestClient(t, mux)

	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Model: "gpt-4o",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err == nil {
		t.Fatal("expected error for 429 response, got nil")
	}
}

func TestChatEmptyChoices(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, chatResponse{
			Model:   "gpt-4o",
			Choices: []choice{},
			Usage:   usage{PromptTokens: 10, CompletionTokens: 0},
		})
	})

	c, _ := newTestClient(t, mux)

	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Model: "gpt-4o",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err == nil {
		t.Fatal("expected error for empty choices, got nil")
	}
}

func TestHealthy(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header is sent.
		if got := r.Header.Get("Authorization"); got != "Bearer test-api-key" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-api-key")
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"data": []map[string]string{{"id": "gpt-4o"}},
		})
	})

	c, _ := newTestClient(t, mux)

	if !c.Healthy(context.Background()) {
		t.Error("Healthy = false, want true")
	}
}

func TestHealthyUnauthorized(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})

	c, _ := newTestClient(t, mux)

	if c.Healthy(context.Background()) {
		t.Error("Healthy = true, want false for unauthorized")
	}
}

func TestHealthyDown(t *testing.T) {
	// Point at an invalid URL to simulate unreachable server.
	c := New("test-key", "gpt-4o", WithBaseURL("http://127.0.0.1:1"))

	if c.Healthy(context.Background()) {
		t.Error("Healthy = true, want false for unreachable server")
	}
}

func TestName(t *testing.T) {
	c := New("key", "gpt-4o")

	if got := c.Name(); got != "openai" {
		t.Errorf("Name() = %q, want %q", got, "openai")
	}
}

func TestCompileTimeInterfaceCheck(t *testing.T) {
	// Verifies the compile-time check at the top of openai.go.
	var _ provider.Provider = (*Client)(nil)
}

func TestWithMaxTokens(t *testing.T) {
	var gotMaxTokens int

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotMaxTokens = req.MaxTokens

		writeJSON(t, w, http.StatusOK, chatResponse{
			Model:   req.Model,
			Choices: []choice{{Message: chatMessage{Role: "assistant", Content: "ok"}}},
			Usage:   usage{PromptTokens: 1, CompletionTokens: 1},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := New("key", "gpt-4o", WithBaseURL(srv.URL), WithMaxTokens(8192))

	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Model: "gpt-4o",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if gotMaxTokens != 8192 {
		t.Errorf("max_tokens = %d, want 8192", gotMaxTokens)
	}
}

func TestDefaultMaxTokens(t *testing.T) {
	var gotMaxTokens int

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotMaxTokens = req.MaxTokens

		writeJSON(t, w, http.StatusOK, chatResponse{
			Model:   req.Model,
			Choices: []choice{{Message: chatMessage{Role: "assistant", Content: "ok"}}},
			Usage:   usage{PromptTokens: 1, CompletionTokens: 1},
		})
	})

	c, _ := newTestClient(t, mux)

	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Model: "gpt-4o",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if gotMaxTokens != 4096 {
		t.Errorf("default max_tokens = %d, want 4096", gotMaxTokens)
	}
}
