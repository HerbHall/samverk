package dispatcher

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"samverk.dev/samverk/internal/agent"
	"samverk.dev/samverk/pkg/models"
)

// extractAcceptanceCriteria parses unchecked task items from the
// "## Acceptance Criteria" section of an issue body.
// Only `- [ ]` items are returned (checked items are already done).
func extractAcceptanceCriteria(issueBody string) []string {
	criteria := make([]string, 0)
	inSection := false
	for _, line := range strings.Split(issueBody, "\n") {
		trimmed := strings.TrimSpace(line)
		// Detect section header (case-insensitive).
		if strings.HasPrefix(strings.ToLower(trimmed), "## acceptance criteria") {
			inSection = true
			continue
		}
		// Stop at next ## heading.
		if inSection && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if inSection && strings.HasPrefix(trimmed, "- [ ]") {
			text := strings.TrimSpace(strings.TrimPrefix(trimmed, "- [ ]"))
			if text != "" {
				criteria = append(criteria, text)
			}
		}
	}
	return criteria
}

// criterionKeywords extracts meaningful words from a criterion string.
// Words of length <= 3 are considered stopwords and skipped.
func criterionKeywords(criterion string) []string {
	words := strings.Fields(criterion)
	keywords := make([]string, 0, len(words))
	for _, w := range words {
		clean := strings.ToLower(strings.Trim(w, ".,!?:;\"'`()[]{}"))
		if len(clean) > 3 {
			keywords = append(keywords, clean)
		}
	}
	return keywords
}

// checkCriteriaCoverage checks how many criteria are mentioned in the output.
// A criterion is considered covered when at least one of its keywords appears
// (case-insensitive substring match) in the output.
// Returns the number covered and the list of uncovered criteria.
func checkCriteriaCoverage(criteria []string, output string) (covered int, missing []string) {
	lowerOutput := strings.ToLower(output)
	missing = make([]string, 0, len(criteria))
	for _, c := range criteria {
		keywords := criterionKeywords(c)
		found := false
		for _, kw := range keywords {
			if strings.Contains(lowerOutput, kw) {
				found = true
				break
			}
		}
		if found {
			covered++
		} else {
			missing = append(missing, c)
		}
	}
	return covered, missing
}

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
// for visibility. Also performs an advisory acceptance-criteria coverage check
// (never blocks the quality gate).
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
	tracker := d.trackerFor(result.Owner, result.Repo)

	if !qr.Pass {
		d.logger.Warn("quality gate failed",
			zap.Int("issue", result.IssueNumber),
			zap.String("reason", qr.Reason),
			zap.Float64("score", qr.Score),
			zap.String("provider", result.ProviderKey),
		)
		// Post quality failure as issue comment for visibility.
		if tracker != nil {
			comment := fmt.Sprintf("**Quality gate failed** (score: %.1f): %s\n\nProvider: `%s` | Session: `%s`",
				qr.Score, qr.Reason, result.ProviderKey, result.SessionID)
			if _, commentErr := tracker.AddComment(ctx, result.IssueNumber, comment); commentErr != nil {
				d.logger.Warn("quality gate: failed to post comment", zap.Error(commentErr))
			}
		}
	}

	// Advisory: check acceptance criteria coverage. This never blocks the
	// quality gate — low coverage is logged and commented for visibility only.
	if tracker != nil && result.IssueNumber > 0 {
		issue, issueErr := tracker.GetIssue(ctx, result.IssueNumber)
		if issueErr == nil && issue != nil {
			criteria := extractAcceptanceCriteria(issue.Body)
			if len(criteria) > 0 {
				covered, missing := checkCriteriaCoverage(criteria, session.PartialOutput)
				total := len(criteria)
				coveragePct := float64(covered) / float64(total)
				d.logger.Info("quality gate: acceptance criteria coverage",
					zap.Int("issue", result.IssueNumber),
					zap.Int("covered", covered),
					zap.Int("total", total),
					zap.Float64("coverage", coveragePct),
				)
				if coveragePct < 0.5 {
					d.logger.Warn("quality gate: low acceptance criteria coverage",
						zap.Int("issue", result.IssueNumber),
						zap.Int("covered", covered),
						zap.Int("total", total),
					)
					coverageComment := fmt.Sprintf(
						"Acceptance criteria coverage: %d/%d (%.0f%%). Missing: %s",
						covered, total, coveragePct*100,
						strings.Join(missing, ", "),
					)
					if _, commentErr := tracker.AddComment(ctx, result.IssueNumber, coverageComment); commentErr != nil {
						d.logger.Warn("quality gate: failed to post coverage comment", zap.Error(commentErr))
					}
				}
			}
		}
	}

	return qr
}
