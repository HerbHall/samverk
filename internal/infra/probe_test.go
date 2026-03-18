package infra

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/synapset"
	"go.uber.org/zap"
)

func TestProbeOllama(t *testing.T) {
	tagsResp := ollamaTagsResponse{
		Models: []ollamaModel{
			{Name: "qwen2.5-coder:14b", Size: 9000000000},
			{Name: "llama3:8b", Size: 4000000000},
		},
	}
	psResp := ollamaPsResponse{
		Models: []ollamaPsModel{
			{Name: "qwen2.5-coder:14b"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tagsResp)
		case "/api/ps":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(psResp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := NewProber(nil, zap.NewNop(), nil)
	ep := Endpoint{Name: "test-ollama", URL: srv.URL, Kind: "ollama", Source: "test"}

	result := p.probeOllama(context.Background(), ep)

	if !result.Reachable {
		t.Fatalf("expected reachable, got error: %s", result.Error)
	}
	if got := result.Fields["models"]; !strings.Contains(got, "llama3:8b") {
		t.Errorf("models = %q, want to contain llama3:8b", got)
	}
	if got := result.Fields["models"]; !strings.Contains(got, "qwen2.5-coder:14b") {
		t.Errorf("models = %q, want to contain qwen2.5-coder:14b", got)
	}
	if got := result.Fields["running_models"]; got != "qwen2.5-coder:14b" {
		t.Errorf("running_models = %q, want qwen2.5-coder:14b", got)
	}
}

func TestProbeOllamaUnreachable(t *testing.T) {
	p := NewProber(nil, zap.NewNop(), nil)
	p.httpClient.Timeout = 500 * time.Millisecond

	ep := Endpoint{Name: "dead-host", URL: "http://127.0.0.1:1", Kind: "ollama", Source: "dead"}
	result := p.probeOllama(context.Background(), ep)

	if result.Reachable {
		t.Fatal("expected unreachable for dead host")
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error for dead host")
	}
}

func TestProbeSamverk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(samverkHealthResponse{Status: "ok"})
		case "/api/v1/status":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(samverkStatusResponse{Version: "v0.1.17", Workers: 3})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := NewProber(nil, zap.NewNop(), nil)
	ep := Endpoint{Name: "test-samverk", URL: srv.URL, Kind: "samverk", Source: "test"}

	result := p.probeSamverk(context.Background(), ep)

	if !result.Reachable {
		t.Fatalf("expected reachable, got error: %s", result.Error)
	}
	if got := result.Fields["health"]; got != "ok" {
		t.Errorf("health = %q, want ok", got)
	}
	if got := result.Fields["version"]; got != "v0.1.17" {
		t.Errorf("version = %q, want v0.1.17", got)
	}
	if got := result.Fields["workers"]; got != "3" {
		t.Errorf("workers = %q, want 3", got)
	}
}

func TestProbeGitea(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/version" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(giteaVersionResponse{Version: "1.22.0"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	p := NewProber(nil, zap.NewNop(), nil)
	ep := Endpoint{Name: "test-gitea", URL: srv.URL, Kind: "gitea", Source: "test"}

	result := p.probeGitea(context.Background(), ep)

	if !result.Reachable {
		t.Fatalf("expected reachable, got error: %s", result.Error)
	}
	if got := result.Fields["version"]; got != "1.22.0" {
		t.Errorf("version = %q, want 1.22.0", got)
	}
}

func TestProbeEndpointUnknownKind(t *testing.T) {
	p := NewProber(nil, zap.NewNop(), nil)
	ep := Endpoint{Name: "mystery", URL: "http://localhost:1", Kind: "unknown", Source: "test"}

	result := p.probeEndpoint(context.Background(), ep)

	if result.Reachable {
		t.Fatal("expected unreachable for unknown kind")
	}
	if !strings.Contains(result.Error, "unknown endpoint kind") {
		t.Errorf("error = %q, want 'unknown endpoint kind'", result.Error)
	}
}

func TestFormatProbeFields(t *testing.T) {
	result := ProbeResult{
		Endpoint: Endpoint{Name: "vm-300", Kind: "ollama"},
		Fields: map[string]string{
			"models":         "llama3:8b, qwen2.5-coder:14b",
			"running_models": "qwen2.5-coder:14b",
		},
	}

	got := formatProbeFields(result)

	if !strings.HasPrefix(got, "vm-300 (ollama): ") {
		t.Errorf("summary prefix = %q, want 'vm-300 (ollama): '", got)
	}
	// Fields should be sorted alphabetically.
	modelsIdx := strings.Index(got, "models=")
	runningIdx := strings.Index(got, "running_models=")
	if modelsIdx >= runningIdx {
		t.Errorf("fields not sorted: models at %d, running_models at %d", modelsIdx, runningIdx)
	}
}

func TestSyncToSynapsetNoChange(t *testing.T) {
	// Mock Synapset that returns existing memory matching probed state.
	existingContent := "vm-300 (ollama): models=llama3:8b"

	srv := newMCPServer(t, func(name string, args json.RawMessage) (interface{}, error) {
		switch name {
		case "query_memory":
			mem := synapset.Memory{
				ID:      "mem-1",
				Content: existingContent,
				Source:  "vm-300",
			}
			memJSON, _ := json.Marshal([]synapset.Memory{mem})
			return mcpToolResult{Content: []mcpToolContent{{Type: "text", Text: string(memJSON)}}}, nil
		case "update_memory":
			t.Error("update_memory called when content unchanged")
			return mcpToolResult{Content: []mcpToolContent{{Type: "text", Text: `{"status":"ok"}`}}}, nil
		default:
			return mcpToolResult{Content: []mcpToolContent{{Type: "text", Text: "[]"}}}, nil
		}
	})
	defer srv.Close()

	sc := synapset.New(synapset.Config{URL: srv.URL, Pool: "machines"}, zap.NewNop())
	p := NewProber(sc, zap.NewNop(), nil)

	result := ProbeResult{
		Endpoint:  Endpoint{Name: "vm-300", Kind: "ollama", Source: "vm-300"},
		Reachable: true,
		Fields:    map[string]string{"models": "llama3:8b"},
	}

	err := p.syncToSynapset(context.Background(), result)
	if err != nil {
		t.Fatalf("syncToSynapset failed: %v", err)
	}
}

func TestSyncToSynapsetWithChange(t *testing.T) {
	var updateCalled bool

	srv := newMCPServer(t, func(name string, args json.RawMessage) (interface{}, error) {
		switch name {
		case "query_memory":
			mem := synapset.Memory{
				ID:      "mem-1",
				Content: "vm-300 (ollama): models=old-model:7b",
				Source:  "vm-300",
			}
			memJSON, _ := json.Marshal([]synapset.Memory{mem})
			return mcpToolResult{Content: []mcpToolContent{{Type: "text", Text: string(memJSON)}}}, nil
		case "update_memory":
			updateCalled = true
			return mcpToolResult{Content: []mcpToolContent{{Type: "text", Text: `{"status":"ok"}`}}}, nil
		default:
			return mcpToolResult{Content: []mcpToolContent{{Type: "text", Text: "[]"}}}, nil
		}
	})
	defer srv.Close()

	sc := synapset.New(synapset.Config{URL: srv.URL, Pool: "machines"}, zap.NewNop())
	p := NewProber(sc, zap.NewNop(), nil)

	result := ProbeResult{
		Endpoint:  Endpoint{Name: "vm-300", Kind: "ollama", Source: "vm-300"},
		Reachable: true,
		Fields:    map[string]string{"models": "new-model:14b"},
	}

	err := p.syncToSynapset(context.Background(), result)
	if err != nil {
		t.Fatalf("syncToSynapset failed: %v", err)
	}
	if !updateCalled {
		t.Error("update_memory not called for changed content")
	}
}

func TestSyncToSynapsetNewMemory(t *testing.T) {
	var storeCalled bool

	srv := newMCPServer(t, func(name string, args json.RawMessage) (interface{}, error) {
		switch name {
		case "query_memory":
			memJSON, _ := json.Marshal([]synapset.Memory{})
			return mcpToolResult{Content: []mcpToolContent{{Type: "text", Text: string(memJSON)}}}, nil
		case "store_memory":
			storeCalled = true
			return mcpToolResult{Content: []mcpToolContent{{Type: "text", Text: `{"id":"new-1","status":"stored"}`}}}, nil
		default:
			return mcpToolResult{Content: []mcpToolContent{{Type: "text", Text: "[]"}}}, nil
		}
	})
	defer srv.Close()

	sc := synapset.New(synapset.Config{URL: srv.URL, Pool: "machines"}, zap.NewNop())
	p := NewProber(sc, zap.NewNop(), nil)

	result := ProbeResult{
		Endpoint:  Endpoint{Name: "new-host", Kind: "gitea", Source: "new-host"},
		Reachable: true,
		Fields:    map[string]string{"version": "1.22.0"},
	}

	err := p.syncToSynapset(context.Background(), result)
	if err != nil {
		t.Fatalf("syncToSynapset failed: %v", err)
	}
	if !storeCalled {
		t.Error("store_memory not called for new memory")
	}
}

func TestSyncToSynapsetNilClient(t *testing.T) {
	p := NewProber(nil, zap.NewNop(), nil)
	result := ProbeResult{
		Endpoint:  Endpoint{Name: "test", Kind: "ollama", Source: "test"},
		Reachable: true,
		Fields:    map[string]string{"models": "test:7b"},
	}

	err := p.syncToSynapset(context.Background(), result)
	if err != nil {
		t.Fatalf("expected nil error for nil client, got %v", err)
	}
}

func TestRunIntegration(t *testing.T) {
	// Set up mock Ollama and Gitea servers.
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			resp := ollamaTagsResponse{Models: []ollamaModel{{Name: "test:7b", Size: 1000}}}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		case "/api/ps":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ollamaPsResponse{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollamaSrv.Close()

	giteaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/version" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(giteaVersionResponse{Version: "1.22.0"})
			return
		}
		http.NotFound(w, r)
	}))
	defer giteaSrv.Close()

	endpoints := []Endpoint{
		{Name: "test-ollama", URL: ollamaSrv.URL, Kind: "ollama", Source: "test-ollama"},
		{Name: "test-gitea", URL: giteaSrv.URL, Kind: "gitea", Source: "test-gitea"},
		// Include an unreachable endpoint to verify graceful handling.
		{Name: "dead", URL: "http://127.0.0.1:1", Kind: "ollama", Source: "dead"},
	}

	p := NewProber(nil, zap.NewNop(), endpoints)
	p.httpClient.Timeout = 500 * time.Millisecond

	results := p.Run(context.Background())

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	// First: reachable ollama.
	if !results[0].Reachable {
		t.Errorf("test-ollama: expected reachable, got error: %s", results[0].Error)
	}
	if got := results[0].Fields["models"]; got != "test:7b" {
		t.Errorf("test-ollama models = %q, want test:7b", got)
	}

	// Second: reachable gitea.
	if !results[1].Reachable {
		t.Errorf("test-gitea: expected reachable, got error: %s", results[1].Error)
	}
	if got := results[1].Fields["version"]; got != "1.22.0" {
		t.Errorf("test-gitea version = %q, want 1.22.0", got)
	}

	// Third: unreachable.
	if results[2].Reachable {
		t.Error("dead host: expected unreachable")
	}
}

