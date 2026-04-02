package mcp_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"samverk.dev/samverk/internal/forge"
	internalmcp "samverk.dev/samverk/internal/mcp"
)

// mockPRManager implements forge.PullRequestManager for testing.
type mockPRManager struct {
	prs           []*forge.PullRequest
	createPRCalls []*forge.CreatePRRequest
	mergePRCalls  []mergePRCall
}

type mergePRCall struct {
	Number    int
	Method    forge.MergeMethod
	CommitMsg string
}

func (m *mockPRManager) CreatePullRequest(_ context.Context, req *forge.CreatePRRequest) (*forge.PullRequest, error) {
	m.createPRCalls = append(m.createPRCalls, req)
	return &forge.PullRequest{
		Number: 42,
		Title:  req.Title,
		Head:   req.Head,
		Base:   req.Base,
		State:  forge.StateOpen,
	}, nil
}

func (m *mockPRManager) GetPullRequest(_ context.Context, number int) (*forge.PullRequest, error) {
	for _, pr := range m.prs {
		if pr.Number == number {
			return pr, nil
		}
	}
	return nil, nil
}

func (m *mockPRManager) ListPullRequests(_ context.Context, _ *forge.ListPROptions) ([]*forge.PullRequest, error) {
	return m.prs, nil
}

func (m *mockPRManager) MergePullRequest(_ context.Context, number int, method forge.MergeMethod, commitMsg string) error {
	m.mergePRCalls = append(m.mergePRCalls, mergePRCall{Number: number, Method: method, CommitMsg: commitMsg})
	return nil
}

func (m *mockPRManager) GetPRChecks(_ context.Context, _ int) ([]forge.Check, error) {
	return nil, nil
}

func (m *mockPRManager) ListReviewComments(_ context.Context, _ int) ([]forge.ReviewComment, error) {
	return nil, nil
}

// newTestMCPServerWithPR sets up an httptest.Server with a PR manager wired.
func newTestMCPServerWithPR(t *testing.T, tracker forge.IssueTracker, prm forge.PullRequestManager) *httptest.Server {
	t.Helper()
	h := internalmcp.NewHandler(tracker, nil, nil, nil, nil)
	h.SetPRManager(prm)
	handler := internalmcp.NewHTTPHandler(h)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

func TestCreatePRTool(t *testing.T) {
	prm := &mockPRManager{}
	ts := newTestMCPServerWithPR(t, &mockTracker{}, prm)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      30,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "create_pr",
			"arguments": map[string]any{
				"title": "Add feature X",
				"body":  "Implements feature X",
				"head":  "feature/x",
				"base":  "main",
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

	var prResult map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &prResult); err != nil {
		t.Fatalf("expected JSON response, got %q: %v", result.Content[0].Text, err)
	}

	if prResult["number"] != float64(42) {
		t.Errorf("number = %v, want 42", prResult["number"])
	}
	if prResult["title"] != "Add feature X" {
		t.Errorf("title = %v, want %q", prResult["title"], "Add feature X")
	}
	if prResult["head"] != "feature/x" {
		t.Errorf("head = %v, want %q", prResult["head"], "feature/x")
	}

	if len(prm.createPRCalls) != 1 {
		t.Fatalf("expected 1 CreatePullRequest call, got %d", len(prm.createPRCalls))
	}
}

func TestListPRsTool(t *testing.T) {
	prm := &mockPRManager{
		prs: []*forge.PullRequest{
			{Number: 1, Title: "PR one", State: forge.StateOpen},
			{Number: 2, Title: "PR two", State: forge.StateOpen},
		},
	}
	ts := newTestMCPServerWithPR(t, &mockTracker{}, prm)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      31,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "list_prs",
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

	var prs []map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &prs); err != nil {
		t.Fatalf("expected JSON array, got %q: %v", result.Content[0].Text, err)
	}

	if len(prs) != 2 {
		t.Errorf("len(prs) = %d, want 2", len(prs))
	}
}

func TestGetPRTool(t *testing.T) {
	prm := &mockPRManager{
		prs: []*forge.PullRequest{
			{Number: 10, Title: "Existing PR", State: forge.StateOpen, Mergeable: true},
		},
	}
	ts := newTestMCPServerWithPR(t, &mockTracker{}, prm)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      32,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "get_pr",
			"arguments": map[string]any{"number": 10},
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

	var prResult map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &prResult); err != nil {
		t.Fatalf("expected JSON response: %v", err)
	}

	if prResult["title"] != "Existing PR" {
		t.Errorf("title = %v, want %q", prResult["title"], "Existing PR")
	}
}

func TestMergePRTool(t *testing.T) {
	prm := &mockPRManager{}
	ts := newTestMCPServerWithPR(t, &mockTracker{}, prm)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      33,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "merge_pr",
			"arguments": map[string]any{
				"number":         10,
				"method":         "squash",
				"commit_message": "Merge PR #10",
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

	if !strings.Contains(result.Content[0].Text, "Merged pull request #10") {
		t.Errorf("expected merge confirmation, got %q", result.Content[0].Text)
	}

	if len(prm.mergePRCalls) != 1 {
		t.Fatalf("expected 1 MergePullRequest call, got %d", len(prm.mergePRCalls))
	}
	if prm.mergePRCalls[0].Method != forge.MergeMethodSquash {
		t.Errorf("method = %q, want %q", prm.mergePRCalls[0].Method, forge.MergeMethodSquash)
	}
}

func TestToolsListIncludesPRTools(t *testing.T) {
	prm := &mockPRManager{}
	ts := newTestMCPServerWithPR(t, &mockTracker{}, prm)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      34,
		Method:  "tools/list",
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}

	var result toolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	wantTools := map[string]bool{
		"create_pr": false,
		"merge_pr":  false,
		"list_prs":  false,
		"get_pr":    false,
	}

	for _, tool := range result.Tools {
		if _, ok := wantTools[tool.Name]; ok {
			wantTools[tool.Name] = true
		}
	}

	for name, found := range wantTools {
		if !found {
			t.Errorf("PR tool %q not found in tools/list response", name)
		}
	}
}
