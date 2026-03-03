package github

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"

	gogithub "github.com/google/go-github/v68/github"
)

// base64Encode is a test helper for encoding content as base64.
func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func TestListFiles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/contents/cmd", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogithub.RepositoryContent{
			{
				Name: gogithub.Ptr("main.go"),
				Path: gogithub.Ptr("cmd/main.go"),
				Type: gogithub.Ptr("file"),
				Size: gogithub.Ptr(1024),
			},
			{
				Name: gogithub.Ptr("util"),
				Path: gogithub.Ptr("cmd/util"),
				Type: gogithub.Ptr("dir"),
				Size: gogithub.Ptr(0),
			},
		})
	})

	c, _ := newTestClient(t, mux)

	entries, err := c.ListFiles(context.Background(), "cmd", "")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].Name != "main.go" {
		t.Errorf("entries[0].Name = %q, want %q", entries[0].Name, "main.go")
	}
	if entries[0].Type != "file" {
		t.Errorf("entries[0].Type = %q, want %q", entries[0].Type, "file")
	}
	if entries[1].Name != "util" {
		t.Errorf("entries[1].Name = %q, want %q", entries[1].Name, "util")
	}
	if entries[1].Type != "dir" {
		t.Errorf("entries[1].Type = %q, want %q", entries[1].Type, "dir")
	}
}

func TestListFiles_WithRef(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/contents/", func(w http.ResponseWriter, r *http.Request) {
		ref := r.URL.Query().Get("ref")
		if ref != "feature-branch" {
			t.Errorf("ref param = %q, want %q", ref, "feature-branch")
		}
		writeJSON(t, w, http.StatusOK, []*gogithub.RepositoryContent{
			{
				Name: gogithub.Ptr("README.md"),
				Path: gogithub.Ptr("README.md"),
				Type: gogithub.Ptr("file"),
				Size: gogithub.Ptr(512),
			},
		})
	})

	c, _ := newTestClient(t, mux)

	entries, err := c.ListFiles(context.Background(), "", "feature-branch")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
}

func TestReadFile(t *testing.T) {
	fileContent := "package main\n\nfunc main() {}\n"
	encoded := base64Encode(fileContent)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/contents/main.go", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.RepositoryContent{
			Name:     gogithub.Ptr("main.go"),
			Path:     gogithub.Ptr("main.go"),
			Type:     gogithub.Ptr("file"),
			Encoding: gogithub.Ptr("base64"),
			Content:  gogithub.Ptr(encoded),
		})
	})

	c, _ := newTestClient(t, mux)

	data, err := c.ReadFile(context.Background(), "main.go", "")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != fileContent {
		t.Errorf("content = %q, want %q", string(data), fileContent)
	}
}

func TestGetDiff(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/compare/main...feature", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogithub.CommitsComparison{
			Status:       gogithub.Ptr("ahead"),
			AheadBy:      gogithub.Ptr(2),
			BehindBy:     gogithub.Ptr(0),
			TotalCommits: gogithub.Ptr(2),
			Files: []*gogithub.CommitFile{
				{
					Filename:  gogithub.Ptr("internal/mcp/handler.go"),
					Status:    gogithub.Ptr("modified"),
					Changes:   gogithub.Ptr(10),
					Additions: gogithub.Ptr(8),
					Deletions: gogithub.Ptr(2),
					Patch:     gogithub.Ptr("@@ -1,5 +1,13 @@\n+new code"),
				},
			},
		})
	})

	c, _ := newTestClient(t, mux)

	diff, err := c.GetDiff(context.Background(), "main", "feature")
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(diff, "ahead") {
		t.Errorf("diff missing status, got %q", diff)
	}
	if !strings.Contains(diff, "handler.go") {
		t.Errorf("diff missing filename, got %q", diff)
	}
}

