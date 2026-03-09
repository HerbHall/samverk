package mcp

import (
	"context"
	"fmt"
	"time"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/pkg/models"
)

// scaleSetInput is the typed input for the scale_set tool.
type scaleSetInput struct {
	Workers int    `json:"workers" jsonschema:"required,target worker count (>= 1)"`
	Note    string `json:"note,omitempty" jsonschema:"optional reason for the manual override"`
}

// registerScalingTools registers MCP tools for scaling control and observation.
func registerScalingTools(srv *gosdk.Server, h *Handler) {
	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "scale_status",
		Description: "Get current scaling control state (paused, manual target, autoscaler mode). Read-only.",
	}, h.handleScaleStatus)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "scale_history",
		Description: "List recent scaling events (scale-up, scale-down, manual-override) from durable storage.",
	}, h.handleScaleHistory)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "scale_set",
		Description: "Force the worker pool to a specific size and pause autonomous autoscaling. Requires Tier 2 confirmation.",
	}, h.handleScaleSet)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "scale_pause",
		Description: "Pause the autoscaler so it takes no autonomous scaling actions. Current workers are kept. Requires Tier 2 confirmation.",
	}, h.handleScalePause)

	gosdk.AddTool(srv, &gosdk.Tool{
		Name:        "scale_resume",
		Description: "Resume autonomous autoscaling and clear any manual worker target. Requires Tier 2 confirmation.",
	}, h.handleScaleResume)
}

// handleScaleStatus returns the current scaling control state (Tier 1 - read-only).
func (h *Handler) handleScaleStatus(
	_ context.Context,
	_ *gosdk.CallToolRequest,
	_ struct{},
) (*gosdk.CallToolResult, any, error) {
	if h.store == nil {
		return scaleText("scaling control not available (no database)"), nil, nil
	}
	ctrl, err := h.store.GetScalingControl(context.Background())
	if err != nil {
		return scaleText(fmt.Sprintf("error: %v", err)), nil, nil
	}
	out := fmt.Sprintf("Scaling control:\n  paused: %v\n  manual_workers: %d\n  note: %s\n",
		ctrl.Paused, ctrl.ManualWorkers, ctrl.Note)
	if !ctrl.SetAt.IsZero() {
		out += fmt.Sprintf("  set_at: %s\n", ctrl.SetAt.UTC().Format(time.RFC3339))
	}
	switch {
	case !ctrl.Paused && ctrl.ManualWorkers == 0:
		out += "  mode: autonomous (autoscaler active)\n"
	case ctrl.Paused && ctrl.ManualWorkers > 0:
		out += fmt.Sprintf("  mode: manual (target=%d workers, autoscaler paused)\n", ctrl.ManualWorkers)
	case ctrl.Paused:
		out += "  mode: paused (autoscaler suspended, worker count unchanged)\n"
	}
	return scaleText(out), nil, nil
}

// handleScaleHistory lists recent scaling events (Tier 1 - read-only).
func (h *Handler) handleScaleHistory(
	_ context.Context,
	_ *gosdk.CallToolRequest,
	_ struct{},
) (*gosdk.CallToolResult, any, error) {
	if h.scalingEvents == nil {
		return scaleText("scaling event history not available (no database)"), nil, nil
	}
	events, err := h.scalingEvents.ListScalingEvents(context.Background(), 20)
	if err != nil {
		return scaleText(fmt.Sprintf("error: %v", err)), nil, nil
	}
	if len(events) == 0 {
		return scaleText("No scaling events recorded yet."), nil, nil
	}
	out := fmt.Sprintf("Recent scaling events (%d):\n", len(events))
	for _, e := range events {
		out += fmt.Sprintf("  [%s] %s %d→%d workers — %s (conf %.0f%%)\n",
			e.Timestamp.UTC().Format("2006-01-02 15:04:05Z"),
			e.Action,
			e.FromWorkers, e.ToWorkers,
			e.Reason,
			e.Confidence*100,
		)
	}
	return scaleText(out), nil, nil
}

