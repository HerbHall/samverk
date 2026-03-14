package dispatcher

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/pkg/models"
)

// recordFailure classifies an error, persists a FailureEvent, and increments
// the persisted failure counter. It is the single entry point for all failure
// recording in the dispatcher.
func (d *Dispatcher) recordFailure(ctx context.Context, issueNumber int, sessionID, agentType, provider, errMsg string, duration time.Duration) {
	if d.store == nil {
		return
	}

	fc := classifyFailure(errMsg)

	// Shutdown kills (exit 143) are expected — log but don't count as failures.
	if fc == models.FailureClassShutdown {
		d.logger.Info("shutdown kill ignored for failure tracking",
			zap.Int("issue", issueNumber),
			zap.String("error", errMsg),
		)
		return
	}

	// Increment persisted counter and use it as the attempt number.
	attempt, err := d.store.IncrementIssueFailureCount(ctx, issueNumber)
	if err != nil {
		d.logger.Error("increment failure count",
			zap.Int("issue", issueNumber),
			zap.Error(err),
		)
		attempt = 1 // fallback
	}

	event := &models.FailureEvent{
		IssueNumber:  issueNumber,
		SessionID:    sessionID,
		FailureClass: fc,
		ErrorMessage: errMsg,
		AgentType:    agentType,
		Provider:     provider,
		AttemptNumber: attempt,
		Duration:     duration,
		Timestamp:    time.Now().UTC(),
	}

	if err = d.store.SaveFailureEvent(ctx, event); err != nil {
		d.logger.Error("save failure event",
			zap.Int("issue", issueNumber),
			zap.String("class", string(fc)),
			zap.Error(err),
		)
	}

	// Update circuit breaker state.
	if d.circuitBreaker != nil {
		d.circuitBreaker.RecordFailure(fc, provider)
	}

	d.logger.Warn("failure recorded",
		zap.Int("issue", issueNumber),
		zap.String("class", string(fc)),
		zap.String("session", sessionID),
		zap.Int("attempt", attempt),
		zap.String("error", errMsg),
	)
}

// clearFailure removes the persisted failure count for an issue (on success).
func (d *Dispatcher) clearFailure(ctx context.Context, issueNumber int) {
	if d.store == nil {
		return
	}
	if err := d.store.ClearIssueFailureCount(ctx, issueNumber); err != nil {
		d.logger.Error("clear failure count",
			zap.Int("issue", issueNumber),
			zap.Error(err),
		)
	}
}

// getPersistedFailureCount returns the failure count from SQLite (survives restarts).
func (d *Dispatcher) getPersistedFailureCount(ctx context.Context, issueNumber int) int {
	if d.store == nil {
		return 0
	}
	count, err := d.store.GetIssueFailureCount(ctx, issueNumber)
	if err != nil {
		d.logger.Error("get failure count",
			zap.Int("issue", issueNumber),
			zap.Error(err),
		)
		return 0
	}
	return count
}
