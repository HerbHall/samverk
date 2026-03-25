package synapset

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// newTestServer creates a mock MCP server that handles initialize, notifications/initialized,
// and tools/call requests. It returns the server and a pointer to the last tool call args.
func newTestServer(t *testing.T, toolHandler func(name string, args json.RawMessage) (interface{}, error)) *httptest.Server {
	t.Helper()

	sessionID := "test-session-123"

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", sessionID)
			resp := jsonRPCResponse{
				JSONRPC: "2.0",
				Result:  json.RawMessage(`{"protocolVersion":"2025-03-26","capabilities":{},"serverInfo":{"name":"synapset","version":"1.0.0"}}`),
				ID:      req.ID,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)

		case "tools/list":
			w.Header().Set("Content-Type", "application/json")
			resp := jsonRPCResponse{
				JSONRPC: "2.0",
				Result:  json.RawMessage(`{"tools":[]}`),
				ID:      req.ID,
			}
			_ = json.NewEncoder(w).Encode(resp)

		case "tools/call":
			raw, _ := json.Marshal(req.Params)
			var params toolCallParams
			if err := json.Unmarshal(raw, &params); err != nil {
				http.Error(w, "bad params", http.StatusBadRequest)
				return
			}

			argsRaw, _ := json.Marshal(params.Arguments)
			result, err := toolHandler(params.Name, argsRaw)
			if err != nil {
				resp := jsonRPCResponse{
					JSONRPC: "2.0",
					Error:   &jsonRPCError{Code: -32000, Message: err.Error()},
					ID:      req.ID,
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
				return
			}

			resultJSON, _ := json.Marshal(result)
			resp := jsonRPCResponse{
				JSONRPC: "2.0",
				Result:  resultJSON,
				ID:      req.ID,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		default:
			http.Error(w, "unknown method", http.StatusNotFound)
		}
	}))
}

func TestSessionInitialization(t *testing.T) {
	var initCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		switch req.Method {
		case "initialize":
			initCalls.Add(1)
			w.Header().Set("Mcp-Session-Id", "sess-abc")
			w.Header().Set("Content-Type", "application/json")
			resp := jsonRPCResponse{
				JSONRPC: "2.0",
				Result:  json.RawMessage(`{"protocolVersion":"2025-03-26"}`),
				ID:      req.ID,
			}
			_ = json.NewEncoder(w).Encode(resp)

		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)

		case "tools/list":
			w.Header().Set("Content-Type", "application/json")
			resp := jsonRPCResponse{JSONRPC: "2.0", Result: json.RawMessage(`{"tools":[]}`), ID: req.ID}
			_ = json.NewEncoder(w).Encode(resp)

		case "tools/call":
			result := toolResult{Content: []toolContent{{Type: "text", Text: "[]"}}}
			resultJSON, _ := json.Marshal(result)
			w.Header().Set("Content-Type", "application/json")
			resp := jsonRPCResponse{JSONRPC: "2.0", Result: resultJSON, ID: req.ID}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer srv.Close()

	client := New(Config{URL: srv.URL, Pool: "test"}, zap.NewNop())

	// First call should trigger initialization.
	_, err := client.SearchMemory(context.Background(), "test", "hello", 5)
	if err != nil {
		t.Fatalf("SearchMemory failed: %v", err)
	}

	// Second call should reuse the session.
	_, err = client.SearchMemory(context.Background(), "test", "world", 5)
	if err != nil {
		t.Fatalf("SearchMemory (second) failed: %v", err)
	}

	if got := initCalls.Load(); got != 1 {
		t.Errorf("initialize called %d times, want 1", got)
	}
}

