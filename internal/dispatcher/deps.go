package dispatcher

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// checkDependencies verifies all depends_on issues are done.
// Returns blocked=true with the list of unsatisfied dependency issue numbers.
func (d *Dispatcher) checkDependencies(ctx context.Context, fm *models.IssueFrontmatter) (blocked bool, blockers []int, err error) {
	if fm == nil || len(fm.DependsOn) == 0 {
		return false, nil, nil
	}

	blockers = make([]int, 0, len(fm.DependsOn))
	for _, dep := range fm.DependsOn {
		issue, getErr := d.tracker.GetIssue(ctx, dep)
		if getErr != nil {
			return false, nil, fmt.Errorf("get dependency #%d: %w", dep, getErr)
		}
		if !isDone(issue) {
			blockers = append(blockers, dep)
		}
	}

	return len(blockers) > 0, blockers, nil
}

// isDone returns true if the issue is closed and has the status:done label.
func isDone(issue *forge.Issue) bool {
	if issue.State != forge.StateClosed {
		return false
	}
	for _, label := range issue.Labels {
		if label == "status:done" {
			return true
		}
	}
	return false
}

// detectCycle runs DFS from startIssue to find dependency cycles.
// Returns the cycle path if found, nil if no cycle exists.
func (d *Dispatcher) detectCycle(ctx context.Context, startIssue int) ([]int, error) {
	// Build the dependency graph from all open issues.
	graph, err := d.buildDependencyGraph(ctx)
	if err != nil {
		return nil, err
	}

	// DFS with visited set and current path tracking.
	visited := make(map[int]bool)
	inPath := make(map[int]bool)
	var path []int

	var dfs func(node int) []int
	dfs = func(node int) []int {
		if inPath[node] {
			// Found a cycle. Extract the cycle from path.
			cycleStart := -1
			for i, n := range path {
				if n == node {
					cycleStart = i
					break
				}
			}
			if cycleStart >= 0 {
				cycle := make([]int, len(path)-cycleStart)
				copy(cycle, path[cycleStart:])
				return append(cycle, node)
			}
			return []int{node}
		}
		if visited[node] {
			return nil
		}

		visited[node] = true
		inPath[node] = true
		path = append(path, node)

		for _, dep := range graph[node] {
			if cycle := dfs(dep); cycle != nil {
				return cycle
			}
		}

		inPath[node] = false
		path = path[:len(path)-1]
		return nil
	}

	return dfs(startIssue), nil
}

// buildDependencyGraph lists open issues and builds an adjacency list
// from each issue number to its depends_on list.
func (d *Dispatcher) buildDependencyGraph(ctx context.Context) (graph map[int][]int, err error) {
	issues, listErr := d.tracker.ListIssues(ctx, &forge.ListOptions{State: forge.StateOpen})
	if listErr != nil {
		return nil, fmt.Errorf("list open issues: %w", listErr)
	}

	graph = make(map[int][]int, len(issues))
	for _, issue := range issues {
		result, parseErr := models.ParseFrontmatter(issue.Body)
		if parseErr != nil || result.Frontmatter == nil {
			continue
		}
		if len(result.Frontmatter.DependsOn) > 0 {
			graph[issue.Number] = result.Frontmatter.DependsOn
		}
	}
	return graph, nil
}

// unblockDependents scans blocked issues and transitions any whose
// dependencies are all satisfied after closedIssueNumber was closed.
func (d *Dispatcher) unblockDependents(ctx context.Context, closedIssueNumber int) error {
	blockedIssues, err := d.tracker.ListIssues(ctx, &forge.ListOptions{
		State:  forge.StateOpen,
		Labels: []string{"status:blocked"},
	})
	if err != nil {
		return fmt.Errorf("list blocked issues: %w", err)
	}

	for _, issue := range blockedIssues {
		result, parseErr := models.ParseFrontmatter(issue.Body)
		if parseErr != nil || result.Frontmatter == nil {
			continue
		}

		// Only check issues that depend on the one that just closed.
		dependsOnClosed := false
		for _, dep := range result.Frontmatter.DependsOn {
			if dep == closedIssueNumber {
				dependsOnClosed = true
				break
			}
		}
		if !dependsOnClosed {
			continue
		}

		// Check if ALL dependencies are now satisfied.
		blocked, _, depErr := d.checkDependencies(ctx, result.Frontmatter)
		if depErr != nil {
			d.logger.Warn("check deps", slog.Int("issue", issue.Number), slog.String("error", depErr.Error()))
			continue
		}
		if blocked {
			continue
		}

		// Unblock: transition from blocked to queued.
		if removeErr := d.tracker.RemoveLabel(ctx, issue.Number, "status:blocked"); removeErr != nil {
			d.logger.Warn("remove label", slog.Int("issue", issue.Number), slog.String("label", "blocked"), slog.String("error", removeErr.Error()))
		}
		if addErr := d.tracker.AddLabel(ctx, issue.Number, "status:queued"); addErr != nil {
			d.logger.Warn("add label", slog.Int("issue", issue.Number), slog.String("label", "queued"), slog.String("error", addErr.Error()))
		}

		comment := fmt.Sprintf(
			"UNBLOCKED [dispatcher] [%s]\nDependency #%d completed. All dependencies satisfied.\nTransitioning to status:queued for routing.",
			time.Now().UTC().Format(time.RFC3339), closedIssueNumber,
		)
		if _, commentErr := d.tracker.AddComment(ctx, issue.Number, comment); commentErr != nil {
			d.logger.Warn("add comment", slog.Int("issue", issue.Number), slog.String("context", "unblock"), slog.String("error", commentErr.Error()))
		}

		d.logger.Info("unblocked", slog.Int("issue", issue.Number), slog.Int("closed_dep", closedIssueNumber))
	}

	return nil
}
