package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"samverk.dev/samverk/internal/autonomy"
)

// writeFileInput is the typed input for the write_file tool.
type writeFileInput struct {
	Branch  string `json:"branch"  jsonschema:"required,target branch name"`
	Path    string `json:"path"    jsonschema:"required,file path in the repository"`
	Content string `json:"content" jsonschema:"required,complete file content (UTF-8)"`
	Message string `json:"message" jsonschema:"required,commit message"`
}

// createBranchInput is the typed input for the create_branch tool.
type createBranchInput struct {
	Name string `json:"name" jsonschema:"required,new branch name (must not already exist)"`
}

// registerRepoWriteTools adds repository write tools to the MCP server.
func registerRepoWriteTools(srv *gosdk.Server, h *Handler) {
	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "write_file",
		Description: "Create or update a file on a branch with a commit message",
	}, h.handleWriteFile)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "create_branch",
		Description: "Create a new branch from the default branch HEAD",
	}, h.handleCreateBranch)
}

// handleWriteFile creates or updates a file on a branch.
func (h *Handler) handleWriteFile(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input writeFileInput,
) (*gosdk.CallToolResult, any, error) {
	if h.activeWriter() == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "write operations not available: no RepoWriter configured"},
			},
		}, nil, nil
	}

	if input.Branch == "" {
		return nil, nil, fmt.Errorf("branch must not be empty")
	}
	if input.Path == "" {
		return nil, nil, fmt.Errorf("path must not be empty")
	}
	if input.Message == "" {
		return nil, nil, fmt.Errorf("message must not be empty")
	}

	if result := h.checkTier(autonomy.ActionEditFile, "write_file", func() (*gosdk.CallToolResult, error) {
		return h.executeWriteFile(ctx, input)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeWriteFile(ctx, input)
	return r, nil, err
}

func (h *Handler) executeWriteFile(ctx context.Context, input writeFileInput) (*gosdk.CallToolResult, error) {
	if err := h.activeWriter().CreateOrUpdateFile(ctx, input.Branch, input.Path, input.Content, input.Message); err != nil {
		return nil, fmt.Errorf("writing file: %w", err)
	}

	result, marshalErr := json.Marshal(map[string]any{
		"branch":    input.Branch,
		"path":      input.Path,
		"committed": true,
	})
	if marshalErr != nil {
		return nil, fmt.Errorf("marshalling result: %w", marshalErr)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil
}

// handleCreateBranch creates a new branch from the default branch HEAD.
func (h *Handler) handleCreateBranch(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input createBranchInput,
) (*gosdk.CallToolResult, any, error) {
	if h.activeWriter() == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "write operations not available: no RepoWriter configured"},
			},
		}, nil, nil
	}

	if input.Name == "" {
		return nil, nil, fmt.Errorf("name must not be empty")
	}

	if result := h.checkTier(autonomy.ActionEditFile, "create_branch", func() (*gosdk.CallToolResult, error) {
		return h.executeCreateBranch(ctx, input)
	}); result != nil {
		return result, nil, nil
	}

	r, err := h.executeCreateBranch(ctx, input)
	return r, nil, err
}

func (h *Handler) executeCreateBranch(ctx context.Context, input createBranchInput) (*gosdk.CallToolResult, error) {
	if err := h.activeWriter().CreateBranch(ctx, input.Name); err != nil {
		return nil, fmt.Errorf("creating branch: %w", err)
	}

	result, marshalErr := json.Marshal(map[string]any{
		"branch":  input.Name,
		"created": true,
	})
	if marshalErr != nil {
		return nil, fmt.Errorf("marshalling result: %w", marshalErr)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil
}
