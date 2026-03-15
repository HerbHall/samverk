package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// mustGit runs a git command in dir and fails the test on error.
// It strips GIT_DIR and GIT_WORK_TREE to avoid interference when
// tests run inside git hooks (e.g. pre-push).
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: test helper
	cmd.Dir = dir
	cmd.Env = cleanGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, string(out))
	}
	return string(out)
}

// setupTestRepo creates a temporary git repo with one commit.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.email", "test@test.com")
	mustGit(t, dir, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-m", "initial")

	return dir
}

// setupTestRepoWithRemote creates a temporary git repo backed by a bare
// "remote" repo so that push operations work.
func setupTestRepoWithRemote(t *testing.T) string {
	t.Helper()

	bareDir := filepath.Join(t.TempDir(), "remote.git")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "init", "--bare", bareDir) //nolint:gosec // G204: test setup
	cmd.Env = cleanGitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}

	repoDir := t.TempDir()
	mustGit(t, repoDir, "init")
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	mustGit(t, repoDir, "remote", "add", "origin", bareDir)

	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	mustGit(t, repoDir, "push", "-u", "origin", "HEAD")

	return repoDir
}

func TestCreateWorkspace(t *testing.T) {
	repoDir := setupTestRepo(t)

	wsPath, cleanup, err := CreateWorkspace(repoDir, "test-session", 42, zap.NewNop())
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	// Verify workspace directory exists.
	if _, statErr := os.Stat(wsPath); statErr != nil {
		t.Fatalf("workspace dir does not exist: %v", statErr)
	}

	// Verify the file from the repo is present in the worktree.
	if _, statErr := os.Stat(filepath.Join(wsPath, "main.go")); statErr != nil {
		t.Fatal("main.go not found in workspace")
	}

	// Verify the branch was created.
	out := mustGit(t, repoDir, "branch", "--list", "agent/42")
	if !strings.Contains(out, "agent/42") {
		t.Errorf("branch agent/42 not found, got: %q", out)
	}

	// Cleanup should remove the directory.
	cleanup()
	if _, statErr := os.Stat(wsPath); !os.IsNotExist(statErr) {
		t.Error("workspace dir should be removed after cleanup")
	}

	// Cleanup should be idempotent.
	cleanup()
}

func TestCreateWorkspace_ExistingBranch(t *testing.T) {
	repoDir := setupTestRepo(t)

	// Pre-create the branch.
	mustGit(t, repoDir, "branch", "agent/99")

	wsPath, cleanup, err := CreateWorkspace(repoDir, "retry-session", 99, zap.NewNop())
	if err != nil {
		t.Fatalf("CreateWorkspace with existing branch: %v", err)
	}
	defer cleanup()

	if _, statErr := os.Stat(wsPath); statErr != nil {
		t.Fatalf("workspace dir does not exist: %v", statErr)
	}
}

