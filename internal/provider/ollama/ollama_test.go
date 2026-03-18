package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/provider"
)

// newTestClient creates a Client pointed at a test HTTP server.
func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New(srv.URL)
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
	mux.HandleFunc("POST /api/chat", func(w http.ResponseWriter, r *http.Request) {
		var req provider.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeJSON(t, w, http.StatusOK, provider.ChatResponse{
			Model: req.Model,
			Message: provider.Message{
				Role:    provider.RoleAssistant,
				Content: "Hello from Ollama!",
			},
			PromptTokens:     10,
			CompletionTokens: 5,
		})
	})

	c, _ := newTestClient(t, mux)

	resp, err := c.Chat(context.Background(), provider.ChatRequest{
		Model: "llama3",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if resp.Model != "llama3" {
		t.Errorf("Model = %q, want %q", resp.Model, "llama3")
	}
	if resp.Message.Role != provider.RoleAssistant {
		t.Errorf("Message.Role = %q, want %q", resp.Message.Role, provider.RoleAssistant)
	}
	if resp.Message.Content != "Hello from Ollama!" {
		t.Errorf("Message.Content = %q, want %q", resp.Message.Content, "Hello from Ollama!")
	}
	if resp.PromptTokens != 10 {
		t.Errorf("PromptTokens = %d, want 10", resp.PromptTokens)
	}
	if resp.CompletionTokens != 5 {
		t.Errorf("CompletionTokens = %d, want 5", resp.CompletionTokens)
	}
}

func TestChatError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	c, _ := newTestClient(t, mux)

	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Model: "llama3",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestChatStreamAlwaysFalse(t *testing.T) {
	var gotStream bool

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if v, ok := raw["stream"].(bool); ok {
			gotStream = v
		}

		writeJSON(t, w, http.StatusOK, provider.ChatResponse{
			Model: "llama3",
			Message: provider.Message{
				Role:    provider.RoleAssistant,
				Content: "response",
			},
		})
	})

	c, _ := newTestClient(t, mux)

	// Pass stream: true -- the client should override to false.
	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Model:  "llama3",
		Stream: true,
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if gotStream {
		t.Error("stream was true in request body, want false")
	}
}

func TestHealthy(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ollama is running"))
	})

	c, _ := newTestClient(t, mux)

	if !c.Healthy(context.Background()) {
		t.Error("Healthy = false, want true")
	}
}

func TestHealthyDown(t *testing.T) {
	// Point at an invalid URL to simulate unreachable server.
	c := New("http://127.0.0.1:1")

	if c.Healthy(context.Background()) {
		t.Error("Healthy = true, want false for unreachable server")
	}
}

func TestName(t *testing.T) {
	c := New("http://localhost:11434")

	if got := c.Name(); got != "ollama" {
		t.Errorf("Name() = %q, want %q", got, "ollama")
	}
}

func TestGenerate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/generate", func(w http.ResponseWriter, r *http.Request) {
		var req GenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeJSON(t, w, http.StatusOK, GenerateResponse{
			Model:            req.Model,
			Response:         "Generated text",
			PromptTokens:     8,
			CompletionTokens: 12,
		})
	})

	c, _ := newTestClient(t, mux)

	resp, err := c.Generate(context.Background(), GenerateRequest{
		Model:  "llama3",
		Prompt: "Tell me a joke",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if resp.Model != "llama3" {
		t.Errorf("Model = %q, want %q", resp.Model, "llama3")
	}
	if resp.Response != "Generated text" {
		t.Errorf("Response = %q, want %q", resp.Response, "Generated text")
	}
	if resp.PromptTokens != 8 {
		t.Errorf("PromptTokens = %d, want 8", resp.PromptTokens)
	}
	if resp.CompletionTokens != 12 {
		t.Errorf("CompletionTokens = %d, want 12", resp.CompletionTokens)
	}
}

func TestPreload(t *testing.T) {
	var gotKeepAlive string

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/generate", func(w http.ResponseWriter, r *http.Request) {
		var req GenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if req.Prompt != "" {
			t.Errorf("Prompt = %q, want empty string for preload", req.Prompt)
		}
		gotKeepAlive = req.KeepAlive

		writeJSON(t, w, http.StatusOK, GenerateResponse{Model: req.Model})
	})

	c, _ := newTestClient(t, mux)

	err := c.Preload(context.Background(), "llama3", "5m")
	if err != nil {
		t.Fatalf("Preload: %v", err)
	}

	if gotKeepAlive != "5m" {
		t.Errorf("keep_alive = %q, want %q", gotKeepAlive, "5m")
	}
}

func TestUnload(t *testing.T) {
	var gotKeepAlive string

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/generate", func(w http.ResponseWriter, r *http.Request) {
		var req GenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if req.Prompt != "" {
			t.Errorf("Prompt = %q, want empty string for unload", req.Prompt)
		}
		gotKeepAlive = req.KeepAlive

		writeJSON(t, w, http.StatusOK, GenerateResponse{Model: req.Model})
	})

	c, _ := newTestClient(t, mux)

	err := c.Unload(context.Background(), "llama3")
	if err != nil {
		t.Fatalf("Unload: %v", err)
	}

	if gotKeepAlive != "0" {
		t.Errorf("keep_alive = %q, want %q", gotKeepAlive, "0")
	}
}

