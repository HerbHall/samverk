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
	issues   []*forge.Issue
	comments []*forge.Comment

	// Capture calls for verification.
	addLabelCalls    []labelCall
	removeLabelCalls []labelCall
	addCommentCalls  []commentCall
	createIssueCalls []*forge.CreateIssueRequest
	updateIssueCalls []updateIssueCall
	setLabelsCalls   []setLabelsCall
}

type updateIssueCall struct {
	Number int
	Req    *forge.UpdateIssueRequest
}

type setLabelsCall struct {
	Number int
	Labels []string
}

type labelCall struct {
	Number int
	Label  string
}

type commentCall struct {
	Number int
	Body   string
}

func (m *mockTracker) CreateIssue(_ context.Context, req *forge.CreateIssueRequest) (*forge.Issue, error) {
	m.createIssueCalls = append(m.createIssueCalls, req)
	return &forge.Issue{
		Number: 99,
		Title:  req.Title,
	}, nil
}

func (m *mockTracker) GetIssue(_ context.Context, number int) (*forge.Issue, error) {
	for _, iss := range m.issues {
		if iss.Number == number {
			return iss, nil
		}
	}
	return nil, nil
}

func (m *mockTracker) UpdateIssue(_ context.Context, number int, req *forge.UpdateIssueRequest) (*forge.Issue, error) {
	m.updateIssueCalls = append(m.updateIssueCalls, updateIssueCall{Number: number, Req: req})
	// Find and return a copy with updates applied.
	for _, iss := range m.issues {
		if iss.Number != number {
			continue
		}
		updated := *iss
		if req.Title != nil {
			updated.Title = *req.Title
		}
		if req.Body != nil {
			updated.Body = *req.Body
		}
		if req.State != nil {
			updated.State = *req.State
		}
		return &updated, nil
	}
	return &forge.Issue{Number: number}, nil
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

func (m *mockTracker) AddComment(_ context.Context, number int, body string) (*forge.Comment, error) {
	m.addCommentCalls = append(m.addCommentCalls, commentCall{Number: number, Body: body})
	return &forge.Comment{
		ID:     42,
		Body:   body,
		Author: "test",
	}, nil
}

func (m *mockTracker) ListComments(_ context.Context, _ int) ([]*forge.Comment, error) {
	return m.comments, nil
}

func (m *mockTracker) SetLabels(_ context.Context, number int, labels []string) error {
	m.setLabelsCalls = append(m.setLabelsCalls, setLabelsCall{Number: number, Labels: labels})
	return nil
}

func (m *mockTracker) AddLabel(_ context.Context, number int, label string) error {
	m.addLabelCalls = append(m.addLabelCalls, labelCall{Number: number, Label: label})
	return nil
}

func (m *mockTracker) RemoveLabel(_ context.Context, number int, label string) error {
	m.removeLabelCalls = append(m.removeLabelCalls, labelCall{Number: number, Label: label})
	return nil
}

func (m *mockTracker) Assign(_ context.Context, _ int, _ string) error    { return nil }
func (m *mockTracker) Unassign(_ context.Context, _ int, _ string) error  { return nil }
func (m *mockTracker) Watch(_ context.Context, _ func(forge.Event)) error { return nil }

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
	h := internalmcp.NewHandler(tracker, costs, nil)
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

	// Verify all tools are registered.
	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	expectedTools := []string{
		"get_digest", "get_cost_summary",
		"add_label", "remove_label", "add_comment", "create_issue",
		"list_issues", "get_issue", "update_issue",
		"close_issue", "reopen_issue", "set_labels", "list_comments",
	}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("%s tool not found in tools/list", name)
		}
	}
	if len(result.Tools) != 13 {
		t.Errorf("expected 13 tools, got %d", len(result.Tools))
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

func TestAddLabelTool(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      10,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "add_label",
			"arguments": map[string]any{
				"issue_number": 42,
				"label":        "bug",
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
	if !strings.Contains(text, "bug") {
		t.Errorf("response missing label name, got %q", text)
	}
	if !strings.Contains(text, "#42") {
		t.Errorf("response missing issue number, got %q", text)
	}

	if len(tracker.addLabelCalls) != 1 {
		t.Fatalf("expected 1 AddLabel call, got %d", len(tracker.addLabelCalls))
	}
	if tracker.addLabelCalls[0].Number != 42 || tracker.addLabelCalls[0].Label != "bug" {
		t.Errorf("AddLabel called with %+v, want {42, bug}", tracker.addLabelCalls[0])
	}
}

func TestAddLabelToolValidation(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	tests := []struct {
		name string
		args map[string]any
	}{
		{"zero issue number", map[string]any{"issue_number": 0, "label": "bug"}},
		{"negative issue number", map[string]any{"issue_number": -1, "label": "bug"}},
		{"empty label", map[string]any{"issue_number": 1, "label": ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBody := postJSON(t, ts.URL, jsonRPCRequest{
				JSONRPC: "2.0",
				ID:      11,
				Method:  "tools/call",
				Params: map[string]any{
					"name":      "add_label",
					"arguments": tt.args,
				},
			})

			var resp jsonRPCResponse
			if err := json.Unmarshal(respBody, &resp); err != nil {
				t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
			}

			// Either protocol-level error or isError=true in result.
			if resp.Error != nil {
				return
			}

			var result callToolResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if !result.IsError {
				t.Error("expected isError=true for invalid input")
			}
		})
	}
}

func TestRemoveLabelTool(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      12,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "remove_label",
			"arguments": map[string]any{
				"issue_number": 7,
				"label":        "wontfix",
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
	if !strings.Contains(text, "wontfix") {
		t.Errorf("response missing label name, got %q", text)
	}
	if !strings.Contains(text, "#7") {
		t.Errorf("response missing issue number, got %q", text)
	}

	if len(tracker.removeLabelCalls) != 1 {
		t.Fatalf("expected 1 RemoveLabel call, got %d", len(tracker.removeLabelCalls))
	}
	if tracker.removeLabelCalls[0].Number != 7 || tracker.removeLabelCalls[0].Label != "wontfix" {
		t.Errorf("RemoveLabel called with %+v, want {7, wontfix}", tracker.removeLabelCalls[0])
	}
}

func TestRemoveLabelToolValidation(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	tests := []struct {
		name string
		args map[string]any
	}{
		{"zero issue number", map[string]any{"issue_number": 0, "label": "bug"}},
		{"empty label", map[string]any{"issue_number": 1, "label": ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBody := postJSON(t, ts.URL, jsonRPCRequest{
				JSONRPC: "2.0",
				ID:      13,
				Method:  "tools/call",
				Params: map[string]any{
					"name":      "remove_label",
					"arguments": tt.args,
				},
			})

			var resp jsonRPCResponse
			if err := json.Unmarshal(respBody, &resp); err != nil {
				t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
			}

			if resp.Error != nil {
				return
			}

			var result callToolResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if !result.IsError {
				t.Error("expected isError=true for invalid input")
			}
		})
	}
}

func TestAddCommentTool(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      14,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "add_comment",
			"arguments": map[string]any{
				"issue_number": 5,
				"body":         "This looks good to merge.",
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

	// Response should be JSON with comment_id and issue_number.
	var commentResult map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &commentResult); err != nil {
		t.Fatalf("expected JSON response, got %q: %v", result.Content[0].Text, err)
	}

	if commentResult["comment_id"] != float64(42) {
		t.Errorf("comment_id = %v, want 42", commentResult["comment_id"])
	}
	if commentResult["issue_number"] != float64(5) {
		t.Errorf("issue_number = %v, want 5", commentResult["issue_number"])
	}

	if len(tracker.addCommentCalls) != 1 {
		t.Fatalf("expected 1 AddComment call, got %d", len(tracker.addCommentCalls))
	}
	if tracker.addCommentCalls[0].Number != 5 {
		t.Errorf("AddComment issue number = %d, want 5", tracker.addCommentCalls[0].Number)
	}
	if tracker.addCommentCalls[0].Body != "This looks good to merge." {
		t.Errorf("AddComment body = %q, want %q", tracker.addCommentCalls[0].Body, "This looks good to merge.")
	}
}

func TestAddCommentToolValidation(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	tests := []struct {
		name string
		args map[string]any
	}{
		{"zero issue number", map[string]any{"issue_number": 0, "body": "hello"}},
		{"empty body", map[string]any{"issue_number": 1, "body": ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBody := postJSON(t, ts.URL, jsonRPCRequest{
				JSONRPC: "2.0",
				ID:      15,
				Method:  "tools/call",
				Params: map[string]any{
					"name":      "add_comment",
					"arguments": tt.args,
				},
			})

			var resp jsonRPCResponse
			if err := json.Unmarshal(respBody, &resp); err != nil {
				t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
			}

			if resp.Error != nil {
				return
			}

			var result callToolResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if !result.IsError {
				t.Error("expected isError=true for invalid input")
			}
		})
	}
}

func TestCreateIssueTool(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      16,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "create_issue",
			"arguments": map[string]any{
				"title":     "Fix login bug",
				"body":      "Users cannot log in when...",
				"labels":    []string{"bug", "priority:high"},
				"assignees": []string{"alice"},
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

	// Response should be JSON with issue_number and title.
	var issueResult map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &issueResult); err != nil {
		t.Fatalf("expected JSON response, got %q: %v", result.Content[0].Text, err)
	}

	if issueResult["issue_number"] != float64(99) {
		t.Errorf("issue_number = %v, want 99", issueResult["issue_number"])
	}
	if issueResult["title"] != "Fix login bug" {
		t.Errorf("title = %v, want %q", issueResult["title"], "Fix login bug")
	}

	if len(tracker.createIssueCalls) != 1 {
		t.Fatalf("expected 1 CreateIssue call, got %d", len(tracker.createIssueCalls))
	}
	req := tracker.createIssueCalls[0]
	if req.Title != "Fix login bug" {
		t.Errorf("CreateIssue title = %q, want %q", req.Title, "Fix login bug")
	}
	if req.Body != "Users cannot log in when..." {
		t.Errorf("CreateIssue body = %q, want %q", req.Body, "Users cannot log in when...")
	}
	if len(req.Labels) != 2 {
		t.Errorf("CreateIssue labels count = %d, want 2", len(req.Labels))
	}
	if len(req.Assignees) != 1 || req.Assignees[0] != "alice" {
		t.Errorf("CreateIssue assignees = %v, want [alice]", req.Assignees)
	}
}

func TestCreateIssueToolMinimal(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      17,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "create_issue",
			"arguments": map[string]any{
				"title": "Simple issue",
				"body":  "Just a body.",
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

	// Verify mock was called without labels/assignees.
	if len(tracker.createIssueCalls) != 1 {
		t.Fatalf("expected 1 CreateIssue call, got %d", len(tracker.createIssueCalls))
	}
	req := tracker.createIssueCalls[0]
	if len(req.Labels) != 0 {
		t.Errorf("CreateIssue labels = %v, want empty", req.Labels)
	}
	if len(req.Assignees) != 0 {
		t.Errorf("CreateIssue assignees = %v, want empty", req.Assignees)
	}
}

func TestCreateIssueToolValidation(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      18,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "create_issue",
			"arguments": map[string]any{
				"title": "",
				"body":  "Some body",
			},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}

	if resp.Error != nil {
		return
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !result.IsError {
		t.Error("expected isError=true for empty title")
	}
}

func TestListIssuesTool(t *testing.T) {
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 1, Title: "Open bug", State: forge.StateOpen, Labels: []string{"bug"}},
			{Number: 2, Title: "Closed task", State: forge.StateClosed, Labels: []string{"task"}},
			{Number: 3, Title: "Open feature", State: forge.StateOpen, Labels: []string{"feature"}},
		},
	}
	ts := newTestMCPServer(t, tracker, nil)

	tests := []struct {
		name      string
		args      map[string]any
		wantCount int
	}{
		{"no filters", map[string]any{}, 3},
		{"filter by open state", map[string]any{"state": "open"}, 2},
		{"filter by closed state", map[string]any{"state": "closed"}, 1},
		{"filter by label", map[string]any{"labels": []string{"bug"}}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBody := postJSON(t, ts.URL, jsonRPCRequest{
				JSONRPC: "2.0",
				ID:      20,
				Method:  "tools/call",
				Params: map[string]any{
					"name":      "list_issues",
					"arguments": tt.args,
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

			var issues []json.RawMessage
			if err := json.Unmarshal([]byte(result.Content[0].Text), &issues); err != nil {
				t.Fatalf("expected JSON array, got %q: %v", result.Content[0].Text, err)
			}
			if len(issues) != tt.wantCount {
				t.Errorf("got %d issues, want %d", len(issues), tt.wantCount)
			}
		})
	}
}

func TestGetIssueTool(t *testing.T) {
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 42, Title: "The answer", State: forge.StateOpen, Body: "Deep thought."},
		},
	}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      21,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "get_issue",
			"arguments": map[string]any{"issue_number": 42},
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

	var issueResult map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &issueResult); err != nil {
		t.Fatalf("expected JSON response, got %q: %v", result.Content[0].Text, err)
	}

	if issueResult["Number"] != float64(42) {
		t.Errorf("Number = %v, want 42", issueResult["Number"])
	}
	if issueResult["Title"] != "The answer" {
		t.Errorf("Title = %v, want %q", issueResult["Title"], "The answer")
	}
}

