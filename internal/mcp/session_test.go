package mcp_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/herbhall/samverk/internal/digest"
	internalmcp "github.com/herbhall/samverk/internal/mcp"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
)

// newTestMCPServerWithStore sets up an httptest.Server with a real SQLite store.
func newTestMCPServerWithStore(t *testing.T, tracker *mockTracker, costs digest.CostSource, s store.Store) *httptest.Server {
	t.Helper()
	h := internalmcp.NewHandler(tracker, costs, s, nil, nil)
	handler := internalmcp.NewHTTPHandler(h)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

func TestSessionRecordingNilStore(t *testing.T) {
	// NewHandler with nil store should not panic when tools are called.
	tracker := &mockTracker{}
	ts := newTestMCPServerWithStore(t, tracker, nil, nil)

	// Call add_label -- should succeed without recording (no panic).
	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      100,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "add_label",
			"arguments": map[string]any{
				"issue_number": 1,
				"label":        "test",
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
	if !strings.Contains(result.Content[0].Text, "test") {
		t.Errorf("response missing label name, got %q", result.Content[0].Text)
	}
}

func TestSessionRecordingWithStore(t *testing.T) {
	// Create in-memory SQLite store.
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	tracker := &mockTracker{}
	costs := digest.NewStoreCostSource(s, 0)
	ts := newTestMCPServerWithStore(t, tracker, costs, s)

	// Call add_label -- should record a session and cost entry.
	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      101,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "add_label",
			"arguments": map[string]any{
				"issue_number": 42,
				"label":        "enhancement",
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

	// Verify a session was created.
	sessions, err := s.ListSessions(context.Background(), "")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	sess := sessions[0]
	if sess.AgentType != "mcp-tool" {
		t.Errorf("agent_type = %q, want %q", sess.AgentType, "mcp-tool")
	}
	if sess.Provider != "mcp" {
		t.Errorf("provider = %q, want %q", sess.Provider, "mcp")
	}
	if sess.Model != "add_label" {
		t.Errorf("model = %q, want %q", sess.Model, "add_label")
	}
	if sess.IssueNumber != 42 {
		t.Errorf("issue_number = %d, want %d", sess.IssueNumber, 42)
	}
	if sess.Status != models.SessionStatusCompleted {
		t.Errorf("status = %q, want %q", sess.Status, models.SessionStatusCompleted)
	}
	if sess.FinishedAt == nil {
		t.Error("finished_at should not be nil")
	}
}

func TestSessionRecordingCreateIssue(t *testing.T) {
	// Verify create_issue uses the returned issue number for recording.
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	tracker := &mockTracker{}
	ts := newTestMCPServerWithStore(t, tracker, nil, s)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      102,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "create_issue",
			"arguments": map[string]any{
				"title": "test: session recording issue",
				"body":  "Test body",
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

	// Verify the session was recorded with issue number 99 (from mock).
	sessions, err := s.ListSessions(context.Background(), "")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].IssueNumber != 99 {
		t.Errorf("issue_number = %d, want 99 (returned by mock)", sessions[0].IssueNumber)
	}
	if sessions[0].Model != "create_issue" {
		t.Errorf("model = %q, want %q", sessions[0].Model, "create_issue")
	}
}

func TestSessionRecordingMultipleTools(t *testing.T) {
	// Verify multiple tool calls each create their own session.
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	tracker := &mockTracker{}
	ts := newTestMCPServerWithStore(t, tracker, nil, s)

	tools := []struct {
		name string
		args map[string]any
	}{
		{"add_label", map[string]any{"issue_number": 1, "label": "bug"}},
		{"remove_label", map[string]any{"issue_number": 2, "label": "wontfix"}},
		{"add_comment", map[string]any{"issue_number": 3, "body": "hello"}},
	}

	for _, tool := range tools {
		respBody := postJSON(t, ts.URL, jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      200,
			Method:  "tools/call",
			Params: map[string]any{
				"name":      tool.name,
				"arguments": tool.args,
			},
		})

		var resp jsonRPCResponse
		if err := json.Unmarshal(respBody, &resp); err != nil {
			t.Fatalf("unmarshal response for %s: %v\nbody: %s", tool.name, err, respBody)
		}
		if resp.Error != nil {
			t.Fatalf("unexpected error for %s: %s", tool.name, resp.Error)
		}
	}

	// Verify 3 sessions were created.
	sessions, err := s.ListSessions(context.Background(), "")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}

	// Verify each session has the correct tool name.
	toolNames := make(map[string]bool, len(sessions))
	for _, sess := range sessions {
		toolNames[sess.Model] = true
	}
	for _, tool := range tools {
		if !toolNames[tool.name] {
			t.Errorf("no session found for tool %q", tool.name)
		}
	}
}
