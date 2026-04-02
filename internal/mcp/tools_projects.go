package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"samverk.dev/samverk/internal/autonomy"
)

// listProjectsInput is the typed input for the list_projects tool.
type listProjectsInput struct{}

// setProjectInput is the typed input for the set_project tool.
type setProjectInput struct {
	Name string `json:"name" jsonschema:"required,the project name to switch to"`
}

// setProjectPhaseInput is the typed input for the set_project_phase tool.
type setProjectPhaseInput struct {
	Name   string `json:"name" jsonschema:"required,the project name to update"`
	Phase  string `json:"phase" jsonschema:"required,the new lifecycle phase (research/planning/design/development/deployed/maintenance/inactive)"`
	Reason string `json:"reason,omitempty" jsonschema:"optional reason for the phase transition"`
}

// projectInfo is the JSON output for a single project in list_projects.
type projectInfo struct {
	Name      string   `json:"name"`
	Owner     string   `json:"owner"`
	Repo      string   `json:"repo"`
	Phase     string   `json:"phase"`
	Tags      []string `json:"tags,omitempty"`
	Active    bool     `json:"active"`
	ForgeType string   `json:"forge_type,omitempty"`
	ForgeURL  string   `json:"forge_url,omitempty"`
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

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "set_project_phase",
		Description: "Transition a project to a new lifecycle phase (requires Tier 3 confirmation). Valid phases: research, planning, design, development, deployed, maintenance, inactive.",
	}, h.handleSetProjectPhase)
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
			Name:      p.Name,
			Owner:     p.Owner,
			Repo:      p.Repo,
			Phase:     p.Phase,
			Tags:      p.Tags,
			Active:    p.Name == activeName,
			ForgeType: p.ForgeType,
			ForgeURL:  p.ForgeURL,
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

// handleSetProjectPhase transitions a project to a new lifecycle phase.
// This is a Tier 3 (ActionRestructure) operation — it queues for user confirmation
// before taking effect. On approval the phase is updated in memory and persisted
// to server.yaml (if a config path is set on the registry).
func (h *Handler) handleSetProjectPhase(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input setProjectPhaseInput,
) (*gosdk.CallToolResult, any, error) {
	if h.projects == nil {
		return nil, nil, fmt.Errorf("multi-project support not configured")
	}
	if input.Name == "" {
		return nil, nil, fmt.Errorf("name must not be empty")
	}
	if input.Phase == "" {
		return nil, nil, fmt.Errorf("phase must not be empty")
	}
	if !ValidPhases[input.Phase] {
		return nil, nil, fmt.Errorf("invalid phase %q: must be one of research, planning, design, development, deployed, maintenance, inactive", input.Phase)
	}

	// Look up current phase for the confirmation message.
	p, exists := h.projects.Get(input.Name)
	if !exists {
		return nil, nil, fmt.Errorf("project %q not found", input.Name)
	}
	oldPhase := p.Phase

	reason := input.Reason
	if reason == "" {
		reason = "no reason provided"
	}

	// Build a descriptive label for the pending action so the user knows exactly
	// what they are approving.
	actionLabel := fmt.Sprintf("set_project_phase: %s %s→%s (%s) at %s",
		input.Name, oldPhase, input.Phase, reason, time.Now().UTC().Format(time.RFC3339))

	if result := h.checkTier(autonomy.ActionRestructure, actionLabel, func() (*gosdk.CallToolResult, error) {
		if err := h.projects.SetPhase(input.Name, input.Phase); err != nil {
			return nil, fmt.Errorf("set phase: %w", err)
		}
		h.recorder.recordToolCall(ctx, "set_project_phase", 0)
		text := fmt.Sprintf("Project %q transitioned from %q to %q. Reason: %s", input.Name, oldPhase, input.Phase, reason)
		return &gosdk.CallToolResult{
			Content: []gosdk.Content{&gosdk.TextContent{Text: text}},
		}, nil
	}); result != nil {
		return result, nil, nil
	}

	// Tier 1/2: execute immediately (should not normally occur for ActionRestructure).
	if err := h.projects.SetPhase(input.Name, input.Phase); err != nil {
		return nil, nil, fmt.Errorf("set phase: %w", err)
	}
	h.recorder.recordToolCall(ctx, "set_project_phase", 0)
	text := fmt.Sprintf("Project %q transitioned from %q to %q. Reason: %s", input.Name, oldPhase, input.Phase, reason)
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{&gosdk.TextContent{Text: text}},
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
