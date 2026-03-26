// Package prwatcher monitors open pull requests and auto-merges eligible ones.
//
// Eligibility requires: not draft, trusted author, no excluded labels,
// all CI checks passed, and mergeable. The watcher runs concurrently with
// the dispatcher via errgroup.
//
// When trusted reviewers leave unresolved review comments on an eligible PR,
// the watcher creates a batched remediation issue for the dispatcher to handle.
package prwatcher

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// Watcher polls open PRs and auto-merges eligible ones.
type Watcher struct {
	prManager    forge.PullRequestManager
	issueTracker forge.IssueTracker
	mergeCfg     autonomy.MergeConfig
	interval     time.Duration
	logger       *zap.Logger
}

// New creates a Watcher with the given PR manager, issue tracker, and merge policy.
// The issue tracker is used to create remediation issues for PRs with blocking
// review comments. It may be nil to disable remediation.
func New(pm forge.PullRequestManager, issues forge.IssueTracker, cfg autonomy.MergeConfig, interval time.Duration, logger *zap.Logger) *Watcher {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Watcher{
		prManager:    pm,
		issueTracker: issues,
		mergeCfg:     cfg,
		interval:     interval,
		logger:       logger,
	}
}

// Run polls open PRs at the configured interval until the context is cancelled.
func (w *Watcher) Run(ctx context.Context) error {
	if !w.mergeCfg.AutoMergeOnCIPass {
		w.logger.Info("pr-watcher: auto-merge disabled, exiting")
		return nil
	}

	w.logger.Info("pr-watcher: starting", zap.Duration("interval", w.interval))

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.poll(ctx); err != nil {
				w.logger.Error("pr-watcher: poll error", zap.Error(err))
			}
		}
	}
}

// poll lists open PRs and attempts to merge eligible ones based on tier policy.
//
// Tier 1 (docs, config): merge immediately on CI green.
// Tier 2 (features, fixes): merge after delay (Tier2DelayMinutes, default 60).
// Tier 3 (architecture, breaking): never auto-merge; label needs-human.
func (w *Watcher) poll(ctx context.Context) error {
	prs, err := w.prManager.ListPullRequests(ctx, &forge.ListPROptions{
		State:   forge.StateOpen,
		PerPage: 50,
	})
	if err != nil {
		return fmt.Errorf("list open PRs: %w", err)
	}

	tier2Delay := time.Duration(w.mergeCfg.Tier2DelayMinutes) * time.Minute
	if tier2Delay <= 0 {
		tier2Delay = 60 * time.Minute // default 1 hour
	}

	var eligible, skippedAuthor, skippedCI, skippedDelay, skippedBlocking, merged, tier3 int

	for _, pr := range prs {
		if !w.isEligible(pr) {
			if !w.isTrustedAuthor(pr.Author) {
				skippedAuthor++
				w.logger.Debug("pr-watcher: skip untrusted author",
					zap.Int("pr", pr.Number), zap.String("author", pr.Author))
			}
			continue
		}
		eligible++

		// Check for blocking review comments and create remediation issues.
		if w.issueTracker != nil {
			hasBlocking, checkErr := w.checkReviewComments(ctx, pr)
			if checkErr != nil {
				w.logger.Error("pr-watcher: check review comments", zap.Int("pr", pr.Number), zap.Error(checkErr))
			}
			if hasBlocking {
				skippedBlocking++
				continue
			}
		}

		checks, checkErr := w.prManager.GetPRChecks(ctx, pr.Number)
		if checkErr != nil {
			w.logger.Error("pr-watcher: get checks", zap.Int("pr", pr.Number), zap.Error(checkErr))
			continue
		}

		ciPassed := w.allChecksPassed(checks)
		tier := ClassifyPRTier(pr, nil)

		switch tier {
		case PRTier3:
			tier3++
			w.labelTier3(ctx, pr)
			continue
		case PRTier2:
			if !ciPassed {
				skippedCI++
				w.logger.Debug("pr-watcher: CI not passed",
					zap.Int("pr", pr.Number), zap.Int("checks", len(checks)), zap.String("tier", tier.String()))
				continue
			}
			if time.Since(pr.UpdatedAt) < tier2Delay {
				skippedDelay++
				w.logger.Debug("pr-watcher: tier-2 delay not elapsed",
					zap.Int("pr", pr.Number), zap.Duration("remaining", tier2Delay-time.Since(pr.UpdatedAt)))
				continue
			}
		case PRTier1:
			if !ciPassed {
				skippedCI++
				w.logger.Debug("pr-watcher: CI not passed",
					zap.Int("pr", pr.Number), zap.Int("checks", len(checks)), zap.String("tier", tier.String()))
				continue
			}
		default:
			if !ciPassed {
				skippedCI++
				continue
			}
		}

		w.logger.Info("pr-watcher: merging", zap.Int("pr", pr.Number), zap.String("title", pr.Title), zap.String("tier", tier.String()))
		commitMsg := fmt.Sprintf("auto-merge: %s (#%d)", pr.Title, pr.Number)
		if mergeErr := w.prManager.MergePullRequest(ctx, pr.Number, forge.MergeMethodSquash, commitMsg); mergeErr != nil {
			w.logger.Error("pr-watcher: merge failed", zap.Int("pr", pr.Number), zap.Error(mergeErr))
			continue
		}
		merged++
		if w.issueTracker != nil {
			issueNums := w.closeLinkedIssues(ctx, pr)
			// Unblock dependent issues for each closed issue.
			for _, issueNum := range issueNums {
				if err := w.unblockDependents(ctx, issueNum); err != nil {
					w.log().Warn("pr-watcher: unblock dependents",
						zap.Int("issue", issueNum), zap.Error(err))
				}
			}
		}
	}

	w.logger.Info("pr-watcher: poll complete",
		zap.Int("open_prs", len(prs)),
		zap.Int("eligible", eligible),
		zap.Int("merged", merged),
		zap.Int("ci_pending", skippedCI),
		zap.Int("delay_pending", skippedDelay),
		zap.Int("blocked_review", skippedBlocking),
		zap.Int("untrusted_author", skippedAuthor),
		zap.Int("tier3_human", tier3),
	)

	return nil
}

