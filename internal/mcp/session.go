package mcp

import (
	"context"
	"go.uber.org/zap"
	"time"

	"samverk.dev/samverk/internal/store"
	"samverk.dev/samverk/pkg/models"
)

// sessionRecorder wraps tool calls with session/cost recording.
type sessionRecorder struct {
	store  store.Store
	logger *zap.Logger
}

// recordToolCall creates a session and cost record for an MCP tool invocation.
// provider and model default to "mcp" and the tool name since the MCP server
// doesn't know the client's model -- real token data comes from provider clients.
func (r *sessionRecorder) recordToolCall(ctx context.Context, toolName string, issueNumber int) {
	if r == nil || r.store == nil {
		return
	}

	now := time.Now().UTC()
	finished := now
	sess := &models.Session{
		IssueNumber: issueNumber,
		AgentType:   "mcp-tool",
		Provider:    "mcp",
		Model:       toolName,
		Status:      models.SessionStatusCompleted,
		StartedAt:   now,
		FinishedAt:  &finished,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := r.store.CreateSession(ctx, sess); err != nil {
		r.logger.Warn("session recorder: failed to create session", zap.String("tool", toolName), zap.Error(err))
		return
	}

	// Record a cost entry (tokens will be 0 for MCP-side calls;
	// real token data comes from provider clients in future).
	cost := &models.CostRecord{
		SessionID: sess.ID,
		Provider:  "mcp",
		Model:     toolName,
	}
	if err := r.store.RecordCost(ctx, cost); err != nil {
		r.logger.Warn("session recorder: failed to record cost", zap.String("tool", toolName), zap.Error(err))
	}
}