func TestGetIssueToolValidation(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	tests := []struct {
		name string
		args map[string]any
	}{
		{"zero issue number", map[string]any{"issue_number": 0}},
		{"negative issue number", map[string]any{"issue_number": -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBody := postJSON(t, ts.URL, jsonRPCRequest{
				JSONRPC: "2.0",
				ID:      22,
				Method:  "tools/call",
				Params: map[string]any{
					"name":      "get_issue",
					"arguments": tt.args,
				},
			})

			var resp jsonRPCResponse
			if err := json.Unmarshal(respBody, &resp); err != nil {
				t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
			}

			if resp.Error != nil {
				return
			}

			var result callToolResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if !result.IsError {
				t.Error("expected isError=true for invalid input")
			}
		})
	}
}

func TestUpdateIssueTool(t *testing.T) {
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 10, Title: "Old title", Body: "Old body", State: forge.StateOpen},
		},
	}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      23,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "update_issue",
			"arguments": map[string]any{
				"issue_number": 10,
				"title":        "New title",
				"state":        "closed",
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

	var issueResult map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &issueResult); err != nil {
		t.Fatalf("expected JSON response, got %q: %v", result.Content[0].Text, err)
	}

	if issueResult["Title"] != "New title" {
		t.Errorf("Title = %v, want %q", issueResult["Title"], "New title")
	}

	if len(tracker.updateIssueCalls) != 1 {
		t.Fatalf("expected 1 UpdateIssue call, got %d", len(tracker.updateIssueCalls))
	}
	call := tracker.updateIssueCalls[0]
	if call.Number != 10 {
		t.Errorf("UpdateIssue number = %d, want 10", call.Number)
	}
	if call.Req.Title == nil || *call.Req.Title != "New title" {
		t.Errorf("UpdateIssue title = %v, want %q", call.Req.Title, "New title")
	}
	if call.Req.State == nil || *call.Req.State != forge.StateClosed {
		t.Errorf("UpdateIssue state = %v, want %q", call.Req.State, forge.StateClosed)
	}
}

