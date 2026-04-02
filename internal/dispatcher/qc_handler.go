package dispatcher

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"samverk.dev/samverk/internal/agent"
	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/pkg/models"
)

// QCVerdict represents the outcome of a QC review.
type QCVerdict string

const (
	QCVerdictPass   QCVerdict = "PASS"
	QCVerdictFail   QCVerdict = "FAIL"
	QCVerdictReview QCVerdict = "REVIEW"
)

// QCResult holds the parsed result of a QC agent review.
type QCResult struct {
	Verdict      QCVerdict
	Summary      string   // full QC output for posting as a comment
	FailedItems  []string // specific items that failed (for correction feedback)
	IssueNumber  int      // source issue number
	PRNumber     int      // PR being reviewed
	ProviderUsed string   // which provider the QC agent used
}

// qcVerdictRe matches [PASS], [FAIL], or [REVIEW] in agent output.
// Accepts both "### Verdict: [PASS]" and standalone "[PASS]" formats.
var qcVerdictRe = regexp.MustCompile(`\[(PASS|FAIL|REVIEW)\]`)

// parseQCVerdict extracts the QC verdict from agent output.
// Returns the verdict and the full output as summary.
func parseQCVerdict(output string) QCResult {
	result := QCResult{
		Verdict: QCVerdictReview, // default to REVIEW if parsing fails
		Summary: output,
	}

	matches := qcVerdictRe.FindStringSubmatch(output)
	if len(matches) >= 2 {
		switch matches[1] {
		case "PASS":
			result.Verdict = QCVerdictPass
		case "FAIL":
			result.Verdict = QCVerdictFail
		case "REVIEW":
			result.Verdict = QCVerdictReview
		}
	}

	// Extract failed items from unchecked checkboxes.
	result.FailedItems = extractFailedItems(output)

	return result
}

// extractFailedItems finds unchecked checkbox items in QC output.
// These represent acceptance criteria or constraints that were not met.
func extractFailedItems(output string) []string {
	items := make([]string, 0, 4)
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ] ") {
			item := strings.TrimPrefix(trimmed, "- [ ] ")
			items = append(items, item)
		}
	}
	return items
}

// parseAcceptanceCriteria extracts acceptance criteria from an issue body.
// It looks for markdown checkboxes (- [ ] ...) under an "## Acceptance Criteria" heading.
// Falls back to finding any checkboxes in the body if no heading is found.
func parseAcceptanceCriteria(body string) []string {
	criteria := make([]string, 0, 8)

	lines := strings.Split(body, "\n")
	inSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect the acceptance criteria heading.
		if strings.HasPrefix(trimmed, "## Acceptance Criteria") ||
			strings.HasPrefix(trimmed, "## acceptance criteria") {
			inSection = true
			continue
		}

		// If we're in the section and hit another heading, stop.
		if inSection && strings.HasPrefix(trimmed, "## ") {
			break
		}

		// Collect checkboxes from within the section.
		if inSection && (strings.HasPrefix(trimmed, "- [ ] ") || strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ")) {
			// Strip the checkbox marker to get the criterion text.
			text := trimmed[6:] // len("- [ ] ") == 6
			if text != "" {
				criteria = append(criteria, text)
			}
		}
	}

	// Fallback: if no section heading found, scan the entire body for checkboxes.
	if len(criteria) == 0 {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- [ ] ") || strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
				text := trimmed[6:]
				if text != "" {
					criteria = append(criteria, text)
				}
			}
		}
	}

	return criteria
}