func TestListRunning(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ps", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, psResponse{
			Models: []RunningModel{
				{
					Name:      "llama3:latest",
					Model:     "llama3",
					Size:      4_000_000_000,
					SizeVRAM:  3_800_000_000,
					ExpiresAt: "2026-02-28T12:00:00Z",
				},
				{
					Name:      "codellama:7b",
					Model:     "codellama",
					Size:      3_500_000_000,
					SizeVRAM:  3_400_000_000,
					ExpiresAt: "2026-02-28T12:05:00Z",
				},
			},
		})
	})

	c, _ := newTestClient(t, mux)

	models, err := c.ListRunning(context.Background())
	if err != nil {
		t.Fatalf("ListRunning: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("len(models) = %d, want 2", len(models))
	}
	if models[0].Name != "llama3:latest" {
		t.Errorf("models[0].Name = %q, want %q", models[0].Name, "llama3:latest")
	}
	if models[0].SizeVRAM != 3_800_000_000 {
		t.Errorf("models[0].SizeVRAM = %d, want %d", models[0].SizeVRAM, int64(3_800_000_000))
	}
	if models[1].Model != "codellama" {
		t.Errorf("models[1].Model = %q, want %q", models[1].Model, "codellama")
	}
}

func TestPull(t *testing.T) {
	var gotReq pullRequest

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/pull", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeJSON(t, w, http.StatusOK, map[string]string{"status": "success"})
	})

	c, _ := newTestClient(t, mux)

	err := c.Pull(context.Background(), "llama3")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	if gotReq.Name != "llama3" {
		t.Errorf("Name = %q, want %q", gotReq.Name, "llama3")
	}
	if gotReq.Stream {
		t.Error("Stream = true, want false")
	}
}

func TestNewWithTimeout(t *testing.T) {
	timeout := 120 * time.Second
	c := NewWithTimeout("http://localhost:11434", timeout)

	if c.httpClient.Timeout != timeout {
		t.Errorf("httpClient.Timeout = %v, want %v", c.httpClient.Timeout, timeout)
	}
	if c.baseURL != "http://localhost:11434" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "http://localhost:11434")
	}
}

func TestNewWithTimeoutTrimsTrailingSlash(t *testing.T) {
	c := NewWithTimeout("http://localhost:11434/", 60*time.Second)

	if c.baseURL != "http://localhost:11434" {
		t.Errorf("baseURL = %q, want trailing slash trimmed", c.baseURL)
	}
}

func TestNewWithTimeoutServesRequests(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ollama is running"))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewWithTimeout(srv.URL, 5*time.Second)

	if !c.Healthy(context.Background()) {
		t.Error("Healthy = false, want true for NewWithTimeout client")
	}
}

func TestCompileTimeInterfaceCheck(t *testing.T) {
	// This test verifies the compile-time check at the top of ollama.go.
	// If the var _ provider.Provider = (*Client)(nil) line compiles,
	// Client satisfies the Provider interface.
	var _ provider.Provider = (*Client)(nil)
}

func TestChatDeadlineExceeded_WrapsAsErrProviderTimeout(t *testing.T) {
	// Use a server that delays longer than the context deadline.
	// The handler writes nothing, so the client sees the context expire.
	done := make(chan struct{})
	defer close(done)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", func(w http.ResponseWriter, _ *http.Request) {
		// Wait until test cleanup signals us to stop, or a generous timeout.
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewWithTimeout(srv.URL, 10*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.Chat(ctx, provider.ChatRequest{
		Model: "qwen2.5-coder:14b",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	})
	if err == nil {
		t.Fatal("expected error on context deadline, got nil")
	}

	// Verify the error is an ErrProviderTimeout.
	var timeout *provider.ErrProviderTimeout
	if !errors.As(err, &timeout) {
		t.Fatalf("expected ErrProviderTimeout, got %T: %v", err, err)
	}
	if timeout.Provider != "ollama" {
		t.Errorf("Provider = %q, want %q", timeout.Provider, "ollama")
	}
	if timeout.Model != "qwen2.5-coder:14b" {
		t.Errorf("Model = %q, want %q", timeout.Model, "qwen2.5-coder:14b")
	}

	// Verify it's retryable (the pool uses this to trigger failover).
	if !provider.IsRetryable(err) {
		t.Error("expected IsRetryable=true for deadline exceeded error")
	}
}

func TestChatNonTimeout_NotWrappedAsTimeout(t *testing.T) {
	// Verify that non-timeout errors (e.g., 500) are NOT wrapped as
	// ErrProviderTimeout -- only infrastructure timeouts should failover.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model overloaded", http.StatusServiceUnavailable)
	})

	c, _ := newTestClient(t, mux)

	_, err := c.Chat(context.Background(), provider.ChatRequest{
		Model: "llama3",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err == nil {
		t.Fatal("expected error for 503, got nil")
	}

	// Should NOT be retryable -- this is a quality/server error, not a timeout.
	if provider.IsRetryable(err) {
		t.Error("expected IsRetryable=false for non-timeout error")
	}
}
