package dispatcher

import (
	"context"
	"math"
	"math/rand/v2"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/agent"
	"github.com/herbhall/samverk/pkg/models"
)

// CorrectionScope indicates whether a correction is temporary (auto-revert on
// success) or permanent (persists for future dispatches).
type CorrectionScope string

const (
	// CorrectionScopeTemporary corrections revert once the issue succeeds.
	CorrectionScopeTemporary CorrectionScope = "temporary"
	// CorrectionScopePermanent corrections persist across retries.
	CorrectionScopePermanent CorrectionScope = "permanent"
)

// CorrectionAction describes the automated response to a failure.
type CorrectionAction string

const (
	CorrectionActionSwitchProvider  CorrectionAction = "switch_provider"
	CorrectionActionIncreaseTimeout CorrectionAction = "increase_timeout"
	CorrectionActionEscalate        CorrectionAction = "escalate"
	CorrectionActionRetryWithContext CorrectionAction = "retry_with_context"
	CorrectionActionRouteHigher     CorrectionAction = "route_higher"
	CorrectionActionPause           CorrectionAction = "pause"
	CorrectionActionRetry           CorrectionAction = "retry"
)

// CorrectionDecision captures what the correction engine decided and why.
type CorrectionDecision struct {
	IssueNumber  int                 `json:"issue_number"`
	FailureClass models.FailureClass `json:"failure_class"`
	Action       CorrectionAction    `json:"action"`
	Scope        CorrectionScope     `json:"scope"`
	Reason       string              `json:"reason"`
	NewProvider  string              `json:"new_provider,omitempty"`
	NewTimeout   time.Duration       `json:"new_timeout_ms,omitempty"`
	BackoffUntil time.Time           `json:"backoff_until,omitempty"`
	Attempt      int                 `json:"attempt"`
}

// maxRetryAttempts is the limit before the correction engine escalates.
const maxRetryAttempts = 3

// backoffBase is the initial backoff duration.
const backoffBase = 1 * time.Minute

// timeoutMultiplier scales the timeout on each retry.
const timeoutMultiplier = 1.5

// decideCorrection examines a failure and returns the correction to apply.
// It maps each of the 13 failure classes to a specific response strategy.
func decideCorrection(
	fc models.FailureClass,
	issueNumber, attempt int,
	currentTimeout time.Duration,
) CorrectionDecision {
	base := CorrectionDecision{
		IssueNumber:  issueNumber,
		FailureClass: fc,
		Attempt:      attempt,
	}

	switch fc {
	case models.FailureClassProviderDown:
		base.Action = CorrectionActionSwitchProvider
		base.Scope = CorrectionScopeTemporary
		base.Reason = "provider unreachable, switching to next in chain"
		base.BackoffUntil = backoffWithJitter(attempt)

	case models.FailureClassTimeout:
		if attempt >= maxRetryAttempts {
			base.Action = CorrectionActionEscalate
			base.Scope = CorrectionScopePermanent
			base.Reason = "timeout after max retries"
			return base
		}
		base.Action = CorrectionActionIncreaseTimeout
		base.Scope = CorrectionScopeTemporary
		base.Reason = "increasing timeout 1.5x"
		base.NewTimeout = time.Duration(float64(currentTimeout) * timeoutMultiplier)
		base.BackoffUntil = backoffWithJitter(attempt)

	case models.FailureClassAuth:
		base.Action = CorrectionActionEscalate
		base.Scope = CorrectionScopePermanent
		base.Reason = "authentication failure, manual intervention required"

	case models.FailureClassPermanent:
		base.Action = CorrectionActionEscalate
		base.Scope = CorrectionScopePermanent
		base.Reason = "permanent failure, no retry possible"

	case models.FailureClassBudget:
		base.Action = CorrectionActionPause
		base.Scope = CorrectionScopePermanent
		base.Reason = "budget exhausted, pausing and escalating"

	case models.FailureClassOOMKill:
		base.Action = CorrectionActionRouteHigher
		base.Scope = CorrectionScopeTemporary
		base.Reason = "OOM kill, routing to provider with more resources"
		base.BackoffUntil = backoffWithJitter(attempt)

	case models.FailureClassPanic:
		if attempt >= maxRetryAttempts {
			base.Action = CorrectionActionEscalate
			base.Scope = CorrectionScopePermanent
			base.Reason = "panic after max retries"
			return base
		}
		base.Action = CorrectionActionRetryWithContext
		base.Scope = CorrectionScopeTemporary
		base.Reason = "runtime panic, retrying with error context in prompt"
		base.BackoffUntil = backoffWithJitter(attempt)

	case models.FailureClassPostProcess:
		if attempt >= maxRetryAttempts {
			base.Action = CorrectionActionEscalate
			base.Scope = CorrectionScopePermanent
			base.Reason = "post-process failure after max retries"
			return base
		}
		base.Action = CorrectionActionRetry
		base.Scope = CorrectionScopeTemporary
		base.Reason = "post-process failure (PR/comment), retrying"
		base.BackoffUntil = backoffWithJitter(attempt)

	case models.FailureClassClassify:
		base.Action = CorrectionActionEscalate
		base.Scope = CorrectionScopePermanent
		base.Reason = "classification failure, issue needs manual frontmatter fix"

	case models.FailureClassCycle:
		base.Action = CorrectionActionEscalate
		base.Scope = CorrectionScopePermanent
		base.Reason = "dependency cycle, manual intervention required"

	case models.FailureClassDecompose:
		if attempt >= maxRetryAttempts {
			base.Action = CorrectionActionEscalate
			base.Scope = CorrectionScopePermanent
			base.Reason = "decomposition failure after max retries"
			return base
		}
		base.Action = CorrectionActionRetry
		base.Scope = CorrectionScopeTemporary
		base.Reason = "decomposition failure, retrying"
		base.BackoffUntil = backoffWithJitter(attempt)

	case models.FailureClassShutdown:
		base.Action = CorrectionActionRetry
		base.Scope = CorrectionScopeTemporary
		base.Reason = "shutdown kill, safe to retry immediately"

	case models.FailureClassUnknown:
		if attempt >= maxRetryAttempts {
			base.Action = CorrectionActionEscalate
			base.Scope = CorrectionScopePermanent
			base.Reason = "unknown failure after max retries"
			return base
		}
		base.Action = CorrectionActionRetryWithContext
		base.Scope = CorrectionScopeTemporary
		base.Reason = "unknown failure, retrying with error context"
		base.BackoffUntil = backoffWithJitter(attempt)
	}

	return base
}

