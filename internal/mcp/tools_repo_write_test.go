package mcp_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/herbhall/samverk/internal/forge"
	internalmcp "github.com/herbhall/samverk/internal/mcp"
)

// mockRepoWriter implements forge.RepoWriter for testing.
type mockRepoWriter struct {
	createdBranches []string
	writtenFiles    []writtenFile
}

type writtenFile struct {
	Branch, Path, Content, Message string
}

func (m *mockRepoWriter) CreateBranch(_ context.Context, name string) error {
	m.createdBranches = append(m.createdBranches, name)
	return nil
}

func (m *mockRepoWriter) CreateOrUpdateFile(_ context.Context, branch, path, content, message string) error {
	m.writtenFiles = append(m.writtenFiles, writtenFile{
		Branch:  branch,
		Path:    path,
		Content: content,
		Message: message,
	})
	return nil
}

// newTestMCPServerWithWriter sets up an httptest.Server with a RepoWriter.
func newTestMCPServerWithWriter(t *testing.T, writer forge.RepoWriter) *httptest.Server {
	t.Helper()
	h := internalmcp.NewHandler(&mockTracker{}, nil, nil, nil, nil)
	if writer != nil {
		h.SetWriter(writer)
	}
	handler := internalmcp.NewHTTPHandler(h)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

func TestWriteFile_Success(t *testing.T) {
	writer := &mockRepoWriter{}
	ts := newTestMCPServerWithWriter(t, writer)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "write_file",
			"arguments": map[string]any{
				"branch":  "feature/test",
				"path":    "README.md",
				"content": "# Hello World",
				"message": "docs: add readme",
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
	if !strings.Contains(result.Content[0].Text, `"committed":true`) {
		t.Errorf("expected committed:true, got %q", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "feature/test") {
		t.Errorf("expected branch name in response, got %q", result.Content[0].Text)
	}

	if len(writer.writtenFiles) != 1 {
		t.Fatalf("expected 1 written file, got %d", len(writer.writtenFiles))
	}
	wf := writer.writtenFiles[0]
	if wf.Branch != "feature/test" || wf.Path != "README.md" || wf.Content != "# Hello World" {
		t.Errorf("unexpected written file: %+v", wf)
	}
}

func TestWriteFile_NilWriter(t *testing.T) {
	ts := newTestMCPServerWithWriter(t, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "write_file",
			"arguments": map[string]any{
				"branch":  "main",
				"path":    "test.go",
				"content": "package main",
				"message": "test",
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
	if len(result.Content) == 0 {
		t.Fatal("expected content in response")
	}
	if !strings.Contains(result.Content[0].Text, "not available") {
		t.Errorf("expected 'not available' message, got %q", result.Content[0].Text)
	}
}

func TestCreateBranch_Success(t *testing.T) {
	writer := &mockRepoWriter{}
	ts := newTestMCPServerWithWriter(t, writer)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "create_branch",
			"arguments": map[string]any{
				"name": "feature/new-branch",
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
	if !strings.Contains(result.Content[0].Text, `"created":true`) {
		t.Errorf("expected created:true, got %q", result.Content[0].Text)
	}

	if len(writer.createdBranches) != 1 || writer.createdBranches[0] != "feature/new-branch" {
		t.Errorf("unexpected branches: %v", writer.createdBranches)
	}
}

func TestCreateBranch_NilWriter(t *testing.T) {
	ts := newTestMCPServerWithWriter(t, nil)

	respBody := postJSON(t, ts.URL, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "create_branch",
			"arguments": map[string]any{
				"name": "feature/test",
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
	if len(result.Content) == 0 {
		t.Fatal("expected content in response")
	}
	if !strings.Contains(result.Content[0].Text, "not available") {
		t.Errorf("expected 'not available' message, got %q", result.Content[0].Text)
	}
}
