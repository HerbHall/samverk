package agent

import (
	"context"
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

// FetchLatest pulls the latest code from origin into the shared clone and
// resets local main to match. This ensures new worktrees branch from current
// code. Errors are logged as warnings and do not block task execution.
func FetchLatest(repoDir string, logger *zap.Logger) {
	if repoDir == "" {
		return
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	if _, err := gitExec(repoDir, "fetch", "origin", "--prune"); err != nil {
		logger.Warn("fetch origin failed", zap.Error(err))
		return
	}
	// Reset local main to match remote (worktrees branch from HEAD).
	if _, err := gitExec(repoDir, "reset", "--hard", "origin/main"); err != nil {
		logger.Warn("reset to origin/main failed", zap.Error(err))
		return
	}
	// Log HEAD SHA so operators can verify freshness from logs.
	if out, err := gitExec(repoDir, "rev-parse", "--short", "HEAD"); err == nil {
		logger.Info("repo updated", zap.String("head", strings.TrimSpace(out)))
	}
}

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

// ChangedFiles returns the list of files changed in the most recent commit
// relative to the workspace directory.
func ChangedFiles(workDir string) ([]string, error) {
	out, err := gitExec(workDir, "diff", "--name-only", "HEAD~1")
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// CheckDocGate examines a list of changed files and returns true if
// infrastructure or code files were modified without corresponding
// documentation updates. Returns false for test-only or docs-only changes.
func CheckDocGate(changed []string) (needsDoc bool, codeFiles []string) {
	var hasCodeChange, hasDocChange bool
	for _, f := range changed {
		switch {
		case strings.HasSuffix(f, "_test.go"):
			// test-only files don't trigger doc gate
		case strings.HasPrefix(f, "internal/") ||
			strings.HasPrefix(f, "cmd/") ||
			strings.HasPrefix(f, "pkg/") ||
			strings.HasPrefix(f, "deploy/"):
			hasCodeChange = true
			codeFiles = append(codeFiles, f)
		case strings.HasPrefix(f, "docs/") ||
			strings.HasPrefix(f, ".samverk/") ||
			strings.HasSuffix(f, "README.md") ||
			strings.HasSuffix(f, "CLAUDE.md"):
			hasDocChange = true
		}
	}
	return hasCodeChange && !hasDocChange, codeFiles
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
// Uses a generous 60-second timeout per command.
func gitExec(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: args are internally constructed
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