// handleScaleSet forces the pool to a specific worker count (Tier 2).
func (h *Handler) handleScaleSet(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	input scaleSetInput,
) (*gosdk.CallToolResult, any, error) {
	if result := h.checkTier(autonomy.ActionRunService, "scale_set", func() (*gosdk.CallToolResult, error) {
		return h.execScaleSet(ctx, input.Workers, input.Note)
	}); result != nil {
		return result, nil, nil
	}
	r, err := h.execScaleSet(ctx, input.Workers, input.Note)
	return r, nil, err
}

func (h *Handler) execScaleSet(ctx context.Context, workers int, note string) (*gosdk.CallToolResult, error) {
	if h.store == nil {
		return scaleText("scaling control not available (no database)"), nil
	}
	if workers < 1 {
		return scaleText("error: workers must be >= 1"), nil
	}
	if note == "" {
		note = "set via MCP"
	}
	ctrl := models.ScalingControl{
		Paused:        true,
		ManualWorkers: workers,
		SetAt:         time.Now().UTC(),
		Note:          note,
	}
	if err := h.store.UpsertScalingControl(ctx, ctrl); err != nil {
		return scaleText(fmt.Sprintf("error: %v", err)), nil
	}
	return scaleText(fmt.Sprintf("OK: manual worker target set to %d, autoscaler paused. Note: %s", workers, note)), nil
}

// handleScalePause pauses the autoscaler (Tier 2).
func (h *Handler) handleScalePause(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	_ struct{},
) (*gosdk.CallToolResult, any, error) {
	if result := h.checkTier(autonomy.ActionRunService, "scale_pause", func() (*gosdk.CallToolResult, error) {
		return h.execScalePause(ctx)
	}); result != nil {
		return result, nil, nil
	}
	r, err := h.execScalePause(ctx)
	return r, nil, err
}

func (h *Handler) execScalePause(ctx context.Context) (*gosdk.CallToolResult, error) {
	if h.store == nil {
		return scaleText("scaling control not available (no database)"), nil
	}
	ctrl := models.ScalingControl{Paused: true, SetAt: time.Now().UTC(), Note: "paused via MCP"}
	if err := h.store.UpsertScalingControl(ctx, ctrl); err != nil {
		return scaleText(fmt.Sprintf("error: %v", err)), nil
	}
	return scaleText("OK: autoscaler paused. Current worker count will be maintained."), nil
}

// handleScaleResume resumes the autoscaler (Tier 2).
func (h *Handler) handleScaleResume(
	ctx context.Context,
	_ *gosdk.CallToolRequest,
	_ struct{},
) (*gosdk.CallToolResult, any, error) {
	if result := h.checkTier(autonomy.ActionRunService, "scale_resume", func() (*gosdk.CallToolResult, error) {
		return h.execScaleResume(ctx)
	}); result != nil {
		return result, nil, nil
	}
	r, err := h.execScaleResume(ctx)
	return r, nil, err
}

func (h *Handler) execScaleResume(ctx context.Context) (*gosdk.CallToolResult, error) {
	if h.store == nil {
		return scaleText("scaling control not available (no database)"), nil
	}
	ctrl := models.ScalingControl{Paused: false, ManualWorkers: 0, SetAt: time.Now().UTC(), Note: "resumed via MCP"}
	if err := h.store.UpsertScalingControl(ctx, ctrl); err != nil {
		return scaleText(fmt.Sprintf("error: %v", err)), nil
	}
	return scaleText("OK: autoscaler resumed. Autonomous scaling is now active."), nil
}

// scaleText returns a *gosdk.CallToolResult with plain-text content.
func scaleText(text string) *gosdk.CallToolResult {
	return &gosdk.CallToolResult{
		Content: []gosdk.Content{&gosdk.TextContent{Text: text}},
	}
}
