package mcp_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/forge"
)

func TestReviewPRTool(t *testing.T) {
	prm := &mockPRManager{
		prs: []*forge.PullRequest{
			{
				Number:    5,
				Title:     "docs: update readme",
				Author:    "bot",
				Head:      "docs/readme",
				Base:      "main",
				Mergeable: true,
				Labels:    []string{"agent:docs"},
				CreatedAt: time.Now().Add(-2 * time.Hour),
			},
		},
	}
	ts := newTestMCPServerWithPR(t, &mockTracker{}, prm)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      40,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "review_pr",
			"arguments": map[string]any{"number": 5},
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

	var review map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &review); err != nil {
		t.Fatalf("expected JSON response: %v", err)
	}

	if review["number"] != float64(5) {
		t.Errorf("number = %v, want 5", review["number"])
	}
	if review["tier"] != "tier-1" {
		t.Errorf("tier = %v, want tier-1", review["tier"])
	}
	if review["ci_status"] != "no-ci" {
		t.Errorf("ci_status = %v, want no-ci", review["ci_status"])
	}
}

func TestListOpenPRsTool(t *testing.T) {
	prm := &mockPRManager{
		prs: []*forge.PullRequest{
			{Number: 1, Title: "feat: new feature", Author: "dev", CreatedAt: time.Now().Add(-24 * time.Hour)},
			{Number: 2, Title: "docs: update guide", Author: "bot", CreatedAt: time.Now().Add(-1 * time.Hour)},
		},
	}
	ts := newTestMCPServerWithPR(t, &mockTracker{}, prm)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      41,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "list_open_prs",
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

	var prs []map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &prs); err != nil {
		t.Fatalf("expected JSON array: %v", err)
	}

	if len(prs) != 2 {
		t.Errorf("len(prs) = %d, want 2", len(prs))
	}

	// Each entry should have tier and ci_status fields.
	for _, pr := range prs {
		if _, ok := pr["tier"]; !ok {
			t.Error("missing tier field in PR entry")
		}
		if _, ok := pr["ci_status"]; !ok {
			t.Error("missing ci_status field in PR entry")
		}
	}
}

func TestBulkMergeDryRun(t *testing.T) {
	prm := &mockPRManager{
		prs: []*forge.PullRequest{
			{Number: 1, Title: "docs: readme update", Author: "bot", Mergeable: true, CreatedAt: time.Now()},
			{Number: 2, Title: "feat: big feature", Author: "dev", Mergeable: true, CreatedAt: time.Now()},
		},
	}
	ts := newTestMCPServerWithPR(t, &mockTracker{}, prm)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      42,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "bulk_merge",
			"arguments": map[string]any{"dry_run": true},
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

	var mergeResult map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &mergeResult); err != nil {
		t.Fatalf("expected JSON: %v", err)
	}

	if mergeResult["dry_run"] != true {
		t.Errorf("dry_run = %v, want true", mergeResult["dry_run"])
	}

	// No actual merges should have happened.
	if len(prm.mergePRCalls) != 0 {
		t.Errorf("expected 0 merge calls in dry run, got %d", len(prm.mergePRCalls))
	}
}

func TestToolsListIncludesPRReviewTools(t *testing.T) {
	prm := &mockPRManager{}
	ts := newTestMCPServerWithPR(t, &mockTracker{}, prm)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      43,
		Method:  "tools/list",
	})

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	var result toolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	wantTools := map[string]bool{
		"review_pr":    false,
		"list_open_prs": false,
		"bulk_merge":   false,
	}

	for _, tool := range result.Tools {
		if _, ok := wantTools[tool.Name]; ok {
			wantTools[tool.Name] = true
		}
	}

	for name, found := range wantTools {
		if !found {
			t.Errorf("PR review tool %q not found in tools/list response", name)
		}
	}
}