func TestUpdateIssueToolValidation(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	tests := []struct {
		name string
		args map[string]any
	}{
		{"zero issue number", map[string]any{"issue_number": 0, "title": "x"}},
		{"negative issue number", map[string]any{"issue_number": -1, "title": "x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBody := postJSON(t, ts.URL, jsonRPCRequest{
				JSONRPC: "2.0",
				ID:      24,
				Method:  "tools/call",
				Params: map[string]any{
					"name":      "update_issue",
					"arguments": tt.args,
				},
			})

			var resp jsonRPCResponse
			if err := json.Unmarshal(respBody, &resp); err != nil {
				t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
			}

			if resp.Error != nil {
				return
			}

			var result callToolResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if !result.IsError {
				t.Error("expected isError=true for invalid input")
			}
		})
	}
}

func TestCloseIssueTool(t *testing.T) {
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 5, Title: "Open issue", State: forge.StateOpen},
		},
	}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      25,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "close_issue",
			"arguments": map[string]any{"issue_number": 5},
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
	if !strings.Contains(text, "Closed") {
		t.Errorf("response missing 'Closed', got %q", text)
	}
	if !strings.Contains(text, "#5") {
		t.Errorf("response missing issue number, got %q", text)
	}

	if len(tracker.updateIssueCalls) != 1 {
		t.Fatalf("expected 1 UpdateIssue call, got %d", len(tracker.updateIssueCalls))
	}
	call := tracker.updateIssueCalls[0]
	if call.Number != 5 {
		t.Errorf("UpdateIssue number = %d, want 5", call.Number)
	}
	if call.Req.State == nil || *call.Req.State != forge.StateClosed {
		t.Errorf("UpdateIssue state = %v, want %q", call.Req.State, forge.StateClosed)
	}
}

