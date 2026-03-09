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
	"log/slog"
	"strings"
	"time"

	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/forge"
)

// Watcher polls open PRs and auto-merges eligible ones.
type Watcher struct {
	prManager    forge.PullRequestManager
	issueTracker forge.IssueTracker
	mergeCfg     autonomy.MergeConfig
	interval     time.Duration
	logger       *slog.Logger
}

// New creates a Watcher with the given PR manager, issue tracker, and merge policy.
// The issue tracker is used to create remediation issues for PRs with blocking
// review comments. It may be nil to disable remediation.
func New(pm forge.PullRequestManager, issues forge.IssueTracker, cfg autonomy.MergeConfig, interval time.Duration) *Watcher {
	return &Watcher{
		prManager:    pm,
		issueTracker: issues,
		mergeCfg:     cfg,
		interval:     interval,
		logger:       slog.Default(),
	}
}

// Run polls open PRs at the configured interval until the context is cancelled.
func (w *Watcher) Run(ctx context.Context) error {
	if !w.mergeCfg.AutoMergeOnCIPass {
		w.logger.Info("pr-watcher: auto-merge disabled, exiting")
		return nil
	}

	w.logger.Info("pr-watcher: starting", "interval", w.interval)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.poll(ctx); err != nil {
				w.logger.Error("pr-watcher: poll error", "error", err)
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

	for _, pr := range prs {
		if !w.isEligible(pr) {
			continue
		}

		// Check for blocking review comments and create remediation issues.
		if w.issueTracker != nil {
			hasBlocking, checkErr := w.checkReviewComments(ctx, pr)
			if checkErr != nil {
				w.logger.Error("pr-watcher: check review comments", "pr", pr.Number, "error", checkErr)
			}
			if hasBlocking {
				continue
			}
		}

		checks, checkErr := w.prManager.GetPRChecks(ctx, pr.Number)
		if checkErr != nil {
			w.logger.Error("pr-watcher: get checks", "pr", pr.Number, "error", checkErr)
			continue
		}

		ciPassed := w.allChecksPassed(checks)
		tier := ClassifyPRTier(pr, nil)

		switch tier {
		case PRTier3:
			// Never auto-merge. Label if not already labeled.
			w.labelTier3(ctx, pr)
			continue
		case PRTier2:
			if !ciPassed {
				continue
			}
			// Merge only after the configured delay.
			if time.Since(pr.UpdatedAt) < tier2Delay {
				w.logger.Debug("pr-watcher: tier-2 delay not elapsed",
					"pr", pr.Number, "age", time.Since(pr.UpdatedAt).Truncate(time.Minute))
				continue
			}
		case PRTier1:
			if !ciPassed {
				continue
			}
			// Merge immediately.
		default:
			if !ciPassed {
				continue
			}
		}

		w.logger.Info("pr-watcher: merging", "pr", pr.Number, "title", pr.Title, "tier", tier.String())
		commitMsg := fmt.Sprintf("auto-merge: %s (#%d)", pr.Title, pr.Number)
		if mergeErr := w.prManager.MergePullRequest(ctx, pr.Number, forge.MergeMethodSquash, commitMsg); mergeErr != nil {
			w.logger.Error("pr-watcher: merge failed", "pr", pr.Number, "error", mergeErr)
		}
	}

	return nil
}

// labelTier3 adds the status:needs-human label to a Tier 3 PR if not already present.
func (w *Watcher) labelTier3(ctx context.Context, pr *forge.PullRequest) {
	for _, l := range pr.Labels {
		if l == "status:needs-human" {
			return
		}
	}
	if w.issueTracker != nil {
		if err := w.issueTracker.AddLabel(ctx, pr.Number, "status:needs-human"); err != nil {
			w.logger.Error("pr-watcher: label tier-3 PR", "pr", pr.Number, "error", err)
		}
	}
}

// checkReviewComments checks for unresolved review comments from trusted reviewers.
// It returns true if blocking comments exist (remediation issue created or already exists).
func (w *Watcher) checkReviewComments(ctx context.Context, pr *forge.PullRequest) (bool, error) {
	// Skip if PR already has status:needs-human label.
	for _, l := range pr.Labels {
		if l == "status:needs-human" {
			return false, nil
		}
	}

	comments, err := w.prManager.ListReviewComments(ctx, pr.Number)
	if err != nil {
		return false, fmt.Errorf("list review comments for PR #%d: %w", pr.Number, err)
	}

	// Filter for unresolved comments from trusted reviewers.
	var blocking []forge.ReviewComment
	for _, c := range comments {
		if c.Resolved {
			continue
		}
		if w.isTrustedReviewer(c.Author) {
			blocking = append(blocking, c)
		}
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
	title := fmt.Sprintf("fix(#%d): address review comments on %q", pr.Number, pr.Title)
	body := buildRemediationBody(pr, blocking)

	_, err = w.issueTracker.CreateIssue(ctx, &forge.CreateIssueRequest{
		Title:  title,
		Body:   body,
		Labels: []string{"status:queued", "agent:code-gen", prLabel},
	})
	if err != nil {
		return true, fmt.Errorf("create remediation issue for PR #%d: %w", pr.Number, err)
	}

	w.log().Info("pr-watcher: created remediation issue", "pr", pr.Number, "comments", len(blocking))
	return true, nil
}

// log returns the watcher's logger, falling back to slog.Default() if nil.
// Allows direct struct construction in tests without setting a logger.
func (w *Watcher) log() *slog.Logger {
	if w.logger != nil {
		return w.logger
	}
	return slog.Default()
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