func TestSearchMemory(t *testing.T) {
	memories := []Memory{
		{ID: "1", Content: "Use table-driven tests in Go", Category: "pattern", Similarity: 0.95},
		{ID: "2", Content: "Always run golangci-lint before committing", Category: "gotcha", Similarity: 0.88},
	}

	srv := newTestServer(t, func(name string, args json.RawMessage) (interface{}, error) {
		if name != "search_memory" {
			t.Errorf("unexpected tool call: %s", name)
		}
		var a map[string]interface{}
		_ = json.Unmarshal(args, &a)
		if a["pool"] != "devkit" {
			t.Errorf("pool = %v, want devkit", a["pool"])
		}

		memoriesJSON, _ := json.Marshal(memories)
		return toolResult{Content: []toolContent{{Type: "text", Text: string(memoriesJSON)}}}, nil
	})
	defer srv.Close()

	client := New(Config{URL: srv.URL, Pool: "test"}, zap.NewNop())
	results, err := client.SearchMemory(context.Background(), "devkit", "testing patterns", 5)
	if err != nil {
		t.Fatalf("SearchMemory failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("got %d memories, want 2", len(results))
	}
	if results[0].Content != "Use table-driven tests in Go" {
		t.Errorf("first memory content = %q", results[0].Content)
	}
}

func TestSearchAll(t *testing.T) {
	srv := newTestServer(t, func(name string, _ json.RawMessage) (interface{}, error) {
		if name != "search_all" {
			t.Errorf("unexpected tool call: %s", name)
		}
		memoriesJSON := `[{"id":"1","content":"cross-pool result","category":"pattern","similarity":0.9}]`
		return toolResult{Content: []toolContent{{Type: "text", Text: memoriesJSON}}}, nil
	})
	defer srv.Close()

	client := New(Config{URL: srv.URL, Pool: "test"}, zap.NewNop())
	results, err := client.SearchAll(context.Background(), "cross-pool query", 3)
	if err != nil {
		t.Fatalf("SearchAll failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
}

func TestStoreMemory(t *testing.T) {
	var capturedName string
	var capturedArgs map[string]interface{}

	srv := newTestServer(t, func(name string, args json.RawMessage) (interface{}, error) {
		capturedName = name
		_ = json.Unmarshal(args, &capturedArgs)
		return toolResult{Content: []toolContent{{Type: "text", Text: `{"id":"new-1","status":"stored"}`}}}, nil
	})
	defer srv.Close()

	client := New(Config{URL: srv.URL, Pool: "samverk"}, zap.NewNop())
	err := client.StoreMemory(context.Background(), "samverk",
		"Agent completed issue #42 with code-gen",
		"decision",
		[]string{"agent", "code-gen"},
		"issue-42",
	)
	if err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}
	if capturedName != "store_memory" {
		t.Errorf("tool name = %q, want store_memory", capturedName)
	}
	if capturedArgs["pool"] != "samverk" {
		t.Errorf("pool = %v, want samverk", capturedArgs["pool"])
	}
	if capturedArgs["category"] != "decision" {
		t.Errorf("category = %v, want decision", capturedArgs["category"])
	}
}

func TestGracefulDegradation_ServerUnavailable(t *testing.T) {
	// Point at a server that doesn't exist.
	client := New(Config{URL: "http://127.0.0.1:1", Pool: "test"}, zap.NewNop())
	client.httpClient.Timeout = 500 * time.Millisecond

	_, err := client.SearchMemory(context.Background(), "test", "query", 5)
	if err == nil {
		t.Fatal("expected error for unavailable server, got nil")
	}
}

func TestSessionReconnection(t *testing.T) {
	var initCalls atomic.Int32
	var toolCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		switch req.Method {
		case "initialize":
			initCalls.Add(1)
			w.Header().Set("Mcp-Session-Id", "sess-new")
			w.Header().Set("Content-Type", "application/json")
			resp := jsonRPCResponse{
				JSONRPC: "2.0",
				Result:  json.RawMessage(`{"protocolVersion":"2025-03-26"}`),
				ID:      req.ID,
			}
			_ = json.NewEncoder(w).Encode(resp)

		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)

		case "tools/list":
			w.Header().Set("Content-Type", "application/json")
			resp := jsonRPCResponse{JSONRPC: "2.0", Result: json.RawMessage(`{"tools":[]}`), ID: req.ID}
			_ = json.NewEncoder(w).Encode(resp)

		case "tools/call":
			count := toolCalls.Add(1)
			if count == 1 {
				// First tool call: simulate session expiry.
				http.Error(w, "session expired", http.StatusNotFound)
				return
			}
			// Subsequent calls succeed.
			result := toolResult{Content: []toolContent{{Type: "text", Text: "[]"}}}
			resultJSON, _ := json.Marshal(result)
			w.Header().Set("Content-Type", "application/json")
			resp := jsonRPCResponse{JSONRPC: "2.0", Result: resultJSON, ID: req.ID}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer srv.Close()

	client := New(Config{URL: srv.URL, Pool: "test"}, zap.NewNop())

	_, err := client.SearchMemory(context.Background(), "test", "reconnect test", 5)
	if err != nil {
		t.Fatalf("SearchMemory failed after reconnect: %v", err)
	}

	// Should have initialized twice: once on first call, once on reconnect.
	if got := initCalls.Load(); got != 2 {
		t.Errorf("initialize called %d times, want 2", got)
	}
}

func TestConfigFromEnv(t *testing.T) {
	// With defaults.
	cfg := ConfigFromEnv(Config{})
	if cfg.URL != DefaultURL {
		t.Errorf("URL = %q, want %q", cfg.URL, DefaultURL)
	}
	if cfg.Pool != DefaultPool {
		t.Errorf("Pool = %q, want %q", cfg.Pool, DefaultPool)
	}

	// With explicit defaults.
	cfg = ConfigFromEnv(Config{URL: "http://custom:8080/mcp", Pool: "custom-pool"})
	if cfg.URL != "http://custom:8080/mcp" {
		t.Errorf("URL = %q, want custom URL", cfg.URL)
	}
	if cfg.Pool != "custom-pool" {
		t.Errorf("Pool = %q, want custom-pool", cfg.Pool)
	}
}

func TestConfigFromEnv_AuthToken(t *testing.T) {
	t.Setenv("SYNAPSET_AUTH_TOKEN", "test-secret-token")
	cfg := ConfigFromEnv(Config{})
	if cfg.AuthToken != "test-secret-token" {
		t.Errorf("AuthToken = %q, want test-secret-token", cfg.AuthToken)
	}

	// Explicit default should be overridden by env.
	cfg = ConfigFromEnv(Config{AuthToken: "default-token"})
	if cfg.AuthToken != "test-secret-token" {
		t.Errorf("AuthToken = %q, want test-secret-token (env override)", cfg.AuthToken)
	}
}

func TestAuthTokenSentInRequests(t *testing.T) {
	var receivedAuth atomic.Value
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth.Store(r.Header.Get("Authorization"))

		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "auth-test-session")
			resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{}`)}
			_ = json.NewEncoder(w).Encode(resp)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"tools":[]}`)}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"content":[{"type":"text","text":"[]"}]}`)}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer ts.Close()

	client := New(Config{URL: ts.URL, Pool: "test", AuthToken: "my-secret"}, zap.NewNop())
	_, _ = client.SearchMemory(context.Background(), "test-pool", "test", 5)

	auth, _ := receivedAuth.Load().(string)
	if auth != "Bearer my-secret" {
		t.Errorf("Authorization header = %q, want 'Bearer my-secret'", auth)
	}
}

func TestNoAuthTokenWhenEmpty(t *testing.T) {
	var receivedAuth atomic.Value
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth.Store(r.Header.Get("Authorization"))

		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "noauth-test-session")
			resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{}`)}
			_ = json.NewEncoder(w).Encode(resp)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"tools":[]}`)}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"content":[{"type":"text","text":"[]"}]}`)}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer ts.Close()

	client := New(Config{URL: ts.URL, Pool: "test"}, zap.NewNop())
	_, _ = client.SearchMemory(context.Background(), "test-pool", "test", 5)

	auth, _ := receivedAuth.Load().(string)
	if auth != "" {
		t.Errorf("Authorization header = %q, want empty (no auth)", auth)
	}
}

