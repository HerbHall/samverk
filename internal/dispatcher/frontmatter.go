package dispatcher

import (
	"context"

	"go.uber.org/zap"

	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/pkg/models"
)

// autoInjectFrontmatter builds an IssueFrontmatter from heuristic classification
// and attempts to persist it back to the issue body via UpdateIssue. If persistence
// fails the generated frontmatter is still returned for in-memory routing.
func (d *Dispatcher) autoInjectFrontmatter(ctx context.Context, owner, repo string, issue *forge.Issue, agentType models.AgentType) *models.IssueFrontmatter {
	fm := &models.IssueFrontmatter{
		SchemaVersion: models.SchemaVersion,
		Type:          models.IssueTypeTask,
		AgentType:     agentType,
		Priority:      derivePriorityFromLabels(issue.Labels),
	}

	newBody := models.RenderFrontmatter(fm, issue.Body)
	tracker := d.trackerFor(owner, repo)
	_, err := tracker.UpdateIssue(ctx, issue.Number, &forge.UpdateIssueRequest{
		Body: &newBody,
	})
	if err != nil {
		d.logger.Warn("auto-inject frontmatter failed",
			zap.Int("issue", issue.Number),
			zap.Error(err),
		)
	} else {
		issue.Body = newBody // keep in-memory copy in sync
		d.logger.Info("auto-injected frontmatter",
			zap.Int("issue", issue.Number),
			zap.String("agent", string(agentType)),
		)
	}

	return fm
}

// derivePriorityFromLabels extracts a priority from issue labels.
// Returns "normal" when no priority label is present.
func derivePriorityFromLabels(labels []string) models.Priority {
	for _, l := range labels {
		switch l {
		case models.LabelPriorityCritical:
			return models.PriorityCritical
		case models.LabelPriorityHigh:
			return models.PriorityHigh
		case models.LabelPriorityLow:
			return models.PriorityLow
		}
	}
	return models.PriorityNormal
}
