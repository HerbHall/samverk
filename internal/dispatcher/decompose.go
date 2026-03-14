package dispatcher

import (
	"context"
	"fmt"
	"time"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
	"go.uber.org/zap"
)

// SubTask is a single child issue proposed by the Decomposer.
type SubTask struct {
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	AgentType models.AgentType `json:"agent_type"`
	DependsOn []int           `json:"depends_on,omitempty"`
}

// Decomposer breaks an oversized issue into smaller sub-tasks.
// Implementations may call an AI provider (e.g. haiku for fast triage)
// or use rule-based heuristics.
type Decomposer interface {
	Decompose(ctx context.Context, issue *forge.Issue, estimatedTimeout, maxTimeout time.Duration) ([]SubTask, error)
}

// shouldDecompose returns true when the issue should be split into children.
// Conditions: decomposition is enabled, timeout exceeds threshold, issue is not
// already a child (has no parent_issue), and issue does not carry the
// no-decompose label.
func shouldDecompose(timeout time.Duration, cfg *Config, fm *models.IssueFrontmatter, issue *forge.Issue) bool {
	// Feature disabled when threshold is zero.
	if cfg.DecompositionThreshold <= 0 {
		return false
	}

	// Already a child issue — don't recurse.
	if fm != nil && fm.ParentIssue > 0 {
		return false
	}

	// Manual override: no-decompose label.
	for _, l := range issue.Labels {
		if l == "no-decompose" {
			return false
		}
	}

	return timeout > cfg.DecompositionThreshold
}

// decomposeAndCreateChildren runs the decomposer and creates child issues.
// It blocks the parent and returns true if decomposition occurred.
func (d *Dispatcher) decomposeAndCreateChildren(
	ctx context.Context,
	issue *forge.Issue,
	fm *models.IssueFrontmatter,
	agentType models.AgentType,
	timeout time.Duration,
) (bool, error) {
	if d.decomposer == nil {
		return false, nil
	}
	if !shouldDecompose(timeout, d.config, fm, issue) {
		return false, nil
	}

	d.logger.Info("decomposing oversized issue",
		zap.Int("issue", issue.Number),
		zap.Duration("estimated_timeout", timeout),
		zap.Duration("threshold", d.config.DecompositionThreshold),
	)

	subtasks, err := d.decomposer.Decompose(ctx, issue, timeout, MaxTimeout)
	if err != nil {
		d.logger.Warn("decomposition failed, routing as-is",
			zap.Int("issue", issue.Number),
			zap.Error(err),
		)
		return false, nil
	}

	if len(subtasks) < 2 {
		d.logger.Info("decomposer returned < 2 subtasks, routing as-is",
			zap.Int("issue", issue.Number),
			zap.Int("subtask_count", len(subtasks)),
		)
		return false, nil
	}

	childNumbers, createErr := d.createChildIssues(ctx, issue, agentType, subtasks)
	if createErr != nil {
		return false, fmt.Errorf("create child issues for #%d: %w", issue.Number, createErr)
	}

	// Block parent on all children.
	if blockErr := d.blockIssue(ctx, issue.Number, childNumbers); blockErr != nil {
		return false, fmt.Errorf("block parent #%d: %w", issue.Number, blockErr)
	}

	d.logger.Info("issue decomposed",
		zap.Int("parent", issue.Number),
		zap.Ints("children", childNumbers),
	)

	return true, nil
}

// createChildIssues creates forge issues for each subtask, linking them
// back to the parent via frontmatter. Returns the created issue numbers.
func (d *Dispatcher) createChildIssues(
	ctx context.Context,
	parent *forge.Issue,
	fallbackAgent models.AgentType,
	subtasks []SubTask,
) ([]int, error) {
	numbers := make([]int, 0, len(subtasks))

	for i, st := range subtasks {
		at := st.AgentType
		if at == "" {
			at = fallbackAgent
		}

		body := buildChildBody(parent.Number, at, st.Body, st.DependsOn)

		labels := []string{
			"status:queued",
			fmt.Sprintf("agent:%s", at),
			"decomposed",
		}

		created, err := d.tracker.CreateIssue(ctx, &forge.CreateIssueRequest{
			Title:  st.Title,
			Body:   body,
			Labels: labels,
		})
		if err != nil {
			return numbers, fmt.Errorf("create child %d/%d: %w", i+1, len(subtasks), err)
		}

		numbers = append(numbers, created.Number)
		d.logger.Info("child issue created",
			zap.Int("parent", parent.Number),
			zap.Int("child", created.Number),
			zap.String("title", st.Title),
		)
	}

	return numbers, nil
}

// buildChildBody constructs the issue body with frontmatter linking to parent.
func buildChildBody(parentNumber int, agentType models.AgentType, description string, dependsOn []int) string {
	fm := fmt.Sprintf("---\nschema_version: \"1.0.0\"\ntype: task\nagent_type: %s\nparent_issue: %d\n", agentType, parentNumber)
	if len(dependsOn) > 0 {
		fm += "depends_on:\n"
		for _, dep := range dependsOn {
			fm += fmt.Sprintf("  - %d\n", dep)
		}
	}
	fm += "---\n\n"
	return fm + description
}
