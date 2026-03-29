package dispatcher

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/agent"
	"github.com/herbhall/samverk/pkg/models"
)

// QualityResult describes the outcome of a post-completion quality check.
type QualityResult struct {
	Pass   bool
	Reason string
	Score  float64 // 0.0 - 1.0
}

// checkOutputQuality evaluates agent output for basic quality signals.
// Returns a QualityResult indicating if the output meets minimum standards.
// maxTurnsHit and turnsUsed are optional signals from the provider; when maxTurnsHit
// is true, it indicates actual truncation (not string matching false positive).
func checkOutputQuality(output string, agentType models.AgentType, maxTurnsHit bool, turnsUsed int) QualityResult {
	output = strings.TrimSpace(output)

	// Empty or near-empty output.
	if len(output) < 50 {
		return QualityResult{Pass: false, Reason: "output too short", Score: 0.0}
	}

	// Primary signal: MaxTurnsHit from provider indicates actual truncation at the
	// provider level (stream-json is_error signal for CLI providers).
	if maxTurnsHit {
		reason := "output truncated: max turns hit"
		if turnsUsed > 0 {
			reason = fmt.Sprintf("output truncated: max turns hit (%d turns used)", turnsUsed)
		}
		return QualityResult{Pass: false, Reason: reason, Score: 0.3}
	}

	// Secondary fallback: string matching for truncation markers. This detects
	// agents that ran out of space on non-CLI providers or wrote the marker naturally.
	// Only match patterns that strongly indicate forced cutoff rather than natural
	// language transitions. "I'll continue" and "Let me continue" produce false
	// positives on normal agent prose.
	truncationMarkers := []string{
		"...truncated",
		"output limit reached",
		"maximum number of turns",
		"reached the turn limit",
	}
	for _, marker := range truncationMarkers {
		if strings.Contains(strings.ToLower(output), strings.ToLower(marker)) {
			return QualityResult{Pass: false, Reason: "output appears truncated: " + marker, Score: 0.3}
		}
	}

	// Code-gen specific: check for actual code content using the shared
	// format constants (agent.HasCodeOutput). This ensures the quality gate
	// matches the same format the edit block parser accepts.
	if agentType == models.AgentTypeCodeGen || agentType == models.AgentTypeTest {
		if !agent.HasCodeOutput(output) {
			return QualityResult{Pass: false, Reason: "code agent produced no code blocks", Score: 0.2}
		}
	}

	return QualityResult{Pass: true, Reason: "quality check passed", Score: 1.0}
}

// checkCompletionQuality retrieves the session output from the store and
// runs the quality gate. Returns the QualityResult. On failure, posts a comment
// for visibility.
func (d *Dispatcher) checkCompletionQuality(ctx context.Context, result agent.TaskResult) QualityResult {
	if d.store == nil || result.SessionID == "" {
		return QualityResult{Pass: true, Reason: "no store or session", Score: 1.0}
	}

	session, err := d.store.GetSession(ctx, result.SessionID)
	if err != nil {
		d.logger.Warn("quality gate: could not retrieve session",
			zap.Int("issue", result.IssueNumber),
			zap.String("session", result.SessionID),
			zap.Error(err),
		)
		return QualityResult{Pass: true, Reason: "could not retrieve session", Score: 1.0}
	}

	qr := checkOutputQuality(session.PartialOutput, result.AgentType, session.MaxTurnsHit, session.TurnsUsed)
	if !qr.Pass {
		d.logger.Warn("quality gate failed",
			zap.Int("issue", result.IssueNumber),
			zap.String("reason", qr.Reason),
			zap.Float64("score", qr.Score),
			zap.String("provider", result.ProviderKey),
		)
		// Post quality failure as issue comment for visibility.
		tracker := d.trackerFor(result.Owner, result.Repo)
		if tracker != nil {
			comment := fmt.Sprintf("**Quality gate failed** (score: %.1f): %s\n\nProvider: `%s` | Session: `%s`",
				qr.Score, qr.Reason, result.ProviderKey, result.SessionID)
			if _, commentErr := tracker.AddComment(ctx, result.IssueNumber, comment); commentErr != nil {
				d.logger.Warn("quality gate: failed to post comment", zap.Error(commentErr))
			}
		}
	}
	return qr
}
