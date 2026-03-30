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
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
)

// IssueCacheReader provides read-only access to cached issue data and failure counts.
// The store.SQLiteStore satisfies this interface directly.
type IssueCacheReader interface {
	GetCachedIssue(ctx context.Context, project string, number int) (*store.CachedIssue, error)
	GetIssueFailureCount(ctx context.Context, issueNumber int) (int, error)
}

// Watcher polls open PRs and auto-merges eligible ones.
type Watcher struct {
	prManager    forge.PullRequestManager
	issueTracker forge.IssueTracker
	issueCache   IssueCacheReader
	project      string // project key for issue cache lookups
	mergeCfg     autonomy.MergeConfig
	interval     time.Duration
	logger       *zap.Logger
}

// New creates a Watcher with the given PR manager, issue tracker, and merge policy.
// The issue tracker is used to create remediation issues for PRs with blocking
// review comments. It may be nil to disable remediation.
// The issueCache provides label reads from the cached issue data (avoids ListComments).
func New(pm forge.PullRequestManager, issues forge.IssueTracker, cfg autonomy.MergeConfig, interval time.Duration, logger *zap.Logger, opts ...Option) *Watcher {
	if logger == nil {
		logger = zap.NewNop()
	}
	w := &Watcher{
		prManager:    pm,
		issueTracker: issues,
		mergeCfg:     cfg,
		interval:     interval,
		logger:       logger,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Option configures a Watcher.
type Option func(*Watcher)

// WithIssueCache sets the issue cache reader for label-based checks.
func WithIssueCache(cache IssueCacheReader, project string) Option {
	return func(w *Watcher) {
		w.issueCache = cache
		w.project = project
	}
}

// Run polls open PRs at the configured interval until the context is cancelled.
func (w *Watcher) Run(ctx context.Context) error {
	if !w.mergeCfg.AutoMergeOnCIPass {
		w.logger.Info("pr-watcher: auto-merge disabled, exiting")
		return nil
	}

	w.logger.Info("pr-watcher: started",
		zap.Duration("poll_interval", w.interval),
		zap.Strings("trusted_authors", w.mergeCfg.TrustedAuthors),
	)

	// Execute first poll immediately so we don't wait a full interval.
	if err := w.poll(ctx); err != nil {
		w.logger.Error("pr-watcher: first poll error", zap.Error(err))
	} else {
		w.logger.Info("pr-watcher: first poll complete")
	}

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

	var eligible, skippedAuthor, skippedCI, skippedDelay, skippedBlocking, merged, tier3, conflictClosed int

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

		// Handle stale CI-failed PRs before checking tier eligibility.
		// Only remediate if all checks are terminal (CI done running), at least one failed, and stale.
		if !ciPassed && w.allChecksTerminal(checks) && w.hasCIFailures(checks) && w.isStaleCI(pr, checks) {
			failedChecks := w.getFailedCheckNames(checks)
			if remediateErr := w.remediateStaleCIFailure(ctx, pr, failedChecks); remediateErr != nil {
				w.logger.Error("pr-watcher: remediate stale CI failure", zap.Int("pr", pr.Number), zap.Error(remediateErr))
			}
			skippedCI++
			continue
		}

		// Check for QC agent approval — a [PASS] verdict comment
		// allows Tier 2 PRs to skip the delay (QC is the verification).
		qcApproved := w.hasQCApproval(ctx, pr)

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
			// QC-approved PRs skip the tier-2 delay — the QC agent
			// has already verified the work.
			if !qcApproved && time.Since(pr.UpdatedAt) < tier2Delay {
				skippedDelay++
				w.logger.Debug("pr-watcher: tier-2 delay not elapsed (no QC approval)",
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

	// Handle non-mergeable stale PRs: close and re-queue source issues.
	for _, pr := range prs {
		// Skip draft, mergeable, or untrusted PRs.
		if pr.Draft || pr.Mergeable || !w.isTrustedAuthor(pr.Author) {
			continue
		}

		// Check if PR is stale (older than the CI failure threshold).
		if !w.isStaleNonMergeable(pr) {
			continue
		}

		// Close the PR and re-queue its source issue.
		if remediateErr := w.remediateNonMergeableConflict(ctx, pr); remediateErr != nil {
			w.logger.Error("pr-watcher: remediate non-mergeable conflict", zap.Int("pr", pr.Number), zap.Error(remediateErr))
			continue
		}
		conflictClosed++
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
		zap.Int("conflict_closed", conflictClosed),
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
		if err := w.issueTracker.AddLabels(ctx, pr.Number, models.LabelStatusNeedsHuman); err != nil {
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

// allChecksTerminal returns true only when every check is either success or failure.
// Returns false if any check is pending (CI still running).
func (w *Watcher) allChecksTerminal(checks []forge.Check) bool {
	if len(checks) == 0 {
		return false
	}

	for _, c := range checks {
		if c.Status == forge.CheckStatusPending {
			return false
		}
	}

	return true
}

// hasCIFailures returns true if at least one check has failed status.
// Pending checks are not considered failures.
func (w *Watcher) hasCIFailures(checks []forge.Check) bool {
	for _, c := range checks {
		if c.Status == forge.CheckStatusFailure {
			return true
		}
	}

	return false
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
		if addErr := w.issueTracker.AddLabels(ctx, issue.Number, models.LabelStatusQueued); addErr != nil {
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

// mostRecentCheckTime returns the timestamp of the most recent check,
// or the zero time if the check list is empty.
func (w *Watcher) mostRecentCheckTime(checks []forge.Check) time.Time {
	if len(checks) == 0 {
		return time.Time{}
	}

	latest := checks[0].CreatedAt
	for i := 1; i < len(checks); i++ {
		if checks[i].CreatedAt.After(latest) {
			latest = checks[i].CreatedAt
		}
	}

	return latest
}

// isStaleNonMergeable checks if a non-mergeable PR is older than the configured
// CI failure threshold. Returns true if the PR has not been updated for longer
// than the threshold, indicating it's stale due to merge conflicts.
func (w *Watcher) isStaleNonMergeable(pr *forge.PullRequest) bool {
	thresholdMinutes := w.mergeCfg.CIFailureThresholdMinutes
	if thresholdMinutes <= 0 {
		thresholdMinutes = 120 // default 2 hours
	}

	return time.Since(pr.UpdatedAt) > time.Duration(thresholdMinutes)*time.Minute
}

// isStaleCI checks if a PR's CI checks have been in failed state for longer than
// the configured threshold. It measures staleness from the most recent check
// status timestamp, not from PR activity (which can be reset by comments, labels, etc).
// Returns false if all checks are not terminal (CI still running).
func (w *Watcher) isStaleCI(pr *forge.PullRequest, checks []forge.Check) bool {
	// Don't remediate if CI is still running.
	if !w.allChecksTerminal(checks) {
		return false
	}

	thresholdMinutes := w.mergeCfg.CIFailureThresholdMinutes
	if thresholdMinutes <= 0 {
		thresholdMinutes = 120 // default 2 hours
	}

	mostRecent := w.mostRecentCheckTime(checks)
	if mostRecent.IsZero() {
		return false
	}

	return time.Since(mostRecent) > time.Duration(thresholdMinutes)*time.Minute
}

// getFailedCheckNames extracts the names of checks with failure status.
// Pending checks are not included; only checks that have actually failed.
func (w *Watcher) getFailedCheckNames(checks []forge.Check) []string {
	failed := make([]string, 0, len(checks))
	for _, c := range checks {
		if c.Status == forge.CheckStatusFailure {
			failed = append(failed, c.Name)
		}
	}
	return failed
}

// countCIFailures returns the persisted CI failure count for an issue.
// Uses the issue_failure_counts SQLite table via the issue cache reader.
// Falls back to comment scanning when no cache reader is configured.
func (w *Watcher) countCIFailures(ctx context.Context, issueNum int) (int, error) {
	// Fast path: query the store directly.
	if w.issueCache != nil {
		return w.issueCache.GetIssueFailureCount(ctx, issueNum)
	}

	// Fallback: comment scanning (only when issue cache is not configured).
	if w.issueTracker == nil {
		return 0, nil
	}

	comments, err := w.issueTracker.ListComments(ctx, issueNum)
	if err != nil {
		return 0, fmt.Errorf("list comments: %w", err)
	}

	count := 0
	for _, c := range comments {
		if strings.Contains(c.Body, "CI FAILED [prwatcher]") {
			count++
		}
	}
	return count, nil
}

// remediateStaleCIFailure closes a stale CI-failed PR and re-queues or escalates its source issue.
//
// It:
// 1. Closes the PR with a comment listing failed checks
// 2. Finds the source issue from "Closes #NNN" in the PR body
// 3. Counts existing CI failures for that issue
// 4. If count < 3: removes status:needs-qc, adds status:queued
// 5. If count >= 3: removes status:needs-qc, adds status:needs-human + human:review
// 6. Adds a comment with failure details to the issue
func (w *Watcher) remediateStaleCIFailure(ctx context.Context, pr *forge.PullRequest, failedChecks []string) error {
	if w.issueTracker == nil {
		return fmt.Errorf("issue tracker is nil")
	}

	// Close the PR with a comment listing failed checks.
	closedState := forge.StateClosed
	_, err := w.issueTracker.UpdateIssue(ctx, pr.Number, &forge.UpdateIssueRequest{State: &closedState})
	if err != nil {
		return fmt.Errorf("close PR #%d: %w", pr.Number, err)
	}

	// Build a comment with the failed checks.
	comment := buildCIFailureComment(pr, failedChecks)
	if _, err := w.issueTracker.AddComment(ctx, pr.Number, comment); err != nil {
		w.log().Warn("pr-watcher: failed to add comment to closed PR",
			zap.Int("pr", pr.Number), zap.Error(err))
	}

	// Find the source issue.
	sourceIssueNum := 0
	issueNums := parseLinkedIssues(pr)
	if len(issueNums) > 0 {
		sourceIssueNum = issueNums[0]
	}
	if sourceIssueNum == 0 {
		w.log().Warn("pr-watcher: no linked issue found for PR", zap.Int("pr", pr.Number))
		w.log().Info("pr-watcher: closed stale CI-failed PR",
			zap.Int("pr", pr.Number),
			zap.Int("failed_checks", len(failedChecks)))
		return nil
	}

	// Count existing CI failures.
	ciFailCount, err := w.countCIFailures(ctx, sourceIssueNum)
	if err != nil {
		w.log().Warn("pr-watcher: count CI failures", zap.Int("issue", sourceIssueNum), zap.Error(err))
		ciFailCount = 0
	}

	// Increment the CI failure count.
	ciFailCount++

	// Remove status:needs-qc from the issue.
	if err := w.issueTracker.RemoveLabel(ctx, sourceIssueNum, models.LabelStatusNeedsQc); err != nil {
		w.log().Warn("pr-watcher: remove status:needs-qc",
			zap.Int("issue", sourceIssueNum), zap.Error(err))
	}

	// Add failure comment to the issue.
	issueComment := buildCIFailureIssueComment(pr, failedChecks, ciFailCount)
	if _, err := w.issueTracker.AddComment(ctx, sourceIssueNum, issueComment); err != nil {
		w.log().Warn("pr-watcher: add comment to issue",
			zap.Int("issue", sourceIssueNum), zap.Error(err))
	}

	// Re-queue or escalate based on CI failure count.
	if ciFailCount < 3 {
		if err := w.issueTracker.AddLabels(ctx, sourceIssueNum, models.LabelStatusQueued); err != nil {
			w.log().Warn("pr-watcher: add status:queued",
				zap.Int("issue", sourceIssueNum), zap.Error(err))
		}

		w.log().Info("pr-watcher: closed stale CI-failed PR and re-queued issue",
			zap.Int("pr", pr.Number),
			zap.Int("issue", sourceIssueNum),
			zap.Int("ci_fail_count", ciFailCount),
			zap.Strings("failed_checks", failedChecks))
	} else {
		// Escalate to needs-human.
		if err := w.issueTracker.AddLabels(ctx, sourceIssueNum, models.LabelStatusNeedsHuman); err != nil {
			w.log().Warn("pr-watcher: add status:needs-human",
				zap.Int("issue", sourceIssueNum), zap.Error(err))
		}
		if err := w.issueTracker.AddLabels(ctx, sourceIssueNum, models.LabelHumanReview); err != nil {
			w.log().Warn("pr-watcher: add human:review",
				zap.Int("issue", sourceIssueNum), zap.Error(err))
		}

		w.log().Info("pr-watcher: escalated issue to needs-human after 3 CI failures",
			zap.Int("pr", pr.Number),
			zap.Int("issue", sourceIssueNum),
			zap.Int("ci_fail_count", ciFailCount),
			zap.Strings("failed_checks", failedChecks))
	}

	return nil
}

// remediateNonMergeableConflict closes a stale non-mergeable PR (merge conflict)
// and re-queues its source issue for a fresh attempt on current main.
//
// It:
// 1. Closes the PR with a comment about the merge conflict
// 2. Finds the source issue from "Closes #NNN" in the PR body
// 3. Removes status:needs-qc and adds status:queued
// 4. Adds a comment to the issue notifying about the conflict
func (w *Watcher) remediateNonMergeableConflict(ctx context.Context, pr *forge.PullRequest) error {
	if w.issueTracker == nil {
		return fmt.Errorf("issue tracker is nil")
	}

	// Close the PR with a comment about merge conflict.
	closedState := forge.StateClosed
	_, err := w.issueTracker.UpdateIssue(ctx, pr.Number, &forge.UpdateIssueRequest{State: &closedState})
	if err != nil {
		return fmt.Errorf("close PR #%d: %w", pr.Number, err)
	}

	// Build a comment about the merge conflict.
	comment := buildConflictComment(pr)
	if _, err := w.issueTracker.AddComment(ctx, pr.Number, comment); err != nil {
		w.log().Warn("pr-watcher: failed to add comment to closed PR",
			zap.Int("pr", pr.Number), zap.Error(err))
	}

	// Find the source issue.
	sourceIssueNum := 0
	issueNums := parseLinkedIssues(pr)
	if len(issueNums) > 0 {
		sourceIssueNum = issueNums[0]
	}
	if sourceIssueNum == 0 {
		w.log().Warn("pr-watcher: no linked issue found for PR", zap.Int("pr", pr.Number))
		w.log().Info("pr-watcher: closed non-mergeable PR",
			zap.Int("pr", pr.Number))
		return nil
	}

	// Remove status:needs-qc from the issue.
	if err := w.issueTracker.RemoveLabel(ctx, sourceIssueNum, models.LabelStatusNeedsQc); err != nil {
		w.log().Warn("pr-watcher: remove status:needs-qc",
			zap.Int("issue", sourceIssueNum), zap.Error(err))
	}

	// Add status:queued to the issue.
	if err := w.issueTracker.AddLabels(ctx, sourceIssueNum, models.LabelStatusQueued); err != nil {
		w.log().Warn("pr-watcher: add status:queued",
			zap.Int("issue", sourceIssueNum), zap.Error(err))
	}

	// Add a comment to the issue.
	issueComment := buildConflictIssueComment(pr)
	if _, err := w.issueTracker.AddComment(ctx, sourceIssueNum, issueComment); err != nil {
		w.log().Warn("pr-watcher: add comment to issue",
			zap.Int("issue", sourceIssueNum), zap.Error(err))
	}

	w.log().Info("pr-watcher: closed non-mergeable PR and re-queued issue",
		zap.Int("pr", pr.Number),
		zap.Int("issue", sourceIssueNum))

	return nil
}

// buildConflictComment creates a comment for a closed non-mergeable PR.
func buildConflictComment(pr *forge.PullRequest) string {
	var b strings.Builder

	fmt.Fprintf(&b, "CLOSED [prwatcher] [%s]\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "PR #%d has a merge conflict with main. Merge conflict with main. Source issue re-queued.\n", pr.Number)

	return b.String()
}

// buildConflictIssueComment creates a comment for the source issue of a non-mergeable PR.
func buildConflictIssueComment(pr *forge.PullRequest) string {
	var b strings.Builder

	fmt.Fprintf(&b, "CONFLICT [prwatcher] [%s]\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "PR #%d closed due to merge conflict with main.\n", pr.Number)
	fmt.Fprintf(&b, "Re-queuing for a fresh attempt on current main.\n")

	return b.String()
}

// buildCIFailureComment creates a comment for a closed stale CI-failed PR.
func buildCIFailureComment(pr *forge.PullRequest, failedChecks []string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "CLOSED [prwatcher] [%s]\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "PR #%d has had failing CI checks for too long. Closing and re-queuing the source issue.\n\n", pr.Number)

	if len(failedChecks) > 0 {
		b.WriteString("**Failed Checks:**\n")
		for _, check := range failedChecks {
			fmt.Fprintf(&b, "- %s\n", check)
		}
	}

	return b.String()
}

// buildCIFailureIssueComment creates a comment for the source issue of a CI-failed PR.
func buildCIFailureIssueComment(pr *forge.PullRequest, failedChecks []string, ciFailCount int) string {
	var b strings.Builder

	fmt.Fprintf(&b, "CI FAILED [prwatcher] [%s] (attempt %d)\n", time.Now().UTC().Format(time.RFC3339), ciFailCount)
	fmt.Fprintf(&b, "PR #%d closed due to failing CI checks.\n\n", pr.Number)

	if len(failedChecks) > 0 {
		b.WriteString("**Failed Checks:**\n")
		for _, check := range failedChecks {
			fmt.Fprintf(&b, "- %s\n", check)
		}
		b.WriteString("\n")
	}

	if ciFailCount < 3 {
		fmt.Fprintf(&b, "Re-queuing for retry (attempt %d of 3).\n", ciFailCount)
	} else {
		b.WriteString("Escalating to status:needs-human (3 CI failures reached).\n")
	}

	return b.String()
}

// QCApprovalMarker is the string that indicates a QC agent approved a PR.
// The dispatcher posts this as part of the QC PASS comment.
const QCApprovalMarker = "**QC Review: [PASS]**"

// hasQCApproval checks whether a PR's linked issues have a qc:pass label.
// QC labels are written to issues (not PRs) by the dispatcher, so we check
// linked issue labels via the issue cache. Falls back to comment scanning
// if no issue cache is configured.
func (w *Watcher) hasQCApproval(ctx context.Context, pr *forge.PullRequest) bool {
	if w.issueTracker == nil {
		return false
	}

	// Fast path: check linked issue labels via issue cache.
	if w.issueCache != nil {
		issueNums := parseLinkedIssues(pr)
		for _, issueNum := range issueNums {
			cached, err := w.issueCache.GetCachedIssue(ctx, w.project, issueNum)
			if err != nil {
				continue
			}
			for _, label := range cached.Labels {
				if label == models.LabelQcPass {
					return true
				}
			}
		}
		return false
	}

	// Fallback: comment scanning (only when issue cache is not configured).
	comments, err := w.issueTracker.ListComments(ctx, pr.Number)
	if err != nil {
		w.log().Debug("pr-watcher: check QC approval", zap.Int("pr", pr.Number), zap.Error(err))
		return false
	}

	for _, c := range comments {
		if strings.Contains(c.Body, QCApprovalMarker) {
			return true
		}
	}

	issueNums := parseLinkedIssues(pr)
	for _, issueNum := range issueNums {
		issueComments, err := w.issueTracker.ListComments(ctx, issueNum)
		if err != nil {
			continue
		}
		for _, c := range issueComments {
			if strings.Contains(c.Body, QCApprovalMarker) {
				return true
			}
		}
	}

	return false
}

// HasQCApprovalComment checks whether a list of comments contains a QC
// approval marker. Exported for testing.
func HasQCApprovalComment(comments []*forge.Comment) bool {
	for _, c := range comments {
		if strings.Contains(c.Body, QCApprovalMarker) {
			return true
		}
	}
	return false
}