func TestCloseIssueToolValidation(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      26,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "close_issue",
			"arguments": map[string]any{"issue_number": 0},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}

	if resp.Error != nil {
		return
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !result.IsError {
		t.Error("expected isError=true for invalid input")
	}
}

func TestReopenIssueTool(t *testing.T) {
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 8, Title: "Closed issue", State: forge.StateClosed},
		},
	}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      27,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "reopen_issue",
			"arguments": map[string]any{"issue_number": 8},
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
	if !strings.Contains(text, "Reopened") {
		t.Errorf("response missing 'Reopened', got %q", text)
	}
	if !strings.Contains(text, "#8") {
		t.Errorf("response missing issue number, got %q", text)
	}

	if len(tracker.updateIssueCalls) != 1 {
		t.Fatalf("expected 1 UpdateIssue call, got %d", len(tracker.updateIssueCalls))
	}
	call := tracker.updateIssueCalls[0]
	if call.Number != 8 {
		t.Errorf("UpdateIssue number = %d, want 8", call.Number)
	}
	if call.Req.State == nil || *call.Req.State != forge.StateOpen {
		t.Errorf("UpdateIssue state = %v, want %q", call.Req.State, forge.StateOpen)
	}
}

func TestReopenIssueToolValidation(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      28,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "reopen_issue",
			"arguments": map[string]any{"issue_number": -1},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}

	if resp.Error != nil {
		return
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !result.IsError {
		t.Error("expected isError=true for invalid input")
	}
}