// labelTier3 adds the status:needs-human label to a Tier 3 PR if not already present.
func (w *Watcher) labelTier3(ctx context.Context, pr *forge.PullRequest) {
	for _, l := range pr.Labels {
		if l == models.LabelStatusNeedsHuman {
			return
		}
	}
	if w.issueTracker != nil {
		if err := w.issueTracker.AddLabel(ctx, pr.Number, models.LabelStatusNeedsHuman); err != nil {
			w.logger.Error("pr-watcher: label tier-3 PR", zap.Int("pr", pr.Number), zap.Error(err))
		}
	}
}

// checkReviewComments checks for unresolved review comments from trusted reviewers
// and Copilot-authored comments. Actionable comments (suggestions, bug reports,
// security warnings) block merge; approval-like comments ("looks good") do not.
// It returns true if blocking comments exist (remediation issue created or already exists).
func (w *Watcher) checkReviewComments(ctx context.Context, pr *forge.PullRequest) (bool, error) {
	// Skip if PR already has status:needs-human label.
	for _, l := range pr.Labels {
		if l == models.LabelStatusNeedsHuman {
			return false, nil
		}
	}

	comments, err := w.prManager.ListReviewComments(ctx, pr.Number)
	if err != nil {
		return false, fmt.Errorf("list review comments for PR #%d: %w", pr.Number, err)
	}

	// Filter for unresolved actionable comments from trusted reviewers or Copilot.
	blocking := make([]forge.ReviewComment, 0, len(comments))
	for i := range comments {
		c := comments[i]
		if c.Resolved {
			continue
		}
		if !w.isTrustedReviewer(c.Author) && !isCopilotAuthor(c.Author) {
			continue
		}
		if !isActionableComment(c.Body) {
			continue
		}
		blocking = append(blocking, c)
	}

	if len(blocking) == 0 {
		return false, nil
	}

	// Check if a remediation issue already exists for this PR.
	prLabel := fmt.Sprintf("pr:%d", pr.Number)
	existing, err := w.issueTracker.ListIssues(ctx, &forge.ListOptions{
		State:  forge.StateOpen,
		Labels: []string{prLabel},
	})
	if err != nil {
		return true, fmt.Errorf("check existing remediation issues: %w", err)
	}
	if len(existing) > 0 {
		return true, nil
	}

	// Create a batched remediation issue.
	title := fmt.Sprintf("fix(#%d): address Copilot review feedback on %q", pr.Number, pr.Title)
	body := buildRemediationBody(pr, blocking)

	created, err := w.issueTracker.CreateIssue(ctx, &forge.CreateIssueRequest{
		Title:  title,
		Body:   body,
		Labels: []string{models.LabelAgentCodeGen, models.LabelPriorityHigh, prLabel},
	})
	if err != nil {
		return true, fmt.Errorf("create remediation issue for PR #%d: %w", pr.Number, err)
	}

	// Comment on the PR to link the remediation issue.
	prComment := fmt.Sprintf("Copilot feedback detected. Created issue #%d to address before merge.", created.Number)
	if _, commentErr := w.issueTracker.AddComment(ctx, pr.Number, prComment); commentErr != nil {
		w.log().Error("pr-watcher: add PR comment", zap.Int("pr", pr.Number), zap.Error(commentErr))
	}

	w.log().Info("pr-watcher: created remediation issue",
		zap.Int("pr", pr.Number),
		zap.Int("issue", created.Number),
		zap.Int("comments", len(blocking)))
	return true, nil
}