// backoffWithJitter calculates exponential backoff with random jitter.
// attempt 1 -> ~1min, attempt 2 -> ~2min, attempt 3 -> ~4min.
func backoffWithJitter(attempt int) time.Time {
	if attempt < 1 {
		attempt = 1
	}
	base := float64(backoffBase) * math.Pow(2, float64(attempt-1))
	// Add jitter: +-25% of the base duration.
	jitter := base * 0.25 * (2*rand.Float64() - 1) //nolint:gosec // G404: jitter for backoff timing, not security-sensitive
	return time.Now().Add(time.Duration(base + jitter))
}

// applyCorrection executes a correction decision. It either escalates the
// issue or re-queues it with adjusted parameters.
func (d *Dispatcher) applyCorrection(ctx context.Context, result agent.TaskResult, decision CorrectionDecision) {
	d.logger.Info("correction decision",
		zap.Int("issue", decision.IssueNumber),
		zap.String("class", string(decision.FailureClass)),
		zap.String("action", string(decision.Action)),
		zap.String("scope", string(decision.Scope)),
		zap.String("reason", decision.Reason),
		zap.Int("attempt", decision.Attempt),
	)

	// Persist the correction decision.
	if d.store != nil {
		if err := d.store.SaveCorrectionEvent(ctx, &models.CorrectionEvent{
			IssueNumber:  decision.IssueNumber,
			FailureClass: decision.FailureClass,
			Action:       string(decision.Action),
			Scope:        string(decision.Scope),
			Reason:       decision.Reason,
			NewProvider:  decision.NewProvider,
			Outcome:      "pending",
		}); err != nil {
			d.logger.Error("save correction event", zap.Error(err))
		}
	}

	switch decision.Action {
	case CorrectionActionEscalate:
		_ = d.escalate(ctx, result.Owner, result.Repo, decision.IssueNumber, string(decision.FailureClass), decision.Reason)
		d.logger.Warn("issue escalated to human",
			zap.Int("issue", decision.IssueNumber),
			zap.String("reason", decision.Reason),
		)

	case CorrectionActionPause:
		_ = d.escalate(ctx, result.Owner, result.Repo, decision.IssueNumber, "budget_exhausted", decision.Reason)
		d.logger.Warn("dispatching paused due to budget",
			zap.Int("issue", decision.IssueNumber),
		)

	case CorrectionActionSwitchProvider, CorrectionActionRouteHigher,
		CorrectionActionIncreaseTimeout, CorrectionActionRetryWithContext,
		CorrectionActionRetry:
		tracker := d.trackerFor(result.Owner, result.Repo)
		if tracker != nil {
			if err := tracker.AddLabel(ctx, result.IssueNumber, "status:queued"); err != nil {
				d.logger.Error("add queued label for retry",
					zap.Int("issue", result.IssueNumber),
					zap.Error(err),
				)
			}
		}
		d.logger.Info("issue re-queued with correction",
			zap.Int("issue", decision.IssueNumber),
			zap.String("action", string(decision.Action)),
			zap.Time("backoff_until", decision.BackoffUntil),
		)
	}
}