func TestSetLabelsTool(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      29,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "set_labels",
			"arguments": map[string]any{
				"issue_number": 15,
				"labels":       []string{"bug", "priority:high"},
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
	if !strings.Contains(text, "2 label(s)") {
		t.Errorf("response missing label count, got %q", text)
	}
	if !strings.Contains(text, "#15") {
		t.Errorf("response missing issue number, got %q", text)
	}

	if len(tracker.setLabelsCalls) != 1 {
		t.Fatalf("expected 1 SetLabels call, got %d", len(tracker.setLabelsCalls))
	}
	call := tracker.setLabelsCalls[0]
	if call.Number != 15 {
		t.Errorf("SetLabels number = %d, want 15", call.Number)
	}
	if len(call.Labels) != 2 {
		t.Errorf("SetLabels labels count = %d, want 2", len(call.Labels))
	}
}

func TestSetLabelsToolValidation(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	tests := []struct {
		name string
		args map[string]any
	}{
		{"zero issue number", map[string]any{"issue_number": 0, "labels": []string{"bug"}}},
		{"empty labels", map[string]any{"issue_number": 1, "labels": []string{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBody := postJSON(t, ts.URL, jsonRPCRequest{
				JSONRPC: "2.0",
				ID:      30,
				Method:  "tools/call",
				Params: map[string]any{
					"name":      "set_labels",
					"arguments": tt.args,
				},
			})

			var resp jsonRPCResponse
			if err := json.Unmarshal(respBody, &resp); err != nil {
				t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
			}

			if resp.Error != nil {
				return
			}

			var result callToolResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if !result.IsError {
				t.Error("expected isError=true for invalid input")
			}
		})
	}
}

func TestListCommentsTool(t *testing.T) {
	tracker := &mockTracker{
		comments: []*forge.Comment{
			{ID: 1, Body: "First comment", Author: "alice", CreatedAt: time.Now()},
			{ID: 2, Body: "Second comment", Author: "bob", CreatedAt: time.Now()},
		},
	}
	ts := newTestMCPServer(t, tracker, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      31,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "list_comments",
			"arguments": map[string]any{"issue_number": 7},
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

	var comments []json.RawMessage
	if err := json.Unmarshal([]byte(result.Content[0].Text), &comments); err != nil {
		t.Fatalf("expected JSON array, got %q: %v", result.Content[0].Text, err)
	}
	if len(comments) != 2 {
		t.Errorf("got %d comments, want 2", len(comments))
	}
}

func TestListCommentsToolValidation(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestMCPServer(t, tracker, nil)

	tests := []struct {
		name string
		args map[string]any
	}{
		{"zero issue number", map[string]any{"issue_number": 0}},
		{"negative issue number", map[string]any{"issue_number": -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBody := postJSON(t, ts.URL, jsonRPCRequest{
				JSONRPC: "2.0",
				ID:      32,
				Method:  "tools/call",
				Params: map[string]any{
					"name":      "list_comments",
					"arguments": tt.args,
				},
			})

			var resp jsonRPCResponse
			if err := json.Unmarshal(respBody, &resp); err != nil {
				t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
			}

			if resp.Error != nil {
				return
			}

			var result callToolResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if !result.IsError {
				t.Error("expected isError=true for invalid input")
			}
		})
	}
}