// log returns the watcher's logger, falling back to zap.NewNop() if nil.
// Allows direct struct construction in tests without setting a logger.
func (w *Watcher) log() *zap.Logger {
	if w.logger != nil {
		return w.logger
	}
	return zap.NewNop()
}

// isTrustedReviewer checks if the author is in the trusted reviewers list.
// An empty list means no reviewers are trusted (remediation disabled).
func (w *Watcher) isTrustedReviewer(author string) bool {
	for _, r := range w.mergeCfg.TrustedReviewers {
		if r == author {
			return true
		}
	}
	return false
}

// isCopilotAuthor returns true if the comment author is GitHub Copilot.
// Copilot reviews appear as "copilot" or as "github-actions[bot]" with
// copilot context, or variations containing "copilot" in the login.
func isCopilotAuthor(author string) bool {
	lower := strings.ToLower(author)
	return strings.Contains(lower, "copilot")
}

// approvalPhrases are comment bodies that indicate approval, not actionable feedback.
var approvalPhrases = []string{
	"looks good",
	"lgtm",
	"no issues found",
	"no concerns",
	"approved",
	"ship it",
	"good to go",
	"well done",
	"nice work",
	"no suggestions",
}

// isActionableComment returns true if the comment body contains actionable
// feedback (suggestions, bug reports, security warnings). Approval-like
// comments ("looks good", "lgtm") are not actionable.
func isActionableComment(body string) bool {
	if strings.TrimSpace(body) == "" {
		return false
	}
	lower := strings.ToLower(body)
	for _, phrase := range approvalPhrases {
		if strings.Contains(lower, phrase) {
			return false
		}
	}
	return true
}

// buildRemediationBody constructs the issue body for a remediation issue.
func buildRemediationBody(pr *forge.PullRequest, comments []forge.ReviewComment) string {
	var b strings.Builder

	fmt.Fprintf(&b, "PR #%d has unresolved review comments that must be addressed before merge.\n\n", pr.Number)
	b.WriteString("## Review Comments\n\n")

	for i, c := range comments {
		fmt.Fprintf(&b, "### Comment %d\n", i+1)
		fmt.Fprintf(&b, "**File:** `%s`", c.Path)
		if c.StartLine > 0 && c.StartLine == c.EndLine {
			fmt.Fprintf(&b, " (line %d)", c.EndLine)
		} else if c.StartLine > 0 && c.EndLine > 0 {
			fmt.Fprintf(&b, " (lines %d\u2013%d)", c.StartLine, c.EndLine)
		}
		fmt.Fprintf(&b, "\n\n%s\n\n", c.Body)
	}

	b.WriteString("## Instructions\n\n")
	fmt.Fprintf(&b, "Read each comment, implement the fix, and push to branch `%s`.\n", pr.Head)
	b.WriteString("Do not open a new PR — push directly to the existing branch.\n")
	fmt.Fprintf(&b, "When all comments are addressed, add the label `review:addressed` to PR #%d.\n", pr.Number)

	return b.String()
}

