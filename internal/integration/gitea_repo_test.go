//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	giteaadapter "samverk.dev/samverk/internal/forge/gitea"
	"samverk.dev/samverk/internal/forge"
)

// envOrDefault returns the value of the environment variable key,
// or def if the variable is unset or empty.
func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

// giteaTestClient creates a Client for the real Gitea test instance.
// It skips the test if GITEA_TOKEN is not set.
func giteaTestClient(t *testing.T) *giteaadapter.Client {
	t.Helper()

	giteaURL := envOrDefault("GITEA_URL", "http://192.168.1.160:3000")
	owner := envOrDefault("GITEA_OWNER", "samverk")
	repo := envOrDefault("GITEA_REPO", "samverk-test")
	token := os.Getenv("GITEA_TOKEN")

	if token == "" {
		t.Skip("GITEA_TOKEN not set; skipping integration test")
	}

	client, err := giteaadapter.New(giteaURL, token, owner, repo)
	if err != nil {
		t.Fatalf("giteaadapter.New: %v", err)
	}

	return client
}

// uniqueBranch returns a unique branch name for a test.
func uniqueBranch() string {
	return fmt.Sprintf("integration-test-%d", time.Now().UnixNano())
}

// TestRepoWriter_CreateBranch exercises CreateBranch against the real Gitea instance.
// It creates a unique branch, verifies it appears in ListBranches, then cleans up.
func TestRepoWriter_CreateBranch(t *testing.T) {
	client := giteaTestClient(t)
	ctx := context.Background()

	branchName := uniqueBranch()

	if err := client.CreateBranch(ctx, branchName); err != nil {
		t.Fatalf("CreateBranch(%q): %v", branchName, err)
	}

	t.Cleanup(func() {
		_ = client.DeleteTestBranch(context.Background(), branchName)
	})

	branches, err := client.ListBranches(ctx)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}

	var found bool
	for _, b := range branches {
		if b.Name == branchName {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("branch %q not found in ListBranches after creation", branchName)
	}
}

// TestRepoWriter_CreateOrUpdateFile exercises create-then-update via the real Gitea API.
// It creates a branch, writes a file, verifies content, updates the file, verifies again.
func TestRepoWriter_CreateOrUpdateFile(t *testing.T) {
	client := giteaTestClient(t)
	ctx := context.Background()

	branchName := uniqueBranch()
	filePath := "integration-test-file.md"

	if err := client.CreateBranch(ctx, branchName); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	t.Cleanup(func() {
		_ = client.DeleteTestBranch(context.Background(), branchName)
	})

	// Create.
	initialContent := "# Integration Test\n\nInitial content."
	if err := client.CreateOrUpdateFile(ctx, branchName, filePath, initialContent, "test: create file"); err != nil {
		t.Fatalf("CreateOrUpdateFile (create): %v", err)
	}

	data, err := client.ReadFile(ctx, filePath, branchName)
	if err != nil {
		t.Fatalf("ReadFile after create: %v", err)
	}

	if string(data) != initialContent {
		t.Errorf("content after create = %q, want %q", string(data), initialContent)
	}

	// Update.
	updatedContent := "# Integration Test\n\nUpdated content."
	if err := client.CreateOrUpdateFile(ctx, branchName, filePath, updatedContent, "test: update file"); err != nil {
		t.Fatalf("CreateOrUpdateFile (update): %v", err)
	}

	data, err = client.ReadFile(ctx, filePath, branchName)
	if err != nil {
		t.Fatalf("ReadFile after update: %v", err)
	}

	if string(data) != updatedContent {
		t.Errorf("content after update = %q, want %q", string(data), updatedContent)
	}
}

// TestRepoReader_ListFiles verifies ListFiles returns entries for the repo root on main.
func TestRepoReader_ListFiles(t *testing.T) {
	client := giteaTestClient(t)
	ctx := context.Background()

	entries, err := client.ListFiles(ctx, "", "main")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("ListFiles returned 0 entries at repo root")
	}

	for _, e := range entries {
		if e.Name == "" {
			t.Errorf("entry has empty Name: %+v", e)
		}

		if e.Type != "file" && e.Type != "dir" {
			t.Errorf("entry %q has unexpected Type %q (want 'file' or 'dir')", e.Name, e.Type)
		}
	}
}

// TestRepoReader_ReadFile reads README.md from the test repo.
func TestRepoReader_ReadFile(t *testing.T) {
	client := giteaTestClient(t)
	ctx := context.Background()

	data, err := client.ReadFile(ctx, "README.md", "main")
	if err != nil {
		t.Fatalf("ReadFile(README.md): %v", err)
	}

	if len(data) == 0 {
		t.Fatal("ReadFile(README.md) returned empty content")
	}
}

