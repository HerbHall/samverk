package mcp_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/forge"
	internalmcp "github.com/herbhall/samverk/internal/mcp"
)

// mockRepoReader implements forge.RepoReader for testing.
type mockRepoReader struct {
	files    []forge.FileEntry
	fileData []byte
	diff     string
	branches []forge.Branch
	commits  []forge.Commit
	search   []forge.SearchResult
}

func (m *mockRepoReader) ListFiles(_ context.Context, _, _ string) ([]forge.FileEntry, error) {
	return m.files, nil
}

func (m *mockRepoReader) ReadFile(_ context.Context, _, _ string) ([]byte, error) {
	return m.fileData, nil
}

func (m *mockRepoReader) GetDiff(_ context.Context, _, _ string) (string, error) {
	return m.diff, nil
}

func (m *mockRepoReader) ListBranches(_ context.Context) ([]forge.Branch, error) {
	return m.branches, nil
}

func (m *mockRepoReader) GetCommitLog(_ context.Context, _ string, _ int) ([]forge.Commit, error) {
	return m.commits, nil
}

func (m *mockRepoReader) SearchCode(_ context.Context, _ string) ([]forge.SearchResult, error) {
	return m.search, nil
}

// newTestMCPServerWithRepo sets up an httptest.Server with a RepoReader.
func newTestMCPServerWithRepo(t *testing.T, tracker *mockTracker, repo forge.RepoReader) *httptest.Server {
	t.Helper()
	h := internalmcp.NewHandler(tracker, nil, nil, nil, repo)
	handler := internalmcp.NewHTTPHandler(h)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

func TestListFiles(t *testing.T) {
	repo := &mockRepoReader{
		files: []forge.FileEntry{
			{Name: "main.go", Path: "cmd/main.go", Type: "file", Size: 1024},
			{Name: "internal", Path: "internal", Type: "dir", Size: 0},
		},
	}
	ts := newTestMCPServerWithRepo(t, &mockTracker{}, repo)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "list_files",
			"arguments": map[string]any{
				"path": "",
				"ref":  "main",
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
	if !strings.Contains(result.Content[0].Text, "main.go") {
		t.Errorf("response missing main.go, got %q", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "internal") {
		t.Errorf("response missing internal dir, got %q", result.Content[0].Text)
	}
}

func TestReadFile(t *testing.T) {
	repo := &mockRepoReader{
		fileData: []byte("package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"),
	}
	ts := newTestMCPServerWithRepo(t, &mockTracker{}, repo)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "read_file",
			"arguments": map[string]any{
				"path": "main.go",
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
	if !strings.Contains(result.Content[0].Text, "package main") {
		t.Errorf("response missing file content, got %q", result.Content[0].Text)
	}
}

func TestReadFile_LineRange(t *testing.T) {
	repo := &mockRepoReader{
		fileData: []byte("line1\nline2\nline3\nline4\nline5\n"),
	}
	ts := newTestMCPServerWithRepo(t, &mockTracker{}, repo)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "read_file",
			"arguments": map[string]any{
				"path":       "test.txt",
				"start_line": 2,
				"end_line":   4,
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
	if !strings.Contains(text, "line2") || !strings.Contains(text, "line4") {
		t.Errorf("expected lines 2-4, got %q", text)
	}
	if strings.Contains(text, "line1") || strings.Contains(text, "line5") {
		t.Errorf("expected only lines 2-4, but got extra content: %q", text)
	}
}

func TestReadFile_EmptyPath(t *testing.T) {
	repo := &mockRepoReader{}
	ts := newTestMCPServerWithRepo(t, &mockTracker{}, repo)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "read_file",
			"arguments": map[string]any{
				"path": "",
			},
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
		t.Error("expected isError=true for empty path")
	}
}

func TestGetDiff(t *testing.T) {
	repo := &mockRepoReader{
		diff: "Comparing main...feature\nStatus: ahead | Ahead by 3 | Behind by 0\n",
	}
	ts := newTestMCPServerWithRepo(t, &mockTracker{}, repo)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "get_diff",
			"arguments": map[string]any{
				"base": "main",
				"head": "feature",
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
	if !strings.Contains(result.Content[0].Text, "feature") {
		t.Errorf("response missing diff content, got %q", result.Content[0].Text)
	}
}

func TestGetDiff_EmptyBase(t *testing.T) {
	repo := &mockRepoReader{}
	ts := newTestMCPServerWithRepo(t, &mockTracker{}, repo)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      6,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "get_diff",
			"arguments": map[string]any{
				"base": "",
				"head": "feature",
			},
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
		t.Error("expected isError=true for empty base")
	}
}

func TestListBranches(t *testing.T) {
	repo := &mockRepoReader{
		branches: []forge.Branch{
			{Name: "main", LastCommit: "abc123", UpdatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
			{Name: "feature/test", LastCommit: "def456", UpdatedAt: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)},
		},
	}
	ts := newTestMCPServerWithRepo(t, &mockTracker{}, repo)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      7,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "list_branches",
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
	if !strings.Contains(result.Content[0].Text, "main") {
		t.Errorf("response missing main branch, got %q", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "feature/test") {
		t.Errorf("response missing feature branch, got %q", result.Content[0].Text)
	}
}

func TestGetCommitLog(t *testing.T) {
	repo := &mockRepoReader{
		commits: []forge.Commit{
			{SHA: "abc123", Message: "feat: add widget", Author: "dev", Date: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)},
			{SHA: "def456", Message: "fix: typo", Author: "dev", Date: time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC)},
		},
	}
	ts := newTestMCPServerWithRepo(t, &mockTracker{}, repo)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      8,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "get_commit_log",
			"arguments": map[string]any{
				"branch": "main",
				"limit":  10,
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
	if !strings.Contains(result.Content[0].Text, "abc123") {
		t.Errorf("response missing commit SHA, got %q", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "feat: add widget") {
		t.Errorf("response missing commit message, got %q", result.Content[0].Text)
	}
}

func TestSearchCode(t *testing.T) {
	repo := &mockRepoReader{
		search: []forge.SearchResult{
			{Path: "internal/server/server.go", LineNumber: 42, Line: "func NewServer(cfg Config) *Server {"},
		},
	}
	ts := newTestMCPServerWithRepo(t, &mockTracker{}, repo)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      9,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "search_code",
			"arguments": map[string]any{
				"query": "NewServer",
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
	if !strings.Contains(result.Content[0].Text, "server.go") {
		t.Errorf("response missing file path, got %q", result.Content[0].Text)
	}
}

func TestSearchCode_EmptyQuery(t *testing.T) {
	repo := &mockRepoReader{}
	ts := newTestMCPServerWithRepo(t, &mockTracker{}, repo)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      10,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "search_code",
			"arguments": map[string]any{
				"query": "",
			},
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
		t.Error("expected isError=true for empty query")
	}
}

func TestRepoToolsNilRepo(t *testing.T) {
	// All 6 repo tools should return "not available" when repo is nil.
	ts := newTestMCPServerWithRepo(t, &mockTracker{}, nil)

	tools := []struct {
		name string
		args map[string]any
	}{
		{"list_files", map[string]any{}},
		{"read_file", map[string]any{"path": "test.go"}},
		{"get_diff", map[string]any{"base": "main", "head": "feature"}},
		{"list_branches", map[string]any{}},
		{"get_commit_log", map[string]any{}},
		{"search_code", map[string]any{"query": "test"}},
	}

	for _, tool := range tools {
		t.Run(tool.name, func(t *testing.T) {
			respBody := postJSON(t, ts.URL, jsonRPCRequest{
				JSONRPC: "2.0",
				ID:      20,
				Method:  "tools/call",
				Params: map[string]any{
					"name":      tool.name,
					"arguments": tool.args,
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
			if !strings.Contains(result.Content[0].Text, "not available") {
				t.Errorf("expected 'not available' message, got %q", result.Content[0].Text)
			}
		})
	}
}