// linkedIssueRe matches "Closes #N", "Fixes #N", "Resolves #N" (case-insensitive)
// and "(#N)" anywhere in title or body.
var linkedIssueRe = regexp.MustCompile(`(?i)(?:closes|fixes|resolves)\s+#(\d+)|\(#(\d+)\)`)

// parseLinkedIssues extracts issue numbers from PR title and body.
// It matches: "Closes #N", "Fixes #N", "Resolves #N" in body (case-insensitive),
// and "(#N)" anywhere in title or body. Duplicates are removed.
func parseLinkedIssues(pr *forge.PullRequest) []int {
	text := pr.Title + "\n" + pr.Body
	matches := linkedIssueRe.FindAllStringSubmatch(text, -1)

	seen := make(map[int]bool)
	result := make([]int, 0, len(matches))
	for _, m := range matches {
		// m[1] is the keyword-style match, m[2] is the (#N) style match.
		raw := m[1]
		if raw == "" {
			raw = m[2]
		}
		n := 0
		for _, ch := range raw {
			n = n*10 + int(ch-'0')
		}
		if n > 0 && !seen[n] {
			seen[n] = true
			result = append(result, n)
		}
	}
	return result
}

// closeLinkedIssues closes all issues referenced in the PR title or body and
// replaces their status labels with "status:done". Returns the list of successfully
// closed issue numbers.
func (w *Watcher) closeLinkedIssues(ctx context.Context, pr *forge.PullRequest) []int {
	issueNums := parseLinkedIssues(pr)
	closed := make([]int, 0, len(issueNums))
	for _, num := range issueNums {
		closedState := forge.StateClosed
		_, err := w.issueTracker.UpdateIssue(ctx, num, &forge.UpdateIssueRequest{State: &closedState})
		if err != nil {
			w.log().Warn("prwatcher: failed to close linked issue",
				zap.Int("issue", num), zap.Int("pr", pr.Number), zap.Error(err))
			continue
		}
		w.log().Info("prwatcher: closed linked issue",
			zap.Int("issue", num), zap.Int("pr", pr.Number))
		if err = w.issueTracker.SetLabels(ctx, num, []string{models.LabelStatusDone}); err != nil {
			w.log().Warn("prwatcher: failed to set status:done on linked issue",
				zap.Int("issue", num), zap.Error(err))
		}
		closed = append(closed, num)
	}
	return closed
}

// isEligible checks static PR attributes against merge policy.
func (w *Watcher) isEligible(pr *forge.PullRequest) bool {
	if pr.Draft {
		return false
	}

	if !pr.Mergeable {
		return false
	}

	if !w.isTrustedAuthor(pr.Author) {
		return false
	}

	if w.hasExcludedLabel(pr.Labels) {
		return false
	}

	return true
}

// isTrustedAuthor checks if the PR author is in the trusted list.
// An empty trusted list means all authors are trusted.
func (w *Watcher) isTrustedAuthor(author string) bool {
	if len(w.mergeCfg.TrustedAuthors) == 0 {
		return true
	}
	for _, a := range w.mergeCfg.TrustedAuthors {
		if a == author {
			return true
		}
	}
	return false
}

// hasExcludedLabel checks if any PR label is in the exclude list.
func (w *Watcher) hasExcludedLabel(labels []string) bool {
	for _, l := range labels {
		for _, excl := range w.mergeCfg.ExcludeLabels {
			if l == excl {
				return true
			}
		}
	}
	return false
}