func TestDefaultEndpoints(t *testing.T) {
	eps := DefaultEndpoints()
	if len(eps) < 5 {
		t.Errorf("got %d default endpoints, want at least 5", len(eps))
	}

	kinds := make(map[string]int)
	for i := range eps {
		kinds[eps[i].Kind]++
	}
	if kinds["ollama"] < 3 {
		t.Errorf("want at least 3 ollama endpoints, got %d", kinds["ollama"])
	}
	if kinds["samverk"] < 1 {
		t.Errorf("want at least 1 samverk endpoint, got %d", kinds["samverk"])
	}
	if kinds["gitea"] < 1 {
		t.Errorf("want at least 1 gitea endpoint, got %d", kinds["gitea"])
	}
}

// --- MCP test server helpers ---

type mcpToolResult struct {
	Content []mcpToolContent `json:"content"`
}

type mcpToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpJSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      *int            `json:"id,omitempty"`
}

type mcpJSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	ID      *int            `json:"id,omitempty"`
}

type mcpToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func newMCPServer(t *testing.T, toolHandler func(name string, args json.RawMessage) (interface{}, error)) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mcpJSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "test-session")
			w.Header().Set("Content-Type", "application/json")
			resp := mcpJSONRPCResponse{
				JSONRPC: "2.0",
				Result:  json.RawMessage(`{"protocolVersion":"2025-03-26","capabilities":{},"serverInfo":{"name":"synapset","version":"1.0.0"}}`),
				ID:      req.ID,
			}
			_ = json.NewEncoder(w).Encode(resp)

		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)

		case "tools/list":
			w.Header().Set("Content-Type", "application/json")
			resp := mcpJSONRPCResponse{JSONRPC: "2.0", Result: json.RawMessage(`{"tools":[]}`), ID: req.ID}
			_ = json.NewEncoder(w).Encode(resp)

		case "tools/call":
			raw, _ := json.Marshal(req.Params)
			var params mcpToolCallParams
			if err := json.Unmarshal(raw, &params); err != nil {
				http.Error(w, "bad params", http.StatusBadRequest)
				return
			}

			result, err := toolHandler(params.Name, params.Arguments)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			resultJSON, _ := json.Marshal(result)
			resp := mcpJSONRPCResponse{JSONRPC: "2.0", Result: resultJSON, ID: req.ID}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		default:
			http.Error(w, "unknown method", http.StatusNotFound)
		}
	}))
}
