package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/herbhall/samverk/internal/cost"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
)

// Runner executes a single agent task: sends the issue to an AI provider,
// records cost, and posts the response as an issue comment.
type Runner struct {
	provider provider.Provider
	model    string
	tracker  forge.IssueTracker
	store    store.Store
	costs    *cost.Tracker
	logger   *slog.Logger
}

// NewRunner creates a runner bound to a specific provider and model.
func NewRunner(p provider.Provider, model string, tracker forge.IssueTracker, st store.Store, costs *cost.Tracker) *Runner {
	return &Runner{
		provider: p,
		model:    model,
		tracker:  tracker,
		store:    st,
		costs:    costs,
		logger:   slog.Default(),
	}
}

// Run processes a single task through the AI provider pipeline.
//
// Steps:
//  1. Update session status to "active"
//  2. Check budget -- if exceeded, mark "failed" and return
//  3. Build system prompt and chat request
//  4. Call provider.Chat
//  5. Record cost via cost tracker
//  6. Post response as issue comment
//  7. Mark session "completed"
//
// On any error, the session is marked "failed" and an error comment is posted.
func (r *Runner) Run(ctx context.Context, task Task) error {
	// Step 1: Mark session active.
	if err := r.updateSessionStatus(ctx, task.SessionID, models.SessionStatusActive, ""); err != nil {
		return fmt.Errorf("activate session: %w", err)
	}

	// Step 2: Check budget.
	if err := r.costs.CheckBudget(ctx); err != nil {
		if errors.Is(err, cost.ErrBudgetExceeded) {
			r.failTask(ctx, task, "budget exceeded: daily spend limit reached")
			return err
		}
		r.failTask(ctx, task, fmt.Sprintf("budget check failed: %v", err))
		return fmt.Errorf("check budget: %w", err)
	}

	// Step 3: Build chat request.
	systemPrompt := buildSystemPrompt(task)
	req := provider.ChatRequest{
		Model: r.model,
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: systemPrompt},
			{Role: provider.RoleUser, Content: task.Issue.Body},
		},
	}

	// Step 4: Call provider.
	resp, err := r.provider.Chat(ctx, req)
	if err != nil {
		r.failTask(ctx, task, fmt.Sprintf("provider error: %v", err))
		return fmt.Errorf("provider chat: %w", err)
	}

	// Step 5: Record cost.
	if err = r.costs.RecordUsage(ctx, task.SessionID, r.provider.Name(), r.model, resp); err != nil {
		r.logger.Error("failed to record cost",
			slog.String("session_id", task.SessionID),
			slog.String("error", err.Error()),
		)
		// Non-fatal: continue even if cost recording fails.
	}

	// Step 6: Post response as comment.
	if _, err = r.tracker.AddComment(ctx, task.Issue.Number, resp.Message.Content); err != nil {
		r.failTask(ctx, task, fmt.Sprintf("failed to post comment: %v", err))
		return fmt.Errorf("add comment: %w", err)
	}

	// Step 7: Mark session completed.
	if err = r.completeSession(ctx, task.SessionID); err != nil {
		return fmt.Errorf("complete session: %w", err)
	}

	return nil
}

// buildSystemPrompt constructs the system prompt for the AI provider based on
// the task's agent type, issue number, title, and labels.
func buildSystemPrompt(task Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are a %s agent working on issue #%d: %s\n",
		task.AgentType, task.Issue.Number, task.Issue.Title)

	if len(task.Issue.Labels) > 0 {
		fmt.Fprintf(&b, "Labels: %s\n", strings.Join(task.Issue.Labels, ", "))
	}

	fmt.Fprintf(&b, "\nAnalyze the issue and provide a thorough response.")
	return b.String()
}

// updateSessionStatus fetches and updates a session's status in the store.
func (r *Runner) updateSessionStatus(ctx context.Context, sessionID string, status models.SessionStatus, errMsg string) error {
	session, err := r.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.Status = status
	session.Error = errMsg
	session.UpdatedAt = time.Now()

	return r.store.UpdateSession(ctx, session)
}

// completeSession marks a session as completed with a finish timestamp.
func (r *Runner) completeSession(ctx context.Context, sessionID string) error {
	session, err := r.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	now := time.Now()
	session.Status = models.SessionStatusCompleted
	session.FinishedAt = &now
	session.UpdatedAt = now

	return r.store.UpdateSession(ctx, session)
}

// failTask marks the session as failed and posts an error comment on the issue.
func (r *Runner) failTask(ctx context.Context, task Task, errMsg string) {
	if err := r.updateSessionStatus(ctx, task.SessionID, models.SessionStatusFailed, errMsg); err != nil {
		r.logger.Error("failed to update session on error",
			slog.String("session_id", task.SessionID),
			slog.String("error", err.Error()),
		)
	}

	comment := fmt.Sprintf("Agent error: %s", errMsg)
	if _, err := r.tracker.AddComment(ctx, task.Issue.Number, comment); err != nil {
		r.logger.Error("failed to post error comment",
			slog.String("session_id", task.SessionID),
			slog.Int("issue", task.Issue.Number),
			slog.String("error", err.Error()),
		)
	}
}