func TestParseMemoriesWrapper(t *testing.T) {
	// Test the wrapper format: {"memories": [...]}
	text := `{"memories":[{"id":"1","content":"wrapped memory","category":"pattern"}]}`
	result := &toolResult{Content: []toolContent{{Type: "text", Text: text}}}

	memories, err := parseMemories(result)
	if err != nil {
		t.Fatalf("parseMemories failed: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("got %d memories, want 1", len(memories))
	}
	if memories[0].Content != "wrapped memory" {
		t.Errorf("content = %q, want 'wrapped memory'", memories[0].Content)
	}
}

func TestParseMemoriesNilResult(t *testing.T) {
	memories, err := parseMemories(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(memories) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(memories))
	}
}

func TestPool(t *testing.T) {
	client := New(Config{URL: "http://example.com", Pool: "my-pool"}, nil)
	if got := client.Pool(); got != "my-pool" {
		t.Errorf("Pool() = %q, want my-pool", got)
	}
}

func TestNonJSONResponseHandling(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		wantContain string
	}{
		{
			name:        "html error page",
			contentType: "text/html",
			body:        "<html><body>Page not found</body></html>",
			wantContain: "unexpected content-type",
		},
		{
			name:        "plain text error",
			contentType: "text/plain",
			body:        "please authenticate first",
			wantContain: "unexpected content-type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req jsonRPCRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}

				switch req.Method {
				case "initialize":
					w.Header().Set("Mcp-Session-Id", "sess-ct")
					w.Header().Set("Content-Type", "application/json")
					resp := jsonRPCResponse{
						JSONRPC: "2.0",
						Result:  json.RawMessage(`{"protocolVersion":"2025-03-26"}`),
						ID:      req.ID,
					}
					_ = json.NewEncoder(w).Encode(resp)
				case "notifications/initialized":
					w.WriteHeader(http.StatusAccepted)
				case "tools/list":
					w.Header().Set("Content-Type", "application/json")
					resp := jsonRPCResponse{
						JSONRPC: "2.0",
						Result:  json.RawMessage(`{"tools":[]}`),
						ID:      req.ID,
					}
					_ = json.NewEncoder(w).Encode(resp)
				case "tools/call":
					w.Header().Set("Content-Type", tt.contentType)
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(tt.body))
				}
			}))
			defer srv.Close()

			client := New(Config{URL: srv.URL, Pool: "test"}, zap.NewNop())
			_, err := client.SearchMemory(context.Background(), "test", "query", 5)
			if err == nil {
				t.Fatal("expected error for non-JSON response, got nil")
			}
			if got := err.Error(); !contains(got, tt.wantContain) {
				t.Errorf("error = %q, want it to contain %q", got, tt.wantContain)
			}
			if got := err.Error(); !contains(got, tt.body) {
				t.Errorf("error = %q, want it to contain body preview %q", got, tt.body)
			}
		})
	}
}

