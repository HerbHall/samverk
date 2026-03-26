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
func checkOutputQuality(output string, agentType models.AgentType) QualityResult {
	output = strings.TrimSpace(output)

	// Empty or near-empty output.
	if len(output) < 50 {
		return QualityResult{Pass: false, Reason: "output too short", Score: 0.0}
	}

	// Truncation markers: only match patterns that strongly indicate forced
	// cutoff rather than natural language transitions. "I'll continue" and
	// "Let me continue" produce false positives on normal agent prose.
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

	// Code-gen specific: check for actual code content.
	if agentType == models.AgentTypeCodeGen || agentType == models.AgentTypeTest {
		hasCode := strings.Contains(output, "```") || strings.Contains(output, "EDIT:")
		if !hasCode {
			return QualityResult{Pass: false, Reason: "code agent produced no code blocks", Score: 0.2}
		}
	}

	return QualityResult{Pass: true, Reason: "quality check passed", Score: 1.0}
}

// checkCompletionQuality retrieves the session output from the store and
// runs the quality gate. On failure, posts a comment and adds a quality-fail
// label for visibility. On pass, logs success.
func (d *Dispatcher) checkCompletionQuality(ctx context.Context, result agent.TaskResult) {
	if d.store == nil || result.SessionID == "" {
		return
	}

	session, err := d.store.GetSession(ctx, result.SessionID)
	if err != nil {
		d.logger.Warn("quality gate: could not retrieve session",
			zap.Int("issue", result.IssueNumber),
			zap.String("session", result.SessionID),
			zap.Error(err),
		)
		return
	}

	qr := checkOutputQuality(session.PartialOutput, result.AgentType)
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
}