// allChecksPassed returns true if all checks have succeeded.
// An empty check list is treated as pending (not passed).
func (w *Watcher) allChecksPassed(checks []forge.Check) bool {
	if len(checks) == 0 {
		return false
	}

	for _, c := range checks {
		if c.Status != forge.CheckStatusSuccess {
			return false
		}
	}

	return true
}

// unblockDependents scans blocked issues and transitions any whose dependencies
// are all satisfied after closedIssueNumber was closed.
// It removes status:blocked and adds status:queued for unblocked issues.
func (w *Watcher) unblockDependents(ctx context.Context, closedIssueNumber int) error {
	if w.issueTracker == nil {
		return nil
	}

	blockedIssues, err := w.issueTracker.ListIssues(ctx, &forge.ListOptions{
		State:   forge.StateOpen,
		Labels:  []string{models.LabelStatusBlocked},
		PerPage: 100,
	})
	if err != nil {
		return fmt.Errorf("list blocked issues: %w", err)
	}

	for _, issue := range blockedIssues {
		result, parseErr := models.ParseFrontmatter(issue.Body)
		if parseErr != nil || result.Frontmatter == nil {
			continue
		}

		// Check if this issue depends on the one that just closed.
		dependsOnClosed := false
		for _, dep := range result.Frontmatter.DependsOn {
			// Only consider local (same-repo) dependencies.
			if !dep.IsCrossProject() && dep.Number == closedIssueNumber {
				dependsOnClosed = true
				break
			}
		}
		if !dependsOnClosed {
			continue
		}

		// Check if ALL dependencies are now satisfied.
		if blocked := w.checkAllDependenciesSatisfied(ctx, result.Frontmatter); blocked {
			continue
		}

		// Unblock: remove status:blocked and add status:queued.
		if removeErr := w.issueTracker.RemoveLabel(ctx, issue.Number, models.LabelStatusBlocked); removeErr != nil {
			w.log().Warn("pr-watcher: remove status:blocked",
				zap.Int("issue", issue.Number), zap.Error(removeErr))
		}
		if addErr := w.issueTracker.AddLabel(ctx, issue.Number, models.LabelStatusQueued); addErr != nil {
			w.log().Warn("pr-watcher: add status:queued",
				zap.Int("issue", issue.Number), zap.Error(addErr))
		}

		comment := fmt.Sprintf(
			"UNBLOCKED [prwatcher] [%s]\nDependency #%d completed. All dependencies satisfied.\nTransitioning to status:queued for routing.",
			time.Now().UTC().Format(time.RFC3339), closedIssueNumber,
		)
		if _, commentErr := w.issueTracker.AddComment(ctx, issue.Number, comment); commentErr != nil {
			w.log().Warn("pr-watcher: add comment",
				zap.Int("issue", issue.Number), zap.Error(commentErr))
		}

		w.log().Info("pr-watcher: unblocked issue",
			zap.Int("issue", issue.Number), zap.Int("closed_dep", closedIssueNumber))
	}

	return nil
}

// checkAllDependenciesSatisfied verifies that all dependencies in the frontmatter
// are satisfied (closed with status:done label). Only checks local dependencies.
func (w *Watcher) checkAllDependenciesSatisfied(ctx context.Context, fm *models.IssueFrontmatter) bool {
	if fm == nil || len(fm.DependsOn) == 0 {
		return false // No dependencies to check.
	}

	for _, dep := range fm.DependsOn {
		// Skip cross-project dependencies in this context.
		if dep.IsCrossProject() {
			continue
		}

		issue, err := w.issueTracker.GetIssue(ctx, dep.Number)
		if err != nil {
			w.log().Debug("pr-watcher: check dependency",
				zap.Int("dep", dep.Number), zap.Error(err))
			return true // If we can't check, assume blocked to be safe.
		}

		// Check if dependency is done (closed with status:done).
		if issue.State != forge.StateClosed {
			return true // Not done, still blocked.
		}

		hasDoneLabel := false
		for _, label := range issue.Labels {
			if label == models.LabelStatusDone {
				hasDoneLabel = true
				break
			}
		}
		if !hasDoneLabel {
			return true // No status:done label, still blocked.
		}
	}

	// All dependencies satisfied.
	return false
}
