package dispatcher

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/internal/store"
	"samverk.dev/samverk/pkg/models"
)

// ProjectResolver looks up a forge tracker for a cross-project dependency.
// This decouples the dispatcher from the concrete ProjectRegistry type.
type ProjectResolver interface {
	// TrackerFor returns the IssueTracker for the given owner/repo pair.
	// Returns nil, false if the project is not registered.
	TrackerFor(owner, repo string) (forge.IssueTracker, bool)

	// PhaseFor returns the lifecycle phase for the project matching the given
	// owner/repo pair. Returns ("", false) if no matching project is found.
	PhaseFor(owner, repo string) (string, bool)
}

// checkDependencies verifies all depends_on issues are done.
// Returns blocked=true with the human-readable list of unsatisfied dependency references.
// owner/repo identify the triggering issue's tracker for resolving local dependencies.
func (d *Dispatcher) checkDependencies(ctx context.Context, owner, repo string, fm *models.IssueFrontmatter) (blocked bool, blockers []string, err error) {
	if fm == nil || len(fm.DependsOn) == 0 {
		return false, nil, nil
	}

	project := owner + "/" + repo
	blockers = make([]string, 0, len(fm.DependsOn))

	for _, dep := range fm.DependsOn {
		if dep.IsCrossProject() {
			done, checkErr := d.checkCrossProjectDep(ctx, dep)
			if checkErr != nil {
				return false, nil, fmt.Errorf("check cross-project dep %s: %w", dep.String(), checkErr)
			}
			if !done {
				blockers = append(blockers, dep.String())
			}
		} else {
			// Fast path: check the issue cache (SQLite) instead of forge API.
			if d.store != nil {
				ci, cacheErr := d.store.GetCachedIssue(ctx, project, dep.Number)
				if cacheErr == nil {
					if !isDoneCached(ci) {
						blockers = append(blockers, dep.String())
					}
					continue
				}
				// Cache miss: fall through to forge API.
			}

			tracker := d.trackerFor(owner, repo)
			if tracker == nil {
				return false, nil, fmt.Errorf("no tracker for %s/%s", owner, repo)
			}
			issue, getErr := tracker.GetIssue(ctx, dep.Number)
			if getErr != nil {
				return false, nil, fmt.Errorf("get dependency #%d: %w", dep.Number, getErr)
			}
			if !isDone(issue) {
				blockers = append(blockers, dep.String())
			}
		}
	}

	return len(blockers) > 0, blockers, nil
}

// checkCrossProjectDep resolves a cross-project dependency via the ProjectResolver.
// Returns true if the referenced issue is closed with status:done.
func (d *Dispatcher) checkCrossProjectDep(ctx context.Context, dep models.Dependency) (bool, error) {
	if d.projects == nil {
		return false, fmt.Errorf("no project resolver configured for cross-project dep %s", dep.String())
	}

	// Fast path: check the issue cache (SQLite) instead of forge API.
	if d.store != nil {
		depProject := dep.Owner + "/" + dep.Repo
		ci, cacheErr := d.store.GetCachedIssue(ctx, depProject, dep.Number)
		if cacheErr == nil {
			return isDoneCached(ci), nil
		}
		// Cache miss: fall through to forge API.
	}

	tracker, ok := d.projects.TrackerFor(dep.Owner, dep.Repo)
	if !ok {
		return false, fmt.Errorf("project %s/%s not registered", dep.Owner, dep.Repo)
	}

	issue, err := tracker.GetIssue(ctx, dep.Number)
	if err != nil {
		return false, fmt.Errorf("get issue %s: %w", dep.String(), err)
	}

	return isDone(issue), nil
}

// isDone returns true if the issue is closed and has the status:done label.
func isDone(issue *forge.Issue) bool {
	if issue.State != forge.StateClosed {
		return false
	}
	for _, label := range issue.Labels {
		if label == models.LabelStatusDone {
			return true
		}
	}
	return false
}

// isDoneCached checks whether a cached issue is closed with status:done.
func isDoneCached(ci *store.CachedIssue) bool {
	if ci.State != "closed" {
		return false
	}
	for _, label := range ci.Labels {
		if label == models.LabelStatusDone {
			return true
		}
	}
	return false
}

// cachedToForgeIssue converts a store.CachedIssue to a forge.Issue.
func cachedToForgeIssue(ci *store.CachedIssue) *forge.Issue {
	return &forge.Issue{
		Number:    ci.Number,
		Title:     ci.Title,
		Body:      ci.Body,
		State:     forge.State(ci.State),
		Labels:    ci.Labels,
		Assignees: ci.Assignees,
		CreatedAt: ci.CreatedAt,
		UpdatedAt: ci.UpdatedAt,
		ClosedAt:  ci.ClosedAt,
	}
}