// TestRepoReader_ListBranches verifies that main branch appears in the branch list.
func TestRepoReader_ListBranches(t *testing.T) {
	client := giteaTestClient(t)

	branches, err := client.ListBranches(context.Background())
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}

	var hasMain bool
	for _, b := range branches {
		if b.Name == "main" {
			hasMain = true

			if b.LastCommit == "" {
				t.Error("main branch has empty LastCommit")
			}

			if b.UpdatedAt.IsZero() {
				t.Error("main branch has zero UpdatedAt")
			}
		}
	}

	if !hasMain {
		t.Error("'main' branch not found in ListBranches")
	}
}

// TestRepoReader_GetCommitLog verifies commits are returned for main.
func TestRepoReader_GetCommitLog(t *testing.T) {
	client := giteaTestClient(t)

	commits, err := client.GetCommitLog(context.Background(), "main", 5)
	if err != nil {
		t.Fatalf("GetCommitLog: %v", err)
	}

	if len(commits) == 0 {
		t.Fatal("GetCommitLog returned 0 commits")
	}

	for _, c := range commits {
		if c.SHA == "" {
			t.Errorf("commit has empty SHA: %+v", c)
		}

		if c.Date.IsZero() {
			t.Errorf("commit %q has zero Date", c.SHA)
		}
	}
}

// TestRepoReader_GetDiff verifies GetDiff returns a non-empty summary.
func TestRepoReader_GetDiff(t *testing.T) {
	client := giteaTestClient(t)
	ctx := context.Background()

	diff, err := client.GetDiff(ctx, "main", "main")
	if err != nil {
		t.Fatalf("GetDiff(main, main): %v", err)
	}

	if !strings.Contains(diff, "main") {
		t.Errorf("GetDiff output missing 'main': %q", diff)
	}
}

// TestPRManager_FullLifecycle exercises the complete PR lifecycle:
// create branch → write file → open PR → get PR → check CI status → merge.
func TestPRManager_FullLifecycle(t *testing.T) {
	client := giteaTestClient(t)
	ctx := context.Background()

	branchName := uniqueBranch()

	// 1. Create branch.
	if err := client.CreateBranch(ctx, branchName); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	t.Cleanup(func() {
		// Best-effort: the branch may already be gone after merge.
		_ = client.DeleteTestBranch(context.Background(), branchName)
	})

	// 2. Write a file so the PR has a diff.
	content := fmt.Sprintf("# PR Lifecycle Test\n\nTimestamp: %s", time.Now().UTC().Format(time.RFC3339))
	if err := client.CreateOrUpdateFile(ctx, branchName, "integration-pr-test.md", content, "test: add PR lifecycle file"); err != nil {
		t.Fatalf("CreateOrUpdateFile: %v", err)
	}

	// 3. Open PR.
	pr, err := client.CreatePullRequest(ctx, &forge.CreatePRRequest{
		Title: "Integration test PR",
		Body:  "Created by TestPRManager_FullLifecycle. Auto-merging.",
		Head:  branchName,
		Base:  "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}

	if pr.Number == 0 {
		t.Fatal("CreatePullRequest returned PR with Number=0")
	}

	t.Logf("created PR #%d", pr.Number)

	// 4. GetPullRequest.
	got, err := client.GetPullRequest(ctx, pr.Number)
	if err != nil {
		t.Fatalf("GetPullRequest(%d): %v", pr.Number, err)
	}

	if got.Head != branchName {
		t.Errorf("GetPullRequest.Head = %q, want %q", got.Head, branchName)
	}

	if got.Base != "main" {
		t.Errorf("GetPullRequest.Base = %q, want %q", got.Base, "main")
	}

	// 5. GetPRChecks — test repo has no CI so empty or single combined:pending is acceptable.
	checks, err := client.GetPRChecks(ctx, pr.Number)
	if err != nil {
		t.Fatalf("GetPRChecks(%d): %v", pr.Number, err)
	}

	t.Logf("GetPRChecks returned %d check(s)", len(checks))

	// 6. ListPullRequests — the new PR should appear in the open list.
	prs, err := client.ListPullRequests(ctx, &forge.ListPROptions{State: "open", PerPage: 50})
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}

	var foundInList bool
	for _, p := range prs {
		if p.Number == pr.Number {
			foundInList = true

			break
		}
	}

	if !foundInList {
		t.Errorf("PR #%d not found in ListPullRequests(open)", pr.Number)
	}

	// 7. MergePullRequest.
	if err := client.MergePullRequest(ctx, pr.Number, forge.MergeMethodMerge, "test: merge PR lifecycle integration test"); err != nil {
		t.Fatalf("MergePullRequest(%d): %v", pr.Number, err)
	}

	// 8. Verify PR is closed.
	merged, err := client.GetPullRequest(ctx, pr.Number)
	if err != nil {
		t.Fatalf("GetPullRequest after merge: %v", err)
	}

	if string(merged.State) != "closed" {
		t.Errorf("PR state after merge = %q, want closed", merged.State)
	}
}
