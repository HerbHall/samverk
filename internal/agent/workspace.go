package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	worktreePrefix   = "samverk-agent-"
	staleWorktreeAge = 2 * time.Hour
)

// CreateWorkspace creates an isolated git worktree for agent task execution.
// The worktree is branched from HEAD at agent/<issueNumber>.
// Returns the workspace path and a cleanup function that removes the worktree
// and its branch. The cleanup function is safe to call multiple times.
func CreateWorkspace(repoDir, sessionID string, issueNumber int, logger *zap.Logger) (wsPath string, cleanup func(), retErr error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Prune stale worktrees before creating a new one.
	PruneStaleWorktrees(logger)
	_, _ = gitExec(repoDir, "worktree", "prune")

	wsPath = filepath.Join(os.TempDir(), fmt.Sprintf("%s%s", worktreePrefix, sessionID))
	branch := fmt.Sprintf("agent/%d", issueNumber)

	// Create worktree with new branch. Fall back to existing branch if it
	// already exists (e.g. retry after a previous session failed without cleanup).
	_, err := gitExec(repoDir, "worktree", "add", wsPath, "-b", branch)
	if err != nil {
		_, err = gitExec(repoDir, "worktree", "add", wsPath, branch)
		if err != nil {
			return "", nil, fmt.Errorf("create worktree: %w", err)
		}
	}

	logger.Info("workspace created",
		zap.String("path", wsPath),
		zap.String("branch", branch),
		zap.String("session", sessionID),
	)

	cleaned := false
	cleanup = func() {
		if cleaned {
			return
		}
		cleaned = true

		if rmErr := os.RemoveAll(wsPath); rmErr != nil {
			logger.Warn("workspace cleanup: remove dir failed",
				zap.String("path", wsPath),
				zap.Error(rmErr),
			)
		}
		if _, pruneErr := gitExec(repoDir, "worktree", "prune"); pruneErr != nil {
			logger.Warn("workspace cleanup: prune failed", zap.Error(pruneErr))
		}
		// Delete the branch (best-effort). It may have been pushed to the
		// remote already, so local deletion is just tidying up.
		if _, delErr := gitExec(repoDir, "branch", "-D", branch); delErr != nil {
			logger.Debug("workspace cleanup: branch delete skipped",
				zap.String("branch", branch),
				zap.Error(delErr),
			)
		}
	}

	return wsPath, cleanup, nil
}

// PruneStaleWorktrees removes agent worktree directories that are older than
// staleWorktreeAge (2 hours). This prevents leaked worktrees from accumulating
// after crashes or ungraceful shutdowns.
func PruneStaleWorktrees(logger *zap.Logger) {
	if logger == nil {
		logger = zap.NewNop()
	}

	pattern := filepath.Join(os.TempDir(), worktreePrefix+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		logger.Warn("stale worktree prune: glob failed", zap.Error(err))
		return
	}

	cutoff := time.Now().Add(-staleWorktreeAge)
	for _, dir := range matches {
		info, statErr := os.Stat(dir)
		if statErr != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			logger.Info("removing stale agent worktree", zap.String("path", dir))
			if rmErr := os.RemoveAll(dir); rmErr != nil {
				logger.Warn("stale worktree prune: remove failed",
					zap.String("path", dir),
					zap.Error(rmErr),
				)
			}
		}
	}
}

// CommitAndPush stages all changes in the workspace directory, commits with the
// given message, and pushes to origin. Returns true if changes were committed,
// false if the workspace was clean.
func CommitAndPush(workDir, commitMsg string) (changed bool, err error) {
	out, err := gitExec(workDir, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	if strings.TrimSpace(out) == "" {
		return false, nil
	}

	if _, err = gitExec(workDir, "add", "-A"); err != nil {
		return false, fmt.Errorf("git add: %w", err)
	}
	if _, err = gitExec(workDir, "commit", "-m", commitMsg); err != nil {
		return false, fmt.Errorf("git commit: %w", err)
	}
	if _, err = gitExec(workDir, "push", "-u", "origin", "HEAD"); err != nil {
		return false, fmt.Errorf("git push: %w", err)
	}

	return true, nil
}

// ReadWorkspaceFiles reads file contents from the given directory for the
// specified paths. Returns a map of path to content. Files that cannot be read
// are included with empty content. The total content size is capped at maxBytes.
func ReadWorkspaceFiles(dir string, paths []string, maxBytes int) map[string]string {
	result := make(map[string]string, len(paths))
	var totalBytes int

	for _, path := range paths {
		fullPath := filepath.Join(dir, path)
		data, err := os.ReadFile(fullPath) //nolint:gosec // G304: paths are extracted from issue body, not user input
		if err != nil {
			result[path] = ""
			continue
		}

		content := string(data)
		if totalBytes+len(content) > maxBytes {
			result[path] = ""
			continue
		}
		totalBytes += len(content)
		result[path] = content
	}

	return result
}

// gitExec runs a git command in the given directory and returns combined output.
// It strips GIT_DIR and GIT_WORK_TREE from the environment to prevent
// interference when running inside git hooks or other git-managed contexts.
func gitExec(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...) //nolint:gosec // G204: args are internally constructed
	cmd.Dir = dir
	cmd.Env = cleanGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// cleanGitEnv returns the current environment with GIT_DIR and GIT_WORK_TREE
// removed. This prevents git commands from inheriting stale values when running
// inside git hooks or other git-managed contexts.
func cleanGitEnv() []string {
	env := os.Environ()
	clean := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "GIT_DIR=") || strings.HasPrefix(e, "GIT_WORK_TREE=") {
			continue
		}
		clean = append(clean, e)
	}
	return clean
}
