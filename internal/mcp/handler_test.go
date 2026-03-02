package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/digest"
	"github.com/herbhall/samverk/internal/forge"
	internalmcp "github.com/herbhall/samverk/internal/mcp"
)

// mockTracker implements forge.IssueTracker for testing.
type mockTracker struct {
	issues []*forge.Issue
}

func (m *mockTracker) CreateIssue(_ context.Context, _ *forge.CreateIssueRequest) (*forge.Issue, error) {
	return nil, nil
}

func (m *mockTracker) GetIssue(_ context.Context, number int) (*forge.Issue, error) {
	for _, iss := range m.issues {
		if iss.Number == number {
			return iss, nil
		}
	}
	return nil, nil
}

func (m *mockTracker) UpdateIssue(_ context.Context, _ int, _ *forge.UpdateIssueRequest) (*forge.Issue, error) {
	return nil, nil
}

func (m *mockTracker) ListIssues(_ context.Context, opts *forge.ListOptions) ([]*forge.Issue, error) {
	result := make([]*forge.Issue, 0, len(m.issues))
	for _, iss := range m.issues {
		if opts != nil && opts.State != "" && string(iss.State) != string(opts.State) {
			continue
		}
		if opts != nil && len(opts.Labels) > 0 {
			if !hasAllLabels(iss.Labels, opts.Labels) {
				continue
			}
		}
		result = append(result, iss)
	}
	return result, nil
}

func hasAllLabels(issueLabels, required []string) bool {
	set := make(map[string]bool, len(issueLabels))
	for _, l := range issueLabels {
		set[l] = true
	}
	for _, r := range required {
		if !set[r] {
			return false
		}
	}
	return true
}

func (m *mockTracker) AddComment(_ context.Context, _ int, _ string) (*forge.Comment, error) {
	return nil, nil
}

func (m *mockTracker) ListComments(_ context.Context, _ int) ([]*forge.Comment, error) {
	return nil, nil
}

func (m *mockTracker) SetLabels(_ context.Context, _ int, _ []string) error { return nil }
func (m *mockTracker) AddLabel(_ context.Context, _ int, _ string) error    { return nil }
func (m *mockTracker) RemoveLabel(_ context.Context, _ int, _ string) error { return nil }
func (m *mockTracker) Assign(_ context.Context, _ int, _ string) error      { return nil }
func (m *mockTracker) Unassign(_ context.Context, _ int, _ string) error    { return nil }
func (m *mockTracker) Watch(_ context.Context, _ func(forge.Event)) error   { return nil }

// mockCostSource implements digest.CostSource for testing.
type mockCostSource struct {
	summary digest.CostSummary
}

func (m *mockCostSource) ComputeCostSince(_ context.Context, _ time.Time) digest.CostSummary {
	return m.summary
}

// jsonRPCRequest is a minimal JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// jsonRPCResponse is a minimal JSON-RPC 2.0 response for test assertions.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error,omitempty"`
}

// toolsListResult is the result of a tools/list response.
type toolsListResult struct {
	Tools []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"tools"`
}

// callToolResult is the result of a tools/call response.
type callToolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError,omitempty"`
}

// newTestMCPServer sets up an httptest.Server serving the MCP handler.
func newTestMCPServer(t *testing.T, tracker forge.IssueTracker, costs digest.CostSource) *httptest.Server {
	t.Helper()
	h := internalmcp.NewHandler(tracker, costs)
	handler := internalmcp.NewHTTPHandler(h)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

// postJSON sends a JSON-RPC request and returns the response body.
func postJSON(t *testing.T, url string, req jsonRPCRequest) []byte {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, url,
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return respBody
}

func TestToolsListDiscovery(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
		Params:  map[string]any{},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var result toolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Verify both tools are registered.
	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	if !toolNames["get_digest"] {
		t.Error("get_digest tool not found in tools/list")
	}
	if !toolNames["get_cost_summary"] {
		t.Error("get_cost_summary tool not found in tools/list")
	}
	if len(result.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(result.Tools))
	}
}

func TestGetDigestTool(t *testing.T) {
	now := time.Now()
	closedAt := now.Add(-30 * time.Minute)

	tracker := &mockTracker{
		issues: []*forge.Issue{
			{
				Number: 1, Title: "Merge PR #10", State: forge.StateOpen,
				Labels:    []string{"status:needs-human"},
				CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-1 * time.Hour),
				Body: "---\ntype: block\npriority: critical\n---\n\n## Context\n\nTests passed.\n",
			},
			{
				Number: 2, Title: "Completed task", State: forge.StateClosed,
				CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now, ClosedAt: &closedAt,
				Body: "---\ntype: task\n---\n\n## Result\n\nDone.\n",
			},
			{
				Number: 3, Title: "Queued work", State: forge.StateOpen,
				Labels:    []string{"status:queued"},
				CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now,
			},
		},
	}

	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "get_digest",
			"arguments": map[string]any{"since": "48h"},
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

	checks := []string{
		"SAMVERK:",
		"NEEDS YOUR DECISION",
		"Merge PR #10",
		"Queued: 1 issue waiting",
	}
	for _, check := range checks {
		if !strings.Contains(text, check) {
			t.Errorf("digest output missing %q", check)
		}
	}
}

func TestGetDigestToolInvalidDuration(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "get_digest",
			"arguments": map[string]any{"since": "not-a-duration"},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}

	// The go-sdk wraps tool errors into CallToolResult with isError=true.
	if resp.Error != nil {
		// Protocol-level error is also acceptable.
		return
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if !result.IsError {
		t.Error("expected isError=true for invalid duration")
	}
}

func TestGetCostSummaryToolNilCosts(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil) // nil CostSource

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "get_cost_summary",
			"arguments": map[string]any{"since": "24h"},
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

	if !strings.Contains(result.Content[0].Text, "no cost data available") {
		t.Errorf("expected 'no cost data available', got %q", result.Content[0].Text)
	}
}

func TestGetCostSummaryToolWithCosts(t *testing.T) {
	tracker := &mockTracker{}
	costs := &mockCostSource{
		summary: digest.CostSummary{
			TokensUsed:         50000,
			EstimatedCostUSD:   2.50,
			BudgetRemainingUSD: 47.50,
		},
	}
	ts := newTestMCPServer(t, tracker, costs)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "get_cost_summary",
			"arguments": map[string]any{"since": "24h"},
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
	checks := []string{
		"50000",
		"$2.50",
		"$47.50",
	}
	for _, check := range checks {
		if !strings.Contains(text, check) {
			t.Errorf("cost summary missing %q", check)
		}
	}
}

func TestMCPServerInfo(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  map[string]any{},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	// Check server info is present in the initialize response.
	var initResult struct {
		ServerInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(resp.Result, &initResult); err != nil {
		t.Fatalf("unmarshal init result: %v", err)
	}

	if initResult.ServerInfo.Name != "samverk" {
		t.Errorf("server name = %q, want %q", initResult.ServerInfo.Name, "samverk")
	}
}