func TestReadWorkspaceFiles(t *testing.T) {
	dir := t.TempDir()

	// Create test files.
	if err := os.MkdirAll(filepath.Join(dir, "internal", "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "server", "main.go"), []byte("package server\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := ReadWorkspaceFiles(dir, []string{
		"internal/server/main.go",
		"README.md",
		"nonexistent.go",
	}, 32000)

	if result["internal/server/main.go"] != "package server\n" {
		t.Errorf("server/main.go content = %q, want %q", result["internal/server/main.go"], "package server\n")
	}
	if result["README.md"] != "# Test\n" {
		t.Errorf("README content = %q, want %q", result["README.md"], "# Test\n")
	}
	if result["nonexistent.go"] != "" {
		t.Errorf("nonexistent file should have empty content, got %q", result["nonexistent.go"])
	}
}

func TestReadWorkspaceFiles_MaxBytes(t *testing.T) {
	dir := t.TempDir()

	bigContent := strings.Repeat("x", 1000)
	if err := os.WriteFile(filepath.Join(dir, "big.go"), []byte(bigContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "small.go"), []byte("package small\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := ReadWorkspaceFiles(dir, []string{"big.go", "small.go"}, 500)

	// big.go (1000 bytes) exceeds max, should be empty.
	if result["big.go"] != "" {
		t.Errorf("big.go should be empty (exceeds budget), got %d bytes", len(result["big.go"]))
	}
	// small.go (14 bytes) fits within budget.
	if result["small.go"] != "package small\n" {
		t.Errorf("small.go content = %q, want %q", result["small.go"], "package small\n")
	}
}

func TestCommitAndPush(t *testing.T) {
	repoDir := setupTestRepoWithRemote(t)

	// Create a worktree to test commit+push.
	wsPath, cleanup, err := CreateWorkspace(repoDir, "push-test", 55, zap.NewNop())
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	defer cleanup()

	// No changes — should return false.
	changed, err := CommitAndPush(wsPath, "no changes")
	if err != nil {
		t.Fatalf("CommitAndPush with no changes: %v", err)
	}
	if changed {
		t.Error("expected changed=false for clean workspace")
	}

	// Make a change.
	if writeErr := os.WriteFile(filepath.Join(wsPath, "new.go"), []byte("package main\n"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	changed, err = CommitAndPush(wsPath, "agent: resolve issue #55")
	if err != nil {
		t.Fatalf("CommitAndPush with changes: %v", err)
	}
	if !changed {
		t.Error("expected changed=true after adding file")
	}

	// Verify the commit exists.
	log := mustGit(t, wsPath, "log", "--oneline", "-1")
	if !strings.Contains(log, "agent: resolve issue #55") {
		t.Errorf("commit message not found in log: %q", log)
	}
}

func TestPruneStaleWorktrees(t *testing.T) {
	// Create a fake stale worktree directory.
	staleDir := filepath.Join(os.TempDir(), worktreePrefix+"stale-test-prune")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(staleDir) // safety net

	// Set modification time to 3 hours ago (beyond staleWorktreeAge).
	past := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(staleDir, past, past); err != nil {
		t.Fatal(err)
	}

	PruneStaleWorktrees(zap.NewNop())

	if _, statErr := os.Stat(staleDir); !os.IsNotExist(statErr) {
		t.Error("stale worktree directory should have been removed")
	}
}

func TestPruneStaleWorktrees_KeepsRecent(t *testing.T) {
	// Create a recent worktree directory (should not be pruned).
	recentDir := filepath.Join(os.TempDir(), worktreePrefix+"recent-test-prune")
	if err := os.MkdirAll(recentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(recentDir)

	PruneStaleWorktrees(zap.NewNop())

	if _, statErr := os.Stat(recentDir); statErr != nil {
		t.Error("recent worktree directory should NOT be removed")
	}
}

func TestGitExec(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init")

	out, err := gitExec(dir, "status")
	if err != nil {
		t.Fatalf("gitExec: %v", err)
	}
	if !strings.Contains(out, "On branch") {
		t.Errorf("unexpected git status output: %q", out)
	}
}

func TestGitExec_Error(t *testing.T) {
	dir := t.TempDir() // not a git repo

	_, err := gitExec(dir, "status")
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}

func TestFetchLatest(t *testing.T) {
	repoDir := setupTestRepoWithRemote(t)

	// Add a new commit to the bare remote via a temporary clone so that
	// the local repo is behind.
	tmpClone := t.TempDir()
	bareDir := strings.TrimSpace(mustGit(t, repoDir, "remote", "get-url", "origin"))
	mustGit(t, tmpClone, "clone", bareDir, ".")
	mustGit(t, tmpClone, "config", "user.email", "test@test.com")
	mustGit(t, tmpClone, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(tmpClone, "new.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, tmpClone, "add", ".")
	mustGit(t, tmpClone, "commit", "-m", "remote commit")
	mustGit(t, tmpClone, "push", "origin", "HEAD")

	// Record HEAD before fetch.
	headBefore := strings.TrimSpace(mustGit(t, repoDir, "rev-parse", "HEAD"))

	FetchLatest(repoDir, zap.NewNop())

	// HEAD should have advanced.
	headAfter := strings.TrimSpace(mustGit(t, repoDir, "rev-parse", "HEAD"))
	if headBefore == headAfter {
		t.Error("FetchLatest did not advance HEAD to match remote")
	}

	// The new file should exist after reset.
	if _, err := os.Stat(filepath.Join(repoDir, "new.go")); err != nil {
		t.Error("new.go not found after FetchLatest")
	}
}

func TestFetchLatest_EmptyDir(t *testing.T) {
	// Should be a no-op, not panic.
	FetchLatest("", zap.NewNop())
}

func TestFetchLatest_NilLogger(t *testing.T) {
	// Should not panic with nil logger.
	FetchLatest("", nil)
}

func TestExtractFileContext_WithWorkDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "internal", "agent", "runner.go"),
		[]byte("package agent\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	runner := &Runner{logger: zap.NewNop()}
	body := "Please update internal/agent/runner.go to add workspace support."
	result := runner.extractFileContext(body, dir)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result["internal/agent/runner.go"] != "package agent\n" {
		t.Errorf("runner.go content = %q, want %q", result["internal/agent/runner.go"], "package agent\n")
	}
}

func TestExtractFileContext_NoDir(t *testing.T) {
	runner := &Runner{logger: zap.NewNop()}
	body := "Fix internal/server/main.go and cmd/samverk/root.go"
	result := runner.extractFileContext(body, "")

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Without a directory, content should be empty.
	if result["internal/server/main.go"] != "" {
		t.Errorf("expected empty content without dir, got %q", result["internal/server/main.go"])
	}
	if result["cmd/samverk/root.go"] != "" {
		t.Errorf("expected empty content without dir, got %q", result["cmd/samverk/root.go"])
	}
}
