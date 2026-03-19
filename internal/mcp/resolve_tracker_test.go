package mcp_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/herbhall/samverk/internal/forge"
	internalmcp "github.com/herbhall/samverk/internal/mcp"
)

// newTestServerWithRegistry builds an httptest.Server whose handler has two
// registered projects: "alpha" (with alphaTracker) and "beta" (with betaTracker).
// "alpha" is set as the active project.
func newTestServerWithRegistry(t *testing.T, alphaTracker, betaTracker forge.IssueTracker) *httptest.Server {
	t.Helper()

	reg := internalmcp.NewProjectRegistry()
	if err := reg.Register(&internalmcp.Project{
		Name:    "alpha",
		Owner:   "org",
		Repo:    "alpha",
		Tracker: alphaTracker,
	}); err != nil {
		t.Fatalf("register alpha: %v", err)
	}
	if err := reg.Register(&internalmcp.Project{
		Name:    "beta",
		Owner:   "org",
		Repo:    "beta",
		Tracker: betaTracker,
	}); err != nil {
		t.Fatalf("register beta: %v", err)
	}
	if err := reg.SetActive("alpha"); err != nil {
		t.Fatalf("set active: %v", err)
	}

	h := internalmcp.NewHandler(nil, nil, nil, nil, nil)
	h.SetProjects(reg)
	ts := httptest.NewServer(internalmcp.NewHTTPHandler(h))
	t.Cleanup(ts.Close)
	return ts
}

// TestResolveTracker_EmptyName verifies that omitting the project parameter
// targets the active project's tracker (alpha), not the other registered one.
func TestResolveTracker_EmptyName(t *testing.T) {
	alphaTracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 1, Title: "alpha issue", State: forge.StateOpen},
		},
	}
	betaTracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 100, Title: "beta issue", State: forge.StateOpen},
		},
	}
	ts := newTestServerWithRegistry(t, alphaTracker, betaTracker)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "list_issues",
			"arguments": map[string]any{},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}

	var issues []map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &issues); err != nil {
		t.Fatalf("unmarshal issues: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue from active (alpha) project, got %d", len(issues))
	}
	if num, ok := issues[0]["Number"].(float64); !ok || int(num) != 1 {
		t.Errorf("expected alpha issue #1, got %v", issues[0]["Number"])
	}
}

// TestResolveTracker_NamedProject verifies that passing project="beta" routes
// the request to the beta tracker even though alpha is active.
func TestResolveTracker_NamedProject(t *testing.T) {
	alphaTracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 1, Title: "alpha issue", State: forge.StateOpen},
		},
	}
	betaTracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 100, Title: "beta issue", State: forge.StateOpen},
		},
	}
	ts := newTestServerWithRegistry(t, alphaTracker, betaTracker)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "list_issues",
			"arguments": map[string]any{
				"project": "beta",
			},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}

	var issues []map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &issues); err != nil {
		t.Fatalf("unmarshal issues: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue from beta project, got %d", len(issues))
	}
	if num, ok := issues[0]["Number"].(float64); !ok || int(num) != 100 {
		t.Errorf("expected beta issue #100, got %v", issues[0]["Number"])
	}
}

// TestResolveTracker_UnknownProject verifies that an unrecognised project name
// results in an error response rather than silently querying the wrong tracker.
func TestResolveTracker_UnknownProject(t *testing.T) {
	alphaTracker := &mockTracker{}
	betaTracker := &mockTracker{}
	ts := newTestServerWithRegistry(t, alphaTracker, betaTracker)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "list_issues",
			"arguments": map[string]any{
				"project": "nonexistent",
			},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// The MCP SDK may surface the error as a JSON-RPC error or as a result
	// with isError=true. Either form is acceptable.
	if resp.Error != nil {
		return // error in JSON-RPC error field -- pass
	}
	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !result.IsError {
		t.Error("expected error response for unknown project name, got success")
	}
}

// TestSearchIssues_ProjectParam verifies that search_issues honours the project
// parameter and routes to the named project's tracker.
func TestSearchIssues_ProjectParam(t *testing.T) {
	alphaTracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 1, Title: "auth bug in alpha", State: forge.StateOpen},
		},
	}
	betaTracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 200, Title: "auth bug in beta", State: forge.StateOpen},
		},
	}
	ts := newTestServerWithRegistry(t, alphaTracker, betaTracker)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "search_issues",
			"arguments": map[string]any{
				"query":   "auth",
				"project": "beta",
			},
		},
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}

	var entries []map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &entries); err != nil {
		t.Fatalf("unmarshal entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 match from beta project, got %d", len(entries))
	}
	if num, ok := entries[0]["number"].(float64); !ok || int(num) != 200 {
		// search_issues returns searchResultEntry which uses lowercase "number"
		t.Errorf("expected beta issue #200, got %v", entries[0]["number"])
	}
}