func TestResponseBodyPreviewInParseError(t *testing.T) {
	srv := newTestServer(t, func(_ string, _ json.RawMessage) (interface{}, error) {
		// Return a tool result with non-JSON text content.
		return toolResult{Content: []toolContent{{Type: "text", Text: "please retry later"}}}, nil
	})
	defer srv.Close()

	client := New(Config{URL: srv.URL, Pool: "test"}, zap.NewNop())
	_, err := client.SearchMemory(context.Background(), "test", "query", 5)
	if err == nil {
		t.Fatal("expected error for non-JSON tool text, got nil")
	}
	if got := err.Error(); !contains(got, "parse memories") {
		t.Errorf("error = %q, want it to contain 'parse memories'", got)
	}
	if got := err.Error(); !contains(got, "please retry later") {
		t.Errorf("error = %q, want it to contain body preview", got)
	}
}

func TestSessionInitDelay(t *testing.T) {
	srv := newTestServer(t, func(_ string, _ json.RawMessage) (interface{}, error) {
		return toolResult{Content: []toolContent{{Type: "text", Text: "[]"}}}, nil
	})
	defer srv.Close()

	client := New(Config{URL: srv.URL, Pool: "test"}, zap.NewNop())

	start := time.Now()
	_, err := client.SearchMemory(context.Background(), "test", "hello", 5)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("SearchMemory failed: %v", err)
	}
	// The post-init delay should be at least postInitDelay (150ms).
	if elapsed < postInitDelay {
		t.Errorf("elapsed %v < postInitDelay %v; init delay not applied", elapsed, postInitDelay)
	}
}

func TestBodyPreview(t *testing.T) {
	short := "short string"
	if got := bodyPreview([]byte(short)); got != short {
		t.Errorf("bodyPreview(%q) = %q", short, got)
	}

	long := strings.Repeat("x", 300)
	got := bodyPreview([]byte(long))
	if len(got) > bodyPreviewLen+3 { // +3 for "..."
		t.Errorf("bodyPreview of long string too long: %d chars", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("bodyPreview of long string should end with '...': %q", got)
	}

	// Whitespace trimming.
	if got := bodyPreview([]byte("  hello  ")); got != "hello" {
		t.Errorf("bodyPreview with whitespace = %q, want 'hello'", got)
	}
}

// contains is a helper to check substring presence.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