// detectCycle runs DFS from startIssue to find dependency cycles.
// Returns the cycle path if found, nil if no cycle exists.
// Only considers same-project (local) dependencies for cycle detection.
func (d *Dispatcher) detectCycle(ctx context.Context, owner, repo string, startIssue int) ([]int, error) {
	// Build the dependency graph from the triggering repo's open issues.
	graph, err := d.buildDependencyGraph(ctx, owner, repo)
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
				cycle := make([]int, 0, len(path)-cycleStart+1)
				cycle = append(cycle, path[cycleStart:]...)
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
// from each issue number to its local depends_on list.
// Cross-project dependencies are excluded from the graph since they
// are resolved separately in checkDependencies.
// The graph is built for the specified owner/repo tracker only (per-repo).
func (d *Dispatcher) buildDependencyGraph(ctx context.Context, owner, repo string) (graph map[int][]int, err error) {
	project := owner + "/" + repo

	// Fast path: build graph from the issue cache (SQLite).
	if d.store != nil {
		cached, cacheErr := d.store.ListCachedIssues(ctx, project, "open", nil)
		if cacheErr == nil {
			graph = make(map[int][]int, len(cached))
			for i := range cached {
				result, parseErr := models.ParseFrontmatter(cached[i].Body)
				if parseErr != nil || result.Frontmatter == nil {
					continue
				}
				localDeps := result.Frontmatter.DependsOn.LocalDeps()
				if len(localDeps) > 0 {
					graph[cached[i].Number] = localDeps
				}
			}
			return graph, nil
		}
	}

	// Fallback: query the forge API directly.
	tracker := d.trackerFor(owner, repo)
	if tracker == nil {
		return nil, fmt.Errorf("no tracker for %s/%s", owner, repo)
	}
	issues, listErr := tracker.ListIssues(ctx, &forge.ListOptions{State: forge.StateOpen})
	if listErr != nil {
		return nil, fmt.Errorf("list open issues: %w", listErr)
	}

	graph = make(map[int][]int, len(issues))
	for _, issue := range issues {
		result, parseErr := models.ParseFrontmatter(issue.Body)
		if parseErr != nil || result.Frontmatter == nil {
			continue
		}
		localDeps := result.Frontmatter.DependsOn.LocalDeps()
		if len(localDeps) > 0 {
			graph[issue.Number] = localDeps
		}
	}
	return graph, nil
}

// unblockDependents scans blocked issues across ALL registered trackers
// and transitions any whose dependencies are all satisfied after
// closedIssueNumber was closed. A close in repo B may unblock repo A.
func (d *Dispatcher) unblockDependents(ctx context.Context, closedIssueNumber int) error {
	for _, entry := range d.trackerEntries {
		// Fast path: query the issue cache (SQLite) for blocked issues.
		var issues []*forge.Issue
		if d.store != nil {
			cached, cacheErr := d.store.ListCachedIssues(ctx, entry.Repo, "open", []string{models.LabelStatusBlocked})
			if cacheErr == nil {
				issues = make([]*forge.Issue, len(cached))
				for i := range cached {
					issues[i] = cachedToForgeIssue(&cached[i])
				}
			} else {
				d.logger.Debug("unblockDependents: cache miss, falling back to API",
					zap.String("owner", entry.Owner), zap.String("repo", entry.Repo), zap.Error(cacheErr))
			}
		}
		// Fallback: query the forge API directly.
		if issues == nil {
			var err error
			issues, err = entry.Tracker.ListIssues(ctx, &forge.ListOptions{
				State:  forge.StateOpen,
				Labels: []string{models.LabelStatusBlocked},
			})
			if err != nil {
				d.logger.Warn("list blocked issues", zap.String("owner", entry.Owner), zap.String("repo", entry.Repo), zap.Error(err))
				continue
			}
		}

		for _, issue := range issues {
			result, parseErr := models.ParseFrontmatter(issue.Body)
			if parseErr != nil || result.Frontmatter == nil {
				continue
			}

			// Only check issues that depend on the one that just closed (local deps).
			dependsOnClosed := false
			for _, dep := range result.Frontmatter.DependsOn {
				if !dep.IsCrossProject() && dep.Number == closedIssueNumber {
					dependsOnClosed = true
					break
				}
			}
			if !dependsOnClosed {
				continue
			}

			// Check if ALL dependencies are now satisfied.
			blocked, _, depErr := d.checkDependencies(ctx, entry.Owner, entry.Repo, result.Frontmatter)
			if depErr != nil {
				d.logger.Warn("check deps", zap.Int("issue", issue.Number), zap.String("error", depErr.Error()))
				continue
			}
			if blocked {
				continue
			}

			// Unblock: transition from blocked to queued.
			if removeErr := entry.Tracker.RemoveLabel(ctx, issue.Number, models.LabelStatusBlocked); removeErr != nil {
				d.logger.Warn("remove label", zap.Int("issue", issue.Number), zap.String("label", models.LabelStatusBlocked), zap.String("error", removeErr.Error()))
			}
			if addErr := entry.Tracker.AddLabels(ctx, issue.Number, models.LabelStatusQueued); addErr != nil {
				d.logger.Warn("add label", zap.Int("issue", issue.Number), zap.String("label", models.LabelStatusQueued), zap.String("error", addErr.Error()))
			}

			comment := fmt.Sprintf(
				"UNBLOCKED [dispatcher] [%s]\nDependency #%d completed. All dependencies satisfied.\nTransitioning to status:queued for routing.",
				time.Now().UTC().Format(time.RFC3339), closedIssueNumber,
			)
			if _, commentErr := entry.Tracker.AddComment(ctx, issue.Number, comment); commentErr != nil {
				d.logger.Warn("add comment", zap.Int("issue", issue.Number), zap.String("context", "unblock"), zap.String("error", commentErr.Error()))
			}

			d.logger.Info("unblocked", zap.Int("issue", issue.Number), zap.Int("closed_dep", closedIssueNumber))
		}
	}

	return nil
}

// recheckCrossProjectDeps scans blocked issues across ALL registered trackers
// with cross-project dependencies and unblocks any whose dependencies are all
// satisfied. This is called periodically from the heartbeat ticker since
// cross-project closures are not delivered via the local forge watcher.
func (d *Dispatcher) recheckCrossProjectDeps(ctx context.Context) error {
	if d.projects == nil {
		return nil
	}

	for _, entry := range d.trackerEntries {
		// Fast path: query the issue cache (SQLite) for blocked issues.
		var blockedIssues []*forge.Issue
		if d.store != nil {
			cached, cacheErr := d.store.ListCachedIssues(ctx, entry.Repo, "open", []string{models.LabelStatusBlocked})
			if cacheErr == nil {
				blockedIssues = make([]*forge.Issue, len(cached))
				for i := range cached {
					blockedIssues[i] = cachedToForgeIssue(&cached[i])
				}
			} else {
				d.logger.Debug("recheckCrossProjectDeps: cache miss, falling back to API",
					zap.String("owner", entry.Owner), zap.String("repo", entry.Repo), zap.Error(cacheErr))
			}
		}
		// Fallback: query the forge API directly.
		if blockedIssues == nil {
			var err error
			blockedIssues, err = entry.Tracker.ListIssues(ctx, &forge.ListOptions{
				State:  forge.StateOpen,
				Labels: []string{models.LabelStatusBlocked},
			})
			if err != nil {
				d.logger.Warn("list blocked issues for cross-project recheck",
					zap.String("owner", entry.Owner), zap.String("repo", entry.Repo), zap.Error(err))
				continue
			}
		}

		for _, issue := range blockedIssues {
			result, parseErr := models.ParseFrontmatter(issue.Body)
			if parseErr != nil || result.Frontmatter == nil {
				continue
			}

			// Only process issues that have at least one cross-project dependency.
			crossDeps := result.Frontmatter.DependsOn.CrossProjectDeps()
			if len(crossDeps) == 0 {
				continue
			}

			// Check if ALL dependencies (local + cross-project) are satisfied.
			blocked, _, depErr := d.checkDependencies(ctx, entry.Owner, entry.Repo, result.Frontmatter)
			if depErr != nil {
				d.logger.Warn("cross-project dep recheck",
					zap.Int("issue", issue.Number), zap.String("error", depErr.Error()))
				continue
			}
			if blocked {
				continue
			}

			// Unblock: transition from blocked to queued.
			if removeErr := entry.Tracker.RemoveLabel(ctx, issue.Number, models.LabelStatusBlocked); removeErr != nil {
				d.logger.Warn("remove label", zap.Int("issue", issue.Number), zap.String("label", models.LabelStatusBlocked), zap.String("error", removeErr.Error()))
			}
			if addErr := entry.Tracker.AddLabels(ctx, issue.Number, models.LabelStatusQueued); addErr != nil {
				d.logger.Warn("add label", zap.Int("issue", issue.Number), zap.String("label", models.LabelStatusQueued), zap.String("error", addErr.Error()))
			}

			comment := fmt.Sprintf(
				"UNBLOCKED [dispatcher] [%s]\nAll cross-project dependencies satisfied.\nTransitioning to status:queued for routing.",
				time.Now().UTC().Format(time.RFC3339),
			)
			if _, commentErr := entry.Tracker.AddComment(ctx, issue.Number, comment); commentErr != nil {
				d.logger.Warn("add comment", zap.Int("issue", issue.Number), zap.String("context", "cross-project-unblock"), zap.String("error", commentErr.Error()))
			}

			d.logger.Info("cross-project unblocked", zap.Int("issue", issue.Number))
		}
	}

	return nil
}
