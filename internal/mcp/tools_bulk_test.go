package mcp_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/herbhall/samverk/internal/forge"
	internalmcp "github.com/herbhall/samverk/internal/mcp"
)

func newTestBulkServer(t *testing.T, tracker forge.IssueTracker) *httptest.Server {
	t.Helper()
	h := internalmcp.NewHandler(tracker, nil, nil, nil, nil)
	ts := httptest.NewServer(internalmcp.NewHTTPHandler(h))
	t.Cleanup(ts.Close)
	return ts
}

func TestBulkUpdateIssues(t *testing.T) {
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 1, Title: "Issue 1", State: forge.StateOpen, Labels: []string{"bug"}},
			{Number: 2, Title: "Issue 2", State: forge.StateOpen},
		},
	}
	ts := newTestBulkServer(t, tracker)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "bulk_update_issues",
			"arguments": {
				"updates": [
					{"issue_number": 1, "labels": ["fix", "priority:high"]},
					{"issue_number": 2, "state": "closed"}
				]
			}
		}`),
	})

	var rpcResp struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	text := rpcResp.Result.Content[0].Text
	var results []map[string]any
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("parse results: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r["status"] != "ok" {
			t.Errorf("expected ok for issue %v, got %v (error: %v)", r["issue_number"], r["status"], r["error"])
		}
	}

	if len(tracker.setLabelsCalls) != 1 {
		t.Fatalf("expected 1 setLabels call, got %d", len(tracker.setLabelsCalls))
	}
	if tracker.setLabelsCalls[0].Number != 1 {
		t.Errorf("expected setLabels on issue 1, got %d", tracker.setLabelsCalls[0].Number)
	}

	if len(tracker.updateIssueCalls) != 1 {
		t.Fatalf("expected 1 updateIssue call, got %d", len(tracker.updateIssueCalls))
	}
	if tracker.updateIssueCalls[0].Number != 2 {
		t.Errorf("expected updateIssue on issue 2, got %d", tracker.updateIssueCalls[0].Number)
	}
}

func TestBulkUpdateIssues_EmptyUpdates(t *testing.T) {
	ts := newTestBulkServer(t, &mockTracker{})

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "bulk_update_issues", "arguments": {"updates": []}}`),
	})

	if !strings.Contains(string(respBody), "error") && !strings.Contains(string(respBody), "must not be empty") {
		t.Errorf("expected error for empty updates, got: %s", string(respBody))
	}
}

func TestBulkUpdateIssues_InvalidIssueNumber(t *testing.T) {
	ts := newTestBulkServer(t, &mockTracker{})

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "bulk_update_issues",
			"arguments": {
				"updates": [{"issue_number": 0, "labels": ["bug"]}]
			}
		}`),
	})

	var rpcResp struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	text := rpcResp.Result.Content[0].Text
	var results []map[string]any
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("parse results: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["status"] != "error" {
		t.Errorf("expected error status, got %v", results[0]["status"])
	}
}

func TestCloseIssueWithComment(t *testing.T) {
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 5, Title: "Bug", State: forge.StateOpen},
		},
	}
	ts := newTestBulkServer(t, tracker)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "close_issue_with_comment",
			"arguments": {"issue_number": 5, "comment": "Closing as resolved"}
		}`),
	})

	var rpcResp struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	text := rpcResp.Result.Content[0].Text
	if text != "Added comment and closed issue #5" {
		t.Errorf("unexpected result: %s", text)
	}

	if len(tracker.addCommentCalls) != 1 {
		t.Fatalf("expected 1 addComment call, got %d", len(tracker.addCommentCalls))
	}
	if tracker.addCommentCalls[0].Number != 5 {
		t.Errorf("expected comment on issue 5, got %d", tracker.addCommentCalls[0].Number)
	}
	if tracker.addCommentCalls[0].Body != "Closing as resolved" {
		t.Errorf("unexpected comment body: %s", tracker.addCommentCalls[0].Body)
	}

	if len(tracker.updateIssueCalls) != 1 {
		t.Fatalf("expected 1 updateIssue call, got %d", len(tracker.updateIssueCalls))
	}
	if tracker.updateIssueCalls[0].Req.State == nil || *tracker.updateIssueCalls[0].Req.State != forge.StateClosed {
		t.Error("expected issue to be closed")
	}
}