func TestListBranches(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/branches", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogithub.Branch{
			{
				Name: gogithub.Ptr("main"),
				Commit: &gogithub.RepositoryCommit{
					SHA: gogithub.Ptr("abc123def456"),
				},
			},
			{
				Name: gogithub.Ptr("feature/test"),
				Commit: &gogithub.RepositoryCommit{
					SHA: gogithub.Ptr("789ghi012jkl"),
				},
			},
		})
	})

	c, _ := newTestClient(t, mux)

	branches, err := c.ListBranches(context.Background())
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 2 {
		t.Fatalf("len(branches) = %d, want 2", len(branches))
	}
	if branches[0].Name != "main" {
		t.Errorf("branches[0].Name = %q, want %q", branches[0].Name, "main")
	}
	if branches[0].LastCommit != "abc123def456" {
		t.Errorf("branches[0].LastCommit = %q, want %q", branches[0].LastCommit, "abc123def456")
	}
	if branches[1].Name != "feature/test" {
		t.Errorf("branches[1].Name = %q, want %q", branches[1].Name, "feature/test")
	}
}

func TestGetCommitLog(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/commits", func(w http.ResponseWriter, r *http.Request) {
		sha := r.URL.Query().Get("sha")
		if sha != "main" {
			t.Errorf("sha param = %q, want %q", sha, "main")
		}
		perPage := r.URL.Query().Get("per_page")
		if perPage != "5" {
			t.Errorf("per_page param = %q, want %q", perPage, "5")
		}

		commitDate := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
		writeJSON(t, w, http.StatusOK, []*gogithub.RepositoryCommit{
			{
				SHA: gogithub.Ptr("abc123"),
				Commit: &gogithub.Commit{
					Message: gogithub.Ptr("feat: add feature"),
					Author: &gogithub.CommitAuthor{
						Name: gogithub.Ptr("Developer"),
						Date: &gogithub.Timestamp{Time: commitDate},
					},
				},
			},
			{
				SHA: gogithub.Ptr("def456"),
				Commit: &gogithub.Commit{
					Message: gogithub.Ptr("fix: bug fix"),
					Author: &gogithub.CommitAuthor{
						Name: gogithub.Ptr("Developer"),
						Date: &gogithub.Timestamp{Time: commitDate.Add(-time.Hour)},
					},
				},
			},
		})
	})

	c, _ := newTestClient(t, mux)

	commits, err := c.GetCommitLog(context.Background(), "main", 5)
	if err != nil {
		t.Fatalf("GetCommitLog: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("len(commits) = %d, want 2", len(commits))
	}
	if commits[0].SHA != "abc123" {
		t.Errorf("commits[0].SHA = %q, want %q", commits[0].SHA, "abc123")
	}
	if commits[0].Message != "feat: add feature" {
		t.Errorf("commits[0].Message = %q, want %q", commits[0].Message, "feat: add feature")
	}
	if commits[0].Author != "Developer" {
		t.Errorf("commits[0].Author = %q, want %q", commits[0].Author, "Developer")
	}
}

func TestGetCommitLog_DefaultBranch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/commits", func(w http.ResponseWriter, r *http.Request) {
		sha := r.URL.Query().Get("sha")
		if sha != "main" {
			t.Errorf("sha param = %q, want %q (default)", sha, "main")
		}
		writeJSON(t, w, http.StatusOK, []*gogithub.RepositoryCommit{})
	})

	c, _ := newTestClient(t, mux)

	_, err := c.GetCommitLog(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("GetCommitLog: %v", err)
	}
}

func TestSearchCode(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /search/code", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			t.Error("expected non-empty query")
		}
		writeJSON(t, w, http.StatusOK, &gogithub.CodeSearchResult{
			Total: gogithub.Ptr(1),
			CodeResults: []*gogithub.CodeResult{
				{
					Path: gogithub.Ptr("internal/mcp/handler.go"),
					TextMatches: []*gogithub.TextMatch{
						{
							Fragment: gogithub.Ptr("func NewHandler(tracker forge.IssueTracker)"),
						},
					},
				},
			},
		})
	})

	c, _ := newTestClient(t, mux)

	results, err := c.SearchCode(context.Background(), "NewHandler")
	if err != nil {
		t.Fatalf("SearchCode: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Path != "internal/mcp/handler.go" {
		t.Errorf("results[0].Path = %q, want %q", results[0].Path, "internal/mcp/handler.go")
	}
	if results[0].Line == "" {
		t.Error("expected non-empty line fragment")
	}
}

