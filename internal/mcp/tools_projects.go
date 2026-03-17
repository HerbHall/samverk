package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// listProjectsInput is the typed input for the list_projects tool.
type listProjectsInput struct{}

// setProjectInput is the typed input for the set_project tool.
type setProjectInput struct {
	Name string `json:"name" jsonschema:"required,the project name to switch to"`
}

// projectInfo is the JSON output for a single project in list_projects.
type projectInfo struct {
	Name   string   `json:"name"`
	Owner  string   `json:"owner"`
	Repo   string   `json:"repo"`
	Phase  string   `json:"phase"`
	Tags   []string `json:"tags,omitempty"`
	Active bool     `json:"active"`
}

// registerProjectTools adds project management tools to the MCP server.
func registerProjectTools(srv *gosdk.Server, h *Handler) {
	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "list_projects",
		Description: "List all registered projects with their active status",
	}, h.handleListProjects)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "set_project",
		Description: "Switch the active project context",
	}, h.handleSetProject)
}

// handleListProjects returns all registered projects with their active status.
func (h *Handler) handleListProjects(
	_ context.Context,
	_ *gosdk.CallToolRequest,
	_ listProjectsInput,
) (*gosdk.CallToolResult, any, error) {
	if h.projects == nil {
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{
				&gosdk.TextContent{Text: "multi-project support not configured"},
			},
		}, nil, nil
	}

	projects := h.projects.List()
	activeName := h.projects.ActiveName()

	infos := make([]projectInfo, 0, len(projects))
	for _, p := range projects {
		infos = append(infos, projectInfo{
			Name:   p.Name,
			Owner:  p.Owner,
			Repo:   p.Repo,
			Phase:  p.Phase,
			Tags:   p.Tags,
			Active: p.Name == activeName,
		})
	}

	result, err := json.Marshal(infos)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling projects: %w", err)
	}

	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// handleSetProject switches the active project context.
func (h *Handler) handleSetProject(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input setProjectInput,
) (*gosdk.CallToolResult, any, error) {
	if h.projects == nil {
		return nil, nil, fmt.Errorf("multi-project support not configured")
	}

	if input.Name == "" {
		return nil, nil, fmt.Errorf("name must not be empty")
	}

	if err := h.projects.SetActive(input.Name); err != nil {
		return nil, nil, fmt.Errorf("setting active project: %w", err)
	}

	h.recorder.recordToolCall(ctx, "set_project", 0)

	p, _ := h.projects.Active()
	text := fmt.Sprintf("Switched to project %q (%s/%s)", p.Name, p.Owner, p.Repo)
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{
			&gosdk.TextContent{Text: text},
		},
	}, nil, nil
}
