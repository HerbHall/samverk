package dispatcher

import (
	"context"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/agent"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// tryApplyEdits checks whether the completed task's output contains EDIT
// blocks posted as a comment. If so, it asks the pool to create a branch
// and PR from them. Returns true when a PR was successfully created.
//
// Only applies to code-gen and test agents — other agent types (docs,
// research, QC) always go through the normal needs-qc path.
func (d *Dispatcher) tryApplyEdits(ctx context.Context, result agent.TaskResult, tracker forge.IssueTracker) bool {
	// Only code-gen and test agents produce EDIT blocks.
	if result.AgentType != models.AgentTypeCodeGen && result.AgentType != models.AgentTypeTest {
		return false
	}

	if d.pool == nil {
		return false
	}

	// Read the latest comment to get the agent's output.
	comments, err := tracker.ListComments(ctx, result.IssueNumber)
	if err != nil || len(comments) == 0 {
		return false
	}

	// Walk backwards to find the most recent comment with EDIT blocks.
	var commentBody string
	for i := len(comments) - 1; i >= 0; i-- {
		if agent.HasEditBlocks(comments[i].Body) {
			commentBody = comments[i].Body
			break
		}
	}
	if commentBody == "" {
		return false
	}

	// Check whether a PR already exists for this issue (by branch name).
	issue, err := tracker.GetIssue(ctx, result.IssueNumber)
	if err != nil {
		d.logger.Warn("apply edits: could not fetch issue", zap.Int("issue", result.IssueNumber), zap.Error(err))
		return false
	}

	branch := agent.BranchSlug(result.IssueNumber, issue.Title)
	if d.branchHasPR(ctx, branch, tracker) {
		d.logger.Debug("apply edits: PR already exists for branch", zap.String("branch", branch))
		return false
	}

	// Attempt to create the PR from EDIT blocks.
	applyResult := d.pool.ApplyEditComment(ctx, result.IssueNumber, issue.Title, commentBody, tracker)
	if !applyResult.Applied {
		d.logger.Warn("apply edits: failed, falling back to needs-qc",
			zap.Int("issue", result.IssueNumber),
			zap.String("error", applyResult.Error),
		)
		// Post a comment so the failure is visible.
		errComment := "Auto-apply EDIT blocks failed: " + applyResult.Error + "\nFalling back to manual QC review."
		if _, err := tracker.AddComment(ctx, result.IssueNumber, errComment); err != nil {
			d.logger.Warn("apply edits: could not post error comment", zap.Error(err))
		}
		return false
	}

	return true
}

// branchHasPR checks whether any open PR exists with the given head branch.
func (d *Dispatcher) branchHasPR(ctx context.Context, branch string, tracker forge.IssueTracker) bool {
	pm, ok := tracker.(forge.PullRequestManager)
	if !ok {
		return false
	}
	prs, err := pm.ListPullRequests(ctx, &forge.ListPROptions{State: forge.StateOpen})
	if err != nil {
		return false
	}
	for _, pr := range prs {
		if pr.Head == branch {
			return true
		}
	}
	return false
}

// BackfillEditComments scans all open issues with status:needs-qc and
// attempts to auto-apply any EDIT comment output as PRs. This drains
// the backlog of comment-only agent output that accumulated before this
// feature existed. Runs once on dispatcher startup.
func (d *Dispatcher) BackfillEditComments(ctx context.Context) {
	d.logger.Info("backfill: scanning needs-qc issues for EDIT comments")
	applied := 0

	for _, entry := range d.trackerEntries {
		issues, err := entry.Tracker.ListIssues(ctx, &forge.ListOptions{State: forge.StateOpen})
		if err != nil {
			d.logger.Warn("backfill: list issues failed",
				zap.String("owner", entry.Owner),
				zap.String("repo", entry.Repo),
				zap.Error(err),
			)
			continue
		}

		for _, issue := range issues {
			if !labelSliceContains(issue.Labels, models.LabelStatusNeedsQc) {
				continue
			}
			// Only code-gen and test issues produce EDIT blocks.
			agentType := agentTypeFromLabels(issue.Labels)
			if agentType != models.AgentTypeCodeGen && agentType != models.AgentTypeTest {
				continue
			}

			// Check for existing PR.
			branch := agent.BranchSlug(issue.Number, issue.Title)
			if d.branchHasPR(ctx, branch, entry.Tracker) {
				continue
			}

			// Read comments for EDIT blocks.
			comments, err := entry.Tracker.ListComments(ctx, issue.Number)
			if err != nil || len(comments) == 0 {
				continue
			}

			var commentBody string
			for i := len(comments) - 1; i >= 0; i-- {
				if agent.HasEditBlocks(comments[i].Body) {
					commentBody = comments[i].Body
					break
				}
			}
			if commentBody == "" {
				continue
			}

			applyResult := d.pool.ApplyEditComment(ctx, issue.Number, issue.Title, commentBody, entry.Tracker)
			if applyResult.Applied {
				_ = entry.Tracker.RemoveLabel(ctx, issue.Number, models.LabelStatusNeedsQc)
				applied++
				d.logger.Info("backfill: applied EDIT comment as PR",
					zap.Int("issue", issue.Number),
					zap.Int("pr", applyResult.PRNumber),
				)
			} else {
				d.logger.Debug("backfill: skip (apply failed)",
					zap.Int("issue", issue.Number),
					zap.String("error", applyResult.Error),
				)
			}
		}
	}

	d.logger.Info("backfill: complete", zap.Int("applied", applied))
}

// agentTypeFromLabels extracts the agent type from issue labels.
func agentTypeFromLabels(labels []string) models.AgentType {
	for _, l := range labels {
		switch l {
		case "agent:code-gen":
			return models.AgentTypeCodeGen
		case "agent:test":
			return models.AgentTypeTest
		case "agent:docs":
			return models.AgentTypeDocs
		case "agent:research":
			return models.AgentTypeResearch
		case "agent:qc":
			return models.AgentTypeQC
		}
	}
	return ""
}

// labelSliceContains returns true if the label slice contains the target.
func labelSliceContains(labels []string, target string) bool {
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}