func TestCloseIssueWithComment_EmptyComment(t *testing.T) {
	ts := newTestBulkServer(t, &mockTracker{})

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "close_issue_with_comment", "arguments": {"issue_number": 1, "comment": ""}}`),
	})

	if !strings.Contains(string(respBody), "error") && !strings.Contains(string(respBody), "must not be empty") {
		t.Errorf("expected error for empty comment, got: %s", string(respBody))
	}
}

func TestCloseIssueWithComment_InvalidIssueNumber(t *testing.T) {
	ts := newTestBulkServer(t, &mockTracker{})

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "close_issue_with_comment", "arguments": {"issue_number": 0, "comment": "test"}}`),
	})

	if !strings.Contains(string(respBody), "error") && !strings.Contains(string(respBody), "must be greater than 0") {
		t.Errorf("expected error for invalid issue number, got: %s", string(respBody))
	}
}

func TestCreateIssueBatch(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestBulkServer(t, tracker)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "create_issue_batch",
			"arguments": {
				"issues": [
					{"title": "feat: task 1 implementation", "body": "Do thing 1", "labels": ["task"]},
					{"title": "feat: task 2 implementation", "body": "Do thing 2"}
				]
			}
		}`),
	})

	var rpcResp struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	text := rpcResp.Result.Content[0].Text
	var results []map[string]any
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("parse results: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r["status"] != "created" {
			t.Errorf("expected created, got %v", r["status"])
		}
	}

	if len(tracker.createIssueCalls) != 2 {
		t.Fatalf("expected 2 createIssue calls, got %d", len(tracker.createIssueCalls))
	}
}

func TestCreateIssueBatch_EmptyTitle(t *testing.T) {
	tracker := &mockTracker{}
	ts := newTestBulkServer(t, tracker)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "create_issue_batch",
			"arguments": {
				"issues": [
					{"title": "", "body": "missing title"},
					{"title": "chore: valid test issue", "body": "has title"}
				]
			}
		}`),
	})

	var rpcResp struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	text := rpcResp.Result.Content[0].Text
	var results []map[string]any
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("parse results: %v", err)
	}

	if results[0]["status"] != "error" {
		t.Errorf("expected error for empty title, got %v", results[0]["status"])
	}
	if results[1]["status"] != "created" {
		t.Errorf("expected created for valid issue, got %v", results[1]["status"])
	}
}

func TestGetProjectSummary(t *testing.T) {
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 1, Title: "Open 1", State: forge.StateOpen, Labels: []string{"bug", "priority:high"}},
			{Number: 2, Title: "Open 2", State: forge.StateOpen, Labels: []string{"bug"}},
			{Number: 3, Title: "Closed 1", State: forge.StateClosed},
		},
	}
	ts := newTestBulkServer(t, tracker)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "get_project_summary", "arguments": {}}`),
	})

	var rpcResp struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	text := rpcResp.Result.Content[0].Text
	var summary map[string]any
	if err := json.Unmarshal([]byte(text), &summary); err != nil {
		t.Fatalf("parse summary: %v", err)
	}

	openCount := int(summary["open_issue_count"].(float64))
	closedCount := int(summary["closed_issue_count"].(float64))

	if openCount != 2 {
		t.Errorf("expected 2 open issues, got %d", openCount)
	}
	if closedCount != 1 {
		t.Errorf("expected 1 closed issue, got %d", closedCount)
	}

	buckets, ok := summary["label_buckets"].([]any)
	if !ok {
		t.Fatal("expected label_buckets array")
	}
	if len(buckets) == 0 {
		t.Error("expected non-empty label_buckets")
	}
}

func TestGetProjectSummary_NilTracker(t *testing.T) {
	h := internalmcp.NewHandler(nil, nil, nil, nil, nil)
	ts := httptest.NewServer(internalmcp.NewHTTPHandler(h))
	t.Cleanup(ts.Close)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "get_project_summary", "arguments": {}}`),
	})

	if !strings.Contains(string(respBody), "error") && !strings.Contains(string(respBody), "no issue tracker") {
		t.Errorf("expected error when tracker is nil, got: %s", string(respBody))
	}
}
