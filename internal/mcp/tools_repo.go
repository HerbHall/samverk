package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// listFilesInput is the typed input for the list_files tool.
type listFilesInput struct {
	Path string `json:"path,omitempty" jsonschema:"repository path to list (default: root)"`
	Ref  string `json:"ref,omitempty" jsonschema:"git ref: branch, tag, or commit SHA (default: HEAD)"`
}

// readFileInput is the typed input for the read_file tool.
type readFileInput struct {
	Path      string `json:"path" jsonschema:"required,file path to read"`
	Ref       string `json:"ref,omitempty" jsonschema:"git ref: branch, tag, or commit SHA (default: HEAD)"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"first line to return (1-based, inclusive)"`
	EndLine   int    `json:"end_line,omitempty" jsonschema:"last line to return (1-based, inclusive)"`
}

// getDiffInput is the typed input for the get_diff tool.
type getDiffInput struct {
	Base string `json:"base" jsonschema:"required,base ref (branch, tag, or commit SHA)"`
	Head string `json:"head" jsonschema:"required,head ref (branch, tag, or commit SHA)"`
}

// listBranchesInput is the typed input for the list_branches tool.
type listBranchesInput struct{}

// getCommitLogInput is the typed input for the get_commit_log tool.
type getCommitLogInput struct {
	Branch string `json:"branch,omitempty" jsonschema:"branch name (default: main)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max number of commits to return (default: 20)"`
}

// searchCodeInput is the typed input for the search_code tool.
type searchCodeInput struct {
	Query string `json:"query" jsonschema:"required,code search query"`
}

// registerRepoTools adds read-only repository browsing tools to the MCP server.
func registerRepoTools(srv *gosdk.Server, h *Handler) {
	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "list_files",
		Description: "List files and directories at a path in the repository",
	}, h.handleListFiles)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "read_file",
		Description: "Read the contents of a file from the repository",
	}, h.handleReadFile)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "get_diff",
		Description: "Get the diff between two refs (branches, tags, or commits)",
	}, h.handleGetDiff)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "list_branches",
		Description: "List all branches in the repository",
	}, h.handleListBranches)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "get_commit_log",
		Description: "Get recent commits on a branch",
	}, h.handleGetCommitLog)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "search_code",
		Description: "Search for code in the repository",
	}, h.handleSearchCode)
}

// handleListFiles lists files at a given path in the repository.
func (h *Handler) handleListFiles(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input listFilesInput,
) (*gosdk.CallToolResult, any, error) {
	if h.activeReader() == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "repo operations not available: no RepoReader configured"},
			},
		}, nil, nil
	}

	entries, err := h.activeReader().ListFiles(ctx, input.Path, input.Ref)
	if err != nil {
		return nil, nil, fmt.Errorf("listing files: %w", err)
	}

	result, err := json.Marshal(entries)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling file entries: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleReadFile reads a file from the repository with optional line range.
func (h *Handler) handleReadFile(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input readFileInput,
) (*gosdk.CallToolResult, any, error) {
	if h.activeReader() == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "repo operations not available: no RepoReader configured"},
			},
		}, nil, nil
	}

	if input.Path == "" {
		return nil, nil, fmt.Errorf("path must not be empty")
	}

	data, err := h.activeReader().ReadFile(ctx, input.Path, input.Ref)
	if err != nil {
		return nil, nil, fmt.Errorf("reading file: %w", err)
	}

	content := string(data)

	// Apply line range if specified.
	if input.StartLine > 0 || input.EndLine > 0 {
		lines := strings.Split(content, "\n")
		start := input.StartLine
		end := input.EndLine

		if start <= 0 {
			start = 1
		}
		if end <= 0 || end > len(lines) {
			end = len(lines)
		}
		if start > len(lines) {
			start = len(lines)
		}
		if start > end {
			start = end
		}

		// Convert to 0-based indexing.
		content = strings.Join(lines[start-1:end], "\n")
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: content},
		},
	}, nil, nil
}

// handleGetDiff returns the diff between two refs.
func (h *Handler) handleGetDiff(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input getDiffInput,
) (*gosdk.CallToolResult, any, error) {
	if h.activeReader() == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "repo operations not available: no RepoReader configured"},
			},
		}, nil, nil
	}

	if input.Base == "" {
		return nil, nil, fmt.Errorf("base must not be empty")
	}
	if input.Head == "" {
		return nil, nil, fmt.Errorf("head must not be empty")
	}

	diff, err := h.activeReader().GetDiff(ctx, input.Base, input.Head)
	if err != nil {
		return nil, nil, fmt.Errorf("getting diff: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: diff},
		},
	}, nil, nil
}

// handleListBranches lists all branches in the repository.
func (h *Handler) handleListBranches(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	_ listBranchesInput,
) (*gosdk.CallToolResult, any, error) {
	if h.activeReader() == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "repo operations not available: no RepoReader configured"},
			},
		}, nil, nil
	}

	branches, err := h.activeReader().ListBranches(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("listing branches: %w", err)
	}

	result, err := json.Marshal(branches)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling branches: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleGetCommitLog returns recent commits on a branch.
func (h *Handler) handleGetCommitLog(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input getCommitLogInput,
) (*gosdk.CallToolResult, any, error) {
	if h.activeReader() == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "repo operations not available: no RepoReader configured"},
			},
		}, nil, nil
	}

	branch := input.Branch
	if branch == "" {
		branch = "main"
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}

	commits, err := h.activeReader().GetCommitLog(ctx, branch, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("getting commit log: %w", err)
	}

	result, err := json.Marshal(commits)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling commits: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleSearchCode searches for code in the repository.
func (h *Handler) handleSearchCode(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input searchCodeInput,
) (*gosdk.CallToolResult, any, error) {
	if h.activeReader() == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "repo operations not available: no RepoReader configured"},
			},
		}, nil, nil
	}

	if input.Query == "" {
		return nil, nil, fmt.Errorf("query must not be empty")
	}

	results, err := h.activeReader().SearchCode(ctx, input.Query)
	if err != nil {
		return nil, nil, fmt.Errorf("searching code: %w", err)
	}

	result, err := json.Marshal(results)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling search results: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}