// spawnQCTask creates and dispatches a QC agent task for the given issue and PR.
// It uses a different provider than the generator to ensure cross-model validation.
func (d *Dispatcher) spawnQCTask(ctx context.Context, owner, repo string, issueNumber int, generatorProvider string) {
	tracker := d.trackerFor(owner, repo)
	if tracker == nil {
		d.logger.Error("spawnQCTask: no tracker", zap.String("owner", owner), zap.String("repo", repo))
		return
	}

	issue, err := tracker.GetIssue(ctx, issueNumber)
	if err != nil {
		d.logger.Error("spawnQCTask: get issue", zap.Int("issue", issueNumber), zap.Error(err))
		return
	}

	fm, _ := d.parseFrontmatter(issue)

	// Pre-QC doc enrichment: check .md files for staleness and contradictions.
	var docContext string
	if docFindings := d.checkDocFindings(ctx, owner, repo, fm); docFindings != nil {
		docContext = docFindings.RenderDocSection()
		for _, f := range docFindings.Findings {
			if label := docLabelFor(f.Category); label != "" {
				_ = tracker.AddLabels(ctx, issueNumber, label)
			}
		}
		d.logger.Info("doc enrichment: findings injected into QC context",
			zap.Int("issue", issueNumber),
			zap.Int("findings", len(docFindings.Findings)),
		)
	}

	// Build QC-specific task with cross-model provider selection.
	providerKey := d.selectQCProvider(issue, generatorProvider)

	timeout := EstimateTimeout(issue, fm, models.AgentTypeQC, providerKey)
	if d.store != nil {
		timeout = CalibratedTimeout(ctx, d.store, d.logger, issue, fm, models.AgentTypeQC, providerKey)
	}

	// Label issue as having QC in progress.
	_ = tracker.RemoveLabel(ctx, issueNumber, models.LabelStatusNeedsQc)

	if d.pool == nil || d.store == nil {
		// No pool available; fall back to labeling needs-qc for manual review.
		if labelErr := tracker.AddLabels(ctx, issueNumber, models.LabelStatusNeedsQc); labelErr != nil {
			d.logger.Error("spawnQCTask: fallback label", zap.Int("issue", issueNumber), zap.Error(labelErr))
		}
		return
	}

	sessionID := fmt.Sprintf("sess_qc_%d_%d", issueNumber, time.Now().Unix())
	session := &models.Session{
		ID:               sessionID,
		IssueNumber:      issueNumber,
		AgentType:        string(models.AgentTypeQC),
		Status:           models.SessionStatusPending,
		EstimatedTimeout: timeout,
		StartedAt:        time.Now(),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := d.store.CreateSession(ctx, session); err != nil {
		d.logger.Error("spawnQCTask: create session", zap.Int("issue", issueNumber), zap.Error(err))
		// Fall back to labeling needs-qc.
		_ = tracker.AddLabels(ctx, issueNumber, models.LabelStatusNeedsQc)
		return
	}

	key := issueKey(owner, repo, issueNumber)
	now := time.Now()
	d.mu.Lock()
	d.claimed[key] = &claimedIssue{
		AgentID:          string(models.AgentTypeQC),
		Owner:            owner,
		Repo:             repo,
		ClaimedAt:        now,
		LastHeartbeat:    now,
		EstimatedTimeout: timeout,
	}
	d.mu.Unlock()

	task := agent.Task{
		Issue:       issue,
		Tracker:     tracker,
		Owner:       owner,
		Repo:        repo,
		AgentType:   models.AgentTypeQC,
		SessionID:   sessionID,
		ProviderKey: providerKey,
		Timeout:     timeout,
		Frontmatter: fm,
		DocContext:  docContext,
		HeartbeatFunc: func() {
			d.mu.Lock()
			if c, ok := d.claimed[key]; ok {
				c.LastHeartbeat = time.Now()
			}
			d.mu.Unlock()
		},
	}

	if err := d.pool.Submit(task); err != nil {
		d.logger.Error("spawnQCTask: submit", zap.Int("issue", issueNumber), zap.Error(err))
		d.mu.Lock()
		delete(d.claimed, key)
		d.mu.Unlock()
		_ = tracker.AddLabels(ctx, issueNumber, models.LabelStatusNeedsQc)
		return
	}

	d.logger.Info("QC agent spawned",
		zap.Int("issue", issueNumber),
		zap.String("provider", providerKey),
		zap.String("generator_provider", generatorProvider),
		zap.String("session", sessionID),
	)
}

// selectQCProvider returns the provider key for QC, preferring a different
// provider than the generator for cross-model validation (ADR-030).
func (d *Dispatcher) selectQCProvider(issue *forge.Issue, generatorProvider string) string {
	// Start with the standard QC routing.
	providerKey, _ := selectProviderKey(issue, models.AgentTypeQC)

	// If the QC chain would use the same provider as the generator,
	// try alternative chains.
	if d.pool != nil && providerKey == generatorProvider {
		routing := d.pool.RegistryRouting()
		// Try "qc" chain first, then "complex", then "default".
		for _, altChain := range []string{"qc", "complex", "default"} {
			if chain, ok := routing[altChain]; ok && len(chain) > 0 {
				// Check if this chain has a different provider.
				for _, p := range chain {
					if p != generatorProvider {
						return altChain
					}
				}
			}
		}
	}

	// Fallback: use whatever the router gives us (some QC > no QC).
	return providerKey
}

// handleQCComplete processes the result of a QC agent task.
// It parses the verdict from the output and takes appropriate action.
func (d *Dispatcher) handleQCComplete(ctx context.Context, result agent.TaskResult) {
	tracker := d.trackerFor(result.Owner, result.Repo)
	if tracker == nil {
		d.logger.Error("handleQCComplete: no tracker", zap.String("owner", result.Owner), zap.String("repo", result.Repo))
		return
	}

	if !result.Success {
		// QC agent itself failed — fall back to manual review.
		d.logger.Warn("QC agent failed, falling back to manual review",
			zap.Int("issue", result.IssueNumber),
			zap.String("error", result.Error),
		)
		_ = tracker.AddLabels(ctx, result.IssueNumber, models.LabelStatusNeedsQc)
		return
	}

	// Retrieve the QC output from session.
	var output string
	if d.store != nil && result.SessionID != "" {
		session, err := d.store.GetSession(ctx, result.SessionID)
		if err == nil {
			output = session.PartialOutput
		}
	}

	// If no output from session, check comments.
	if output == "" {
		comments, err := tracker.ListComments(ctx, result.IssueNumber)
		if err == nil {
			for i := len(comments) - 1; i >= 0; i-- {
				if strings.Contains(comments[i].Body, "[PASS]") ||
					strings.Contains(comments[i].Body, "[FAIL]") ||
					strings.Contains(comments[i].Body, "[REVIEW]") {
					output = comments[i].Body
					break
				}
			}
		}
	}

	if output == "" {
		d.logger.Warn("QC agent produced no parseable output, escalating to manual review",
			zap.Int("issue", result.IssueNumber),
		)
		_ = tracker.AddLabels(ctx, result.IssueNumber, models.LabelStatusNeedsQc)
		return
	}

	qcResult := parseQCVerdict(output)
	qcResult.IssueNumber = result.IssueNumber
	qcResult.ProviderUsed = result.ProviderKey

	d.logger.Info("QC verdict",
		zap.Int("issue", result.IssueNumber),
		zap.String("verdict", string(qcResult.Verdict)),
		zap.String("provider", result.ProviderKey),
	)

	switch qcResult.Verdict {
	case QCVerdictPass:
		d.handleQCPass(ctx, result, tracker, qcResult)
	case QCVerdictFail:
		d.handleQCFail(ctx, result, tracker, qcResult)
	case QCVerdictReview:
		d.handleQCReview(ctx, result, tracker, qcResult)
	}
}

// handleQCPass processes a PASS verdict: post comment and label for auto-merge.
func (d *Dispatcher) handleQCPass(ctx context.Context, result agent.TaskResult, tracker forge.IssueTracker, qcResult QCResult) {
	// Find the PR associated with this issue.
	prNumber := d.findPRForIssue(ctx, result.IssueNumber, tracker)

	// Post QC pass comment on the PR (or issue if no PR found).
	commentTarget := result.IssueNumber
	if prNumber > 0 {
		commentTarget = prNumber
	}

	comment := fmt.Sprintf("**QC Review: [PASS]** (automated)\n\nProvider: `%s`\n\n%s",
		qcResult.ProviderUsed, truncateOutput(qcResult.Summary, 2000))
	if _, err := tracker.AddComment(ctx, commentTarget, comment); err != nil {
		d.logger.Error("handleQCPass: add comment", zap.Int("target", commentTarget), zap.Error(err))
	}

	// Remove needs-qc and add qc:pass label for PR watcher to pick up.
	_ = tracker.RemoveLabel(ctx, result.IssueNumber, models.LabelStatusNeedsQc)
	if err := tracker.AddLabels(ctx, result.IssueNumber, models.LabelQcPass); err != nil {
		d.logger.Warn("handleQCPass: add qc:pass label", zap.Int("issue", result.IssueNumber), zap.Error(err))
	}

	// Label the PR for auto-merge if it exists.
	if prNumber > 0 {
		_ = tracker.RemoveLabel(ctx, prNumber, models.LabelStatusNeedsQc)
	}

	d.recordPipelineEvent(ctx, result.Owner, result.Repo, result.IssueNumber, models.LabelStatusNeedsQc, "qc:pass", "qc-agent")

	broadcastEvent(d.broadcaster, "qc.pass", map[string]any{
		"issue_number": result.IssueNumber,
		"pr_number":    prNumber,
	})
}

// handleQCFail processes a FAIL verdict: post feedback and trigger correction engine.
func (d *Dispatcher) handleQCFail(ctx context.Context, result agent.TaskResult, tracker forge.IssueTracker, qcResult QCResult) {
	// Post QC failure comment on the issue.
	var failedItemsStr string
	if len(qcResult.FailedItems) > 0 {
		failedItemsStr = "\n\n**Failed items:**\n"
		for _, item := range qcResult.FailedItems {
			failedItemsStr += "- " + item + "\n"
		}
	}

	comment := fmt.Sprintf("**QC Review: [FAIL]** (automated)\n\nProvider: `%s`%s\n\n%s",
		qcResult.ProviderUsed, failedItemsStr, truncateOutput(qcResult.Summary, 2000))
	if _, err := tracker.AddComment(ctx, result.IssueNumber, comment); err != nil {
		d.logger.Error("handleQCFail: add comment", zap.Int("issue", result.IssueNumber), zap.Error(err))
	}

	// Remove needs-qc label and add qc:fail for decision surface.
	_ = tracker.RemoveLabel(ctx, result.IssueNumber, models.LabelStatusNeedsQc)
	if err := tracker.AddLabels(ctx, result.IssueNumber, models.LabelQcFail); err != nil {
		d.logger.Warn("handleQCFail: add qc:fail label", zap.Int("issue", result.IssueNumber), zap.Error(err))
	}

	// Build structured feedback for the correction engine.
	feedback := "QC review failed."
	if len(qcResult.FailedItems) > 0 {
		feedback = "QC review failed: " + strings.Join(qcResult.FailedItems, "; ")
	}

	// Record as a post-process failure to trigger retry via correction engine.
	d.recordFailure(ctx, result.IssueNumber, result.SessionID,
		string(models.AgentTypeQC), result.ProviderKey, feedback, 0)

	attempt := d.getPersistedFailureCount(ctx, result.IssueNumber)
	decision := decideCorrection(models.FailureClassPostProcess, result.IssueNumber, attempt, 0)
	decision.Reason = feedback
	d.applyCorrection(ctx, result, decision)

	d.recordPipelineEvent(ctx, result.Owner, result.Repo, result.IssueNumber, models.LabelStatusNeedsQc, "qc:fail", "qc-agent")

	broadcastEvent(d.broadcaster, "qc.fail", map[string]any{
		"issue_number": result.IssueNumber,
		"failed_items": qcResult.FailedItems,
	})
}

// handleQCReview processes a REVIEW verdict: escalate to human.
func (d *Dispatcher) handleQCReview(ctx context.Context, result agent.TaskResult, tracker forge.IssueTracker, qcResult QCResult) {
	comment := fmt.Sprintf("**QC Review: [REVIEW]** (automated) — requires human judgment\n\nProvider: `%s`\n\n%s",
		qcResult.ProviderUsed, truncateOutput(qcResult.Summary, 2000))
	if _, err := tracker.AddComment(ctx, result.IssueNumber, comment); err != nil {
		d.logger.Error("handleQCReview: add comment", zap.Int("issue", result.IssueNumber), zap.Error(err))
	}

	_ = tracker.RemoveLabel(ctx, result.IssueNumber, models.LabelStatusNeedsQc)
	_ = tracker.AddLabels(ctx, result.IssueNumber, models.LabelStatusNeedsHuman, models.LabelQcReview)

	d.recordPipelineEvent(ctx, result.Owner, result.Repo, result.IssueNumber, models.LabelStatusNeedsQc, models.LabelStatusNeedsHuman, "qc-agent")

	broadcastEvent(d.broadcaster, "qc.review", map[string]any{
		"issue_number": result.IssueNumber,
	})
}

// findPRForIssue looks for an open PR that references the given issue number.
// Returns 0 if no PR is found.
func (d *Dispatcher) findPRForIssue(ctx context.Context, issueNumber int, tracker forge.IssueTracker) int {
	// Search for PRs by checking comments or branch naming convention.
	// The standard pattern is "Closes #N" in PR body.
	issues, err := tracker.ListIssues(ctx, &forge.ListOptions{
		State:   forge.StateOpen,
		PerPage: 50,
	})
	if err != nil {
		return 0
	}

	// Look for PRs (issues that are pull requests) referencing this issue.
	closesPattern := fmt.Sprintf("#%d", issueNumber)
	for _, issue := range issues {
		if issue.IsPullRequest && strings.Contains(issue.Body, closesPattern) {
			return issue.Number
		}
	}

	return 0
}

// truncateOutput truncates text to maxLen, appending "..." if truncated.
func truncateOutput(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}
