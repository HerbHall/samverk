// Package prwatcher monitors open pull requests and auto-merges eligible ones.
//
// Eligibility requires: not draft, trusted author, no excluded labels,
// all CI checks passed, and mergeable. The watcher runs concurrently with
// the dispatcher via errgroup.
package prwatcher

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/forge"
)

// Watcher polls open PRs and auto-merges eligible ones.
type Watcher struct {
	prManager forge.PullRequestManager
	mergeCfg  autonomy.MergeConfig
	interval  time.Duration
	logger    *slog.Logger
}

// New creates a Watcher with the given PR manager and merge policy.
func New(pm forge.PullRequestManager, cfg autonomy.MergeConfig, interval time.Duration) *Watcher {
	return &Watcher{
		prManager: pm,
		mergeCfg:  cfg,
		interval:  interval,
		logger:    slog.Default(),
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

// poll lists open PRs and attempts to merge eligible ones.
func (w *Watcher) poll(ctx context.Context) error {
	prs, err := w.prManager.ListPullRequests(ctx, &forge.ListPROptions{
		State:   forge.StateOpen,
		PerPage: 50,
	})
	if err != nil {
		return fmt.Errorf("list open PRs: %w", err)
	}

	for _, pr := range prs {
		if !w.isEligible(pr) {
			continue
		}

		checks, err := w.prManager.GetPRChecks(ctx, pr.Number)
		if err != nil {
			w.logger.Error("pr-watcher: get checks", "pr", pr.Number, "error", err)
			continue
		}

		if !w.allChecksPassed(checks) {
			continue
		}

		w.logger.Info("pr-watcher: merging", "pr", pr.Number, "title", pr.Title)
		commitMsg := fmt.Sprintf("auto-merge: %s (#%d)", pr.Title, pr.Number)
		if err := w.prManager.MergePullRequest(ctx, pr.Number, forge.MergeMethodSquash, commitMsg); err != nil {
			w.logger.Error("pr-watcher: merge failed", "pr", pr.Number, "error", err)
		}
	}

	return nil
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
