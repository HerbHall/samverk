package mcp_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"samverk.dev/samverk/internal/logstore"
	"samverk.dev/samverk/internal/metrics"
	"samverk.dev/samverk/internal/provider"
	internalmcp "samverk.dev/samverk/internal/mcp"
)

// mockPoolMetrics implements poolMetricsSource for testing.
type mockPoolMetrics struct {
	snap metrics.PoolSnapshot
}

func (m *mockPoolMetrics) Snapshot() metrics.PoolSnapshot {
	return m.snap
}

// mockProviderHealth implements providerHealthSource for testing.
type mockProviderHealth struct {
	health []provider.ProviderHealth
}

func (m *mockProviderHealth) AllHealth() []provider.ProviderHealth {
	return m.health
}

// mockLogQuerier implements logQuerier for testing.
type mockLogQuerier struct {
	entries []logstore.LogEntry
}

func (m *mockLogQuerier) Query(_ context.Context, _ logstore.QueryFilter) ([]logstore.LogEntry, error) {
	return m.entries, nil
}

// newTestMCPServerWithObservability sets up an httptest.Server with observability sources.
func newTestMCPServerWithObservability(
	t *testing.T,
	pool *mockPoolMetrics,
	health *mockProviderHealth,
	logs *mockLogQuerier,
) *httptest.Server {
	t.Helper()
	h := internalmcp.NewHandler(&mockTracker{}, nil, nil, nil, nil)
	if pool != nil {
		h.SetMetrics(pool, nil, nil)
	}
	if health != nil {
		h.SetProviderHealth(health)
	}
	if logs != nil {
		h.SetLogQuerier(logs)
	}
	handler := internalmcp.NewHTTPHandler(h)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

func TestListWorkers_Success(t *testing.T) {
	pool := &mockPoolMetrics{
		snap: metrics.PoolSnapshot{
			TotalWorkers:  3,
			ActiveWorkers: 2,
			IdleWorkers:   1,
			QueueDepth:    5,
		},
	}
	ts := newTestMCPServerWithObservability(t, pool, nil, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "list_workers",
			"arguments": map[string]any{},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in response")
	}
	text := result.Content[0].Text
	if !strings.Contains(text, `"total_workers":3`) {
		t.Errorf("expected total_workers:3, got %q", text)
	}
	if !strings.Contains(text, `"active_workers":2`) {
		t.Errorf("expected active_workers:2, got %q", text)
	}
}

func TestListWorkers_NilPool(t *testing.T) {
	ts := newTestMCPServerWithObservability(t, nil, nil, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "list_workers",
			"arguments": map[string]any{},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected protocol error: %s", resp.Error)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "not available") {
		t.Errorf("expected 'not available' message")
	}
}

func TestGetSessionLog_Success(t *testing.T) {
	logs := &mockLogQuerier{
		entries: []logstore.LogEntry{
			{
				ID:          2,
				Timestamp:   time.Date(2026, 3, 18, 10, 0, 1, 0, time.UTC),
				Level:       "info",
				Message:     "task completed",
				Component:   "agent",
				SessionID:   "sess-001",
				IssueNumber: 42,
			},
			{
				ID:          1,
				Timestamp:   time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC),
				Level:       "info",
				Message:     "task started",
				Component:   "agent",
				SessionID:   "sess-001",
				IssueNumber: 42,
			},
		},
	}
	ts := newTestMCPServerWithObservability(t, nil, nil, logs)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "get_session_log",
			"arguments": map[string]any{
				"issue_number": 42,
				"last_n_lines": 10,
			},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in response")
	}
	text := result.Content[0].Text
	// Should be in chronological order (reversed from DESC query).
	if !strings.Contains(text, "task started") || !strings.Contains(text, "task completed") {
		t.Errorf("expected both log entries, got %q", text)
	}
	if !strings.Contains(text, `"count":2`) {
		t.Errorf("expected count:2, got %q", text)
	}
}

func TestGetSessionLog_NilStore(t *testing.T) {
	ts := newTestMCPServerWithObservability(t, nil, nil, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "get_session_log",
			"arguments": map[string]any{
				"issue_number": 42,
			},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected protocol error: %s", resp.Error)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "not available") {
		t.Errorf("expected 'not available' message")
	}
}

func TestGetProviderHealth_Success(t *testing.T) {
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	health := &mockProviderHealth{
		health: []provider.ProviderHealth{
			{
				Name:        "ollama-nzxt",
				Healthy:     true,
				LastChecked: now,
				LastHealthy: now,
				ModelLoaded: true,
				VRAMFree:    8_000_000_000,
				VRAMTotal:   32_000_000_000,
			},
			{
				Name:        "claude-sonnet",
				Healthy:     true,
				LastChecked: now,
				LastHealthy: now,
			},
		},
	}
	ts := newTestMCPServerWithObservability(t, nil, health, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "get_provider_health",
			"arguments": map[string]any{},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in response")
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "ollama-nzxt") {
		t.Errorf("expected ollama-nzxt provider, got %q", text)
	}
	if !strings.Contains(text, "claude-sonnet") {
		t.Errorf("expected claude-sonnet provider, got %q", text)
	}
	if !strings.Contains(text, `"count":2`) {
		t.Errorf("expected count:2, got %q", text)
	}
}

func TestGetProviderHealth_RoutingChains(t *testing.T) {
	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	health := &mockProviderHealth{
		health: []provider.ProviderHealth{
			{
				Name:        "claude-sonnet",
				Healthy:     true,
				LastChecked: now,
				LastHealthy: now,
			},
		},
	}
	ts := newTestMCPServerWithObservability(t, nil, health, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      7,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "get_provider_health",
			"arguments": map[string]any{},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in response")
	}
	text := result.Content[0].Text

	// When no provider registry is set, routing_chains key must be absent.
	if strings.Contains(text, "routing_chains") {
		t.Errorf("expected no routing_chains without registry, got %q", text)
	}
}

func TestGetProviderHealth_WithRegistry(t *testing.T) {
	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	health := &mockProviderHealth{
		health: []provider.ProviderHealth{
			{
				Name:        "claude-sonnet",
				Healthy:     true,
				LastChecked: now,
				LastHealthy: now,
			},
		},
	}

	reg := provider.NewRegistry(nil)
	reg.SetRouting(map[string][]string{
		"default": {"claude-sonnet"},
		"triage":  {"claude-sonnet"},
	})

	h := internalmcp.NewHandler(&mockTracker{}, nil, nil, nil, nil)
	h.SetProviderHealth(health)
	h.SetProviderRegistry(reg)
	handler := internalmcp.NewHTTPHandler(h)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      8,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "get_provider_health",
			"arguments": map[string]any{},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in response")
	}
	text := result.Content[0].Text

	if !strings.Contains(text, "routing_chains") {
		t.Errorf("expected routing_chains in response, got %q", text)
	}
	if !strings.Contains(text, "default") {
		t.Errorf("expected 'default' chain in routing_chains, got %q", text)
	}
	if !strings.Contains(text, "triage") {
		t.Errorf("expected 'triage' chain in routing_chains, got %q", text)
	}
}

func TestGetProviderHealth_NilMonitor(t *testing.T) {
	ts := newTestMCPServerWithObservability(t, nil, nil, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      6,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "get_provider_health",
			"arguments": map[string]any{},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected protocol error: %s", resp.Error)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "not available") {
		t.Errorf("expected 'not available' message")
	}
}
