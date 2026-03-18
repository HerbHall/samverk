package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/herbhall/samverk/internal/forge"
)

func TestSearchIssuesTool(t *testing.T) {
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 1, Title: "fix login bug", Body: "Users cannot login when password contains special chars", State: forge.StateOpen, Labels: []string{"bug"}},
			{Number: 2, Title: "add search feature", Body: "Implement full-text search for issues", State: forge.StateOpen, Labels: []string{"feature"}},
			{Number: 3, Title: "update docs", Body: "Documentation is outdated", State: forge.StateClosed, Labels: []string{"docs"}},
		},
	}
	ts := newTestMCPServer(t, tracker, nil)

	t.Run("basic search", func(t *testing.T) {
		respBody := postJSON(t, ts.URL, jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/call",
			Params: map[string]any{
				"name": "search_issues",
				"arguments": map[string]any{
					"query": "login",
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
			t.Fatalf("expected 1 match, got %d", len(entries))
		}
		if num, ok := entries[0]["number"].(float64); !ok || int(num) != 1 {
			t.Errorf("expected issue #1, got %v", entries[0]["number"])
		}
		if snippet, ok := entries[0]["snippet"].(string); !ok || snippet == "" {
			t.Error("expected non-empty snippet")
		}
	})

	t.Run("search with state filter", func(t *testing.T) {
		respBody := postJSON(t, ts.URL, jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      2,
			Method:  "tools/call",
			Params: map[string]any{
				"name": "search_issues",
				"arguments": map[string]any{
					"query": "docs",
					"state": "closed",
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

		var entries []map[string]any
		if err := json.Unmarshal([]byte(result.Content[0].Text), &entries); err != nil {
			t.Fatalf("unmarshal entries: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 match, got %d", len(entries))
		}
		if num, ok := entries[0]["number"].(float64); !ok || int(num) != 3 {
			t.Errorf("expected issue #3, got %v", entries[0]["number"])
		}
	})

	t.Run("empty query returns error", func(t *testing.T) {
		respBody := postJSON(t, ts.URL, jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      3,
			Method:  "tools/call",
			Params: map[string]any{
				"name": "search_issues",
				"arguments": map[string]any{
					"query": "  ",
				},
			},
		})

		var resp jsonRPCResponse
		if err := json.Unmarshal(respBody, &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		// The MCP SDK may return the error as a JSON-RPC error OR as a
		// result with isError=true. Check both.
		if resp.Error != nil {
			return // error is in the JSON-RPC error field -- pass
		}
		var result callToolResult
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if !result.IsError {
			t.Error("expected error for empty query")
		}
	})

	t.Run("no matches returns empty array", func(t *testing.T) {
		respBody := postJSON(t, ts.URL, jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      4,
			Method:  "tools/call",
			Params: map[string]any{
				"name": "search_issues",
				"arguments": map[string]any{
					"query": "nonexistent-term-xyz",
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

		var entries []map[string]any
		if err := json.Unmarshal([]byte(result.Content[0].Text), &entries); err != nil {
			t.Fatalf("unmarshal entries: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 matches, got %d", len(entries))
		}
	})
}

func TestSearchPRsTool(t *testing.T) {
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 10, Title: "feat: add search", Body: "Implements search", State: forge.StateOpen, IsPullRequest: true},
			{Number: 11, Title: "fix: login timeout", Body: "Fixes timeout in login", State: forge.StateClosed, IsPullRequest: true},
			{Number: 12, Title: "regular issue about search", Body: "Not a PR", State: forge.StateOpen, IsPullRequest: false},
		},
	}
	ts := newTestMCPServer(t, tracker, nil)

	t.Run("search PRs only", func(t *testing.T) {
		respBody := postJSON(t, ts.URL, jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/call",
			Params: map[string]any{
				"name": "search_prs",
				"arguments": map[string]any{
					"query": "search",
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

		var entries []map[string]any
		if err := json.Unmarshal([]byte(result.Content[0].Text), &entries); err != nil {
			t.Fatalf("unmarshal entries: %v", err)
		}
		// Should find PR #10 only (not issue #12, not closed PR #11 -- default state is open)
		if len(entries) != 1 {
			t.Fatalf("expected 1 match, got %d", len(entries))
		}
		if num, ok := entries[0]["number"].(float64); !ok || int(num) != 10 {
			t.Errorf("expected PR #10, got %v", entries[0]["number"])
		}
	})

	t.Run("search PRs all states", func(t *testing.T) {
		respBody := postJSON(t, ts.URL, jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      2,
			Method:  "tools/call",
			Params: map[string]any{
				"name": "search_prs",
				"arguments": map[string]any{
					"query": "search",
					"state": "all",
				},
			},
		})

		var resp jsonRPCResponse
		if err := json.Unmarshal(respBody, &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}

		var result callToolResult
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}

		var entries []map[string]any
		if err := json.Unmarshal([]byte(result.Content[0].Text), &entries); err != nil {
			t.Fatalf("unmarshal entries: %v", err)
		}
		// Should find PR #10 only (issue #12 is not a PR)
		if len(entries) != 1 {
			t.Fatalf("expected 1 match, got %d", len(entries))
		}
	})
}
