package agent

import (
	"context"
	"fmt"

	"samverk.dev/samverk/internal/forge"
	"go.uber.org/zap"
)

// ApplyEditsResult describes the outcome of attempting to convert EDIT
// comment output into a pull request.
type ApplyEditsResult struct {
	Applied  bool   // true if a PR was created
	PRNumber int    // PR number when Applied is true
	Error    string // non-empty on failure
}

// ApplyEditComment parses EDIT blocks from commentBody, creates a branch,
// writes the files, and opens a PR. It is called by the dispatcher when a
// runner posted EDIT output as a comment instead of creating a PR directly
// (e.g. Ollama providers without workspace access).
//
// Returns Applied=true on success. On any failure the caller should fall
// back to the normal needs-qc labeling so a human can review.
func (p *Pool) ApplyEditComment(ctx context.Context, issueNumber int, issueTitle, commentBody string, tracker forge.IssueTracker) ApplyEditsResult {
	if p.repoWriter == nil || p.prManager == nil {
		return ApplyEditsResult{Error: "repo writer or PR manager not configured"}
	}

	parsed := ParseEditBlocks(commentBody)
	if len(parsed.Edits) == 0 {
		return ApplyEditsResult{Error: "no EDIT blocks found in comment"}
	}

	branch := BranchSlug(issueNumber, issueTitle)

	// Create branch from default branch HEAD.
	if err := p.repoWriter.CreateBranch(ctx, branch); err != nil {
		return ApplyEditsResult{Error: fmt.Sprintf("create branch %q: %v", branch, err)}
	}

	// Write each file to the branch.
	for _, edit := range parsed.Edits {
		msg := fmt.Sprintf("agent: update %s for issue #%d", edit.Path, issueNumber)
		if err := p.repoWriter.CreateOrUpdateFile(ctx, branch, edit.Path, edit.Content, msg); err != nil {
			return ApplyEditsResult{Error: fmt.Sprintf("write file %q: %v", edit.Path, err)}
		}
	}

	// Open the PR.
	prTitle := parsed.PRTitle
	if prTitle == "" {
		prTitle = fmt.Sprintf("agent: resolve issue #%d", issueNumber)
	}
	pr, err := p.prManager.CreatePullRequest(ctx, &forge.CreatePRRequest{
		Title: prTitle,
		Body:  fmt.Sprintf("Closes #%d\n\nAuto-applied from EDIT comment output.", issueNumber),
		Head:  branch,
		Base:  "main",
	})
	if err != nil {
		return ApplyEditsResult{Error: fmt.Sprintf("create PR: %v", err)}
	}

	// Post link comment on issue.
	comment := fmt.Sprintf("PR opened (auto-applied from EDIT comment): #%d", pr.Number)
	if _, err = tracker.AddComment(ctx, issueNumber, comment); err != nil {
		p.logger.Warn("apply edits: failed to post PR link comment",
			zap.Int("issue", issueNumber),
			zap.Error(err),
		)
	}

	p.logger.Info("apply edits: PR created from EDIT comment",
		zap.Int("issue", issueNumber),
		zap.Int("pr", pr.Number),
		zap.Int("files", len(parsed.Edits)),
	)

	return ApplyEditsResult{Applied: true, PRNumber: pr.Number}
}
