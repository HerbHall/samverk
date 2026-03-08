package gitea

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	gogitea "code.gitea.io/sdk/gitea"
)

func TestCreateBranch(t *testing.T) {
	var gotBody gogitea.CreateBranchOption

	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/v1/repos/owner/repo/branches", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeJSON(t, w, http.StatusCreated, &gogitea.Branch{
			Name: gotBody.BranchName,
		})
	})

	c, _ := newTestClient(t, handler)

	err := c.CreateBranch(context.Background(), "feature/new-branch")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	if gotBody.BranchName != "feature/new-branch" {
		t.Errorf("BranchName = %q, want %q", gotBody.BranchName, "feature/new-branch")
	}
	if gotBody.OldBranchName != "main" {
		t.Errorf("OldBranchName = %q, want %q", gotBody.OldBranchName, "main")
	}
}

func TestCreateOrUpdateFile_Create(t *testing.T) {
	// GetContents returns 404 (file does not exist), so CreateFile should be called.
	var gotCreateBody gogitea.CreateFileOptions

	handler := http.NewServeMux()
	// GetContents: 404 — file not found.
	handler.HandleFunc("GET /api/v1/repos/owner/repo/contents/docs/new.md", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	// CreateFile.
	handler.HandleFunc("POST /api/v1/repos/owner/repo/contents/docs/new.md", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotCreateBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeJSON(t, w, http.StatusCreated, &gogitea.FileResponse{})
	})

	c, _ := newTestClient(t, handler)

	err := c.CreateOrUpdateFile(context.Background(), "main", "docs/new.md", "# New File", "docs: add new.md")
	if err != nil {
		t.Fatalf("CreateOrUpdateFile (create): %v", err)
	}

	if gotCreateBody.FileOptions.Message != "docs: add new.md" {
		t.Errorf("Message = %q, want %q", gotCreateBody.FileOptions.Message, "docs: add new.md")
	}
	if gotCreateBody.FileOptions.BranchName != "main" {
		t.Errorf("BranchName = %q, want %q", gotCreateBody.FileOptions.BranchName, "main")
	}

	decoded, err := base64.StdEncoding.DecodeString(gotCreateBody.Content)
	if err != nil {
		t.Fatalf("decode content: %v", err)
	}
	if string(decoded) != "# New File" {
		t.Errorf("content = %q, want %q", string(decoded), "# New File")
	}
}

func TestCreateOrUpdateFile_Update(t *testing.T) {
	// GetContents returns the file with a SHA, so UpdateFile should be called.
	const existingSHA = "abc123def456"
	var gotUpdateBody gogitea.UpdateFileOptions

	handler := http.NewServeMux()
	// GetContents: file exists.
	handler.HandleFunc("GET /api/v1/repos/owner/repo/contents/README.md", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogitea.ContentsResponse{
			Name: "README.md",
			Path: "README.md",
			Type: "file",
			SHA:  existingSHA,
		})
	})
	// UpdateFile.
	handler.HandleFunc("PUT /api/v1/repos/owner/repo/contents/README.md", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotUpdateBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeJSON(t, w, http.StatusOK, &gogitea.FileResponse{})
	})

	c, _ := newTestClient(t, handler)

	err := c.CreateOrUpdateFile(context.Background(), "main", "README.md", "updated content", "docs: update README")
	if err != nil {
		t.Fatalf("CreateOrUpdateFile (update): %v", err)
	}

	if gotUpdateBody.SHA != existingSHA {
		t.Errorf("SHA = %q, want %q", gotUpdateBody.SHA, existingSHA)
	}
	if gotUpdateBody.FileOptions.Message != "docs: update README" {
		t.Errorf("Message = %q, want %q", gotUpdateBody.FileOptions.Message, "docs: update README")
	}
	if gotUpdateBody.FileOptions.BranchName != "main" {
		t.Errorf("BranchName = %q, want %q", gotUpdateBody.FileOptions.BranchName, "main")
	}

	decoded, err := base64.StdEncoding.DecodeString(gotUpdateBody.Content)
	if err != nil {
		t.Fatalf("decode content: %v", err)
	}
	if string(decoded) != "updated content" {
		t.Errorf("content = %q, want %q", string(decoded), "updated content")
	}
}

func TestListFiles(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v1/repos/owner/repo/contents/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogitea.ContentsResponse{
			{Name: "main.go", Path: "main.go", Type: "file", Size: 1024},
			{Name: "internal", Path: "internal", Type: "dir", Size: 0},
		})
	})

	c, _ := newTestClient(t, handler)

	entries, err := c.ListFiles(context.Background(), "", "main")
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
	if entries[0].Size != 1024 {
		t.Errorf("entries[0].Size = %d, want 1024", entries[0].Size)
	}
	if entries[1].Name != "internal" {
		t.Errorf("entries[1].Name = %q, want %q", entries[1].Name, "internal")
	}
	if entries[1].Type != "dir" {
		t.Errorf("entries[1].Type = %q, want %q", entries[1].Type, "dir")
	}
}

func TestReadFile(t *testing.T) {
	fileContent := []byte("package main\n\nfunc main() {}\n")

	handler := http.NewServeMux()
	// Gitea SDK >= 1.14 uses /repos/{owner}/{repo}/raw/{filepath}?ref={ref}
	// (older versions used /{ref}/{filepath} in the path).
	handler.HandleFunc("GET /api/v1/repos/owner/repo/raw/cmd/main.go", func(w http.ResponseWriter, r *http.Request) {
		ref := r.URL.Query().Get("ref")
		if ref != "main" {
			t.Errorf("ref query param = %q, want %q", ref, "main")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fileContent)
	})

	c, _ := newTestClient(t, handler)

	data, err := c.ReadFile(context.Background(), "cmd/main.go", "main")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != string(fileContent) {
		t.Errorf("content = %q, want %q", string(data), string(fileContent))
	}
}

func TestListBranches(t *testing.T) {
	ts := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v1/repos/owner/repo/branches", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []*gogitea.Branch{
			{
				Name: "main",
				Commit: &gogitea.PayloadCommit{
					ID:        "abc123def456",
					Timestamp: ts,
				},
			},
			{
				Name: "feature/test",
				Commit: &gogitea.PayloadCommit{
					ID:        "789ghi012jkl",
					Timestamp: ts.Add(-24 * time.Hour),
				},
			},
		})
	})

	c, _ := newTestClient(t, handler)

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
	if !branches[0].UpdatedAt.Equal(ts) {
		t.Errorf("branches[0].UpdatedAt = %v, want %v", branches[0].UpdatedAt, ts)
	}
	if branches[1].Name != "feature/test" {
		t.Errorf("branches[1].Name = %q, want %q", branches[1].Name, "feature/test")
	}
}

func TestGetCommitLog(t *testing.T) {
	commitDate := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v1/repos/owner/repo/commits", func(w http.ResponseWriter, r *http.Request) {
		sha := r.URL.Query().Get("sha")
		if sha != "main" {
			t.Errorf("sha param = %q, want %q", sha, "main")
		}

		writeJSON(t, w, http.StatusOK, []*gogitea.Commit{
			{
				CommitMeta: &gogitea.CommitMeta{
					SHA:     "abc123",
					Created: commitDate,
				},
				RepoCommit: &gogitea.RepoCommit{
					Message: "feat: add feature",
					Author: &gogitea.CommitUser{
						Identity: gogitea.Identity{Name: "Developer"},
					},
				},
			},
			{
				CommitMeta: &gogitea.CommitMeta{
					SHA:     "def456",
					Created: commitDate.Add(-time.Hour),
				},
				RepoCommit: &gogitea.RepoCommit{
					Message: "fix: bug fix",
					Author: &gogitea.CommitUser{
						Identity: gogitea.Identity{Name: "Developer"},
					},
				},
			},
		})
	})

	c, _ := newTestClient(t, handler)

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
	if !commits[0].Date.Equal(commitDate) {
		t.Errorf("commits[0].Date = %v, want %v", commits[0].Date, commitDate)
	}
}

func TestGetCommitLog_DefaultBranch(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v1/repos/owner/repo/commits", func(w http.ResponseWriter, r *http.Request) {
		sha := r.URL.Query().Get("sha")
		if sha != "main" {
			t.Errorf("sha param = %q, want %q (default)", sha, "main")
		}
		writeJSON(t, w, http.StatusOK, []*gogitea.Commit{})
	})

	c, _ := newTestClient(t, handler)

	_, err := c.GetCommitLog(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("GetCommitLog with defaults: %v", err)
	}
}

func TestGetDiff(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v1/repos/owner/repo/branches/main", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogitea.Branch{
			Name: "main",
			Commit: &gogitea.PayloadCommit{
				ID: "base-sha-111",
			},
		})
	})
	handler.HandleFunc("GET /api/v1/repos/owner/repo/branches/feature", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, &gogitea.Branch{
			Name: "feature",
			Commit: &gogitea.PayloadCommit{
				ID: "head-sha-222",
			},
		})
	})

	c, _ := newTestClient(t, handler)

	diff, err := c.GetDiff(context.Background(), "main", "feature")
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff summary")
	}
	if !strings.Contains(diff, "main") {
		t.Errorf("diff missing base ref, got %q", diff)
	}
	if !strings.Contains(diff, "feature") {
		t.Errorf("diff missing head ref, got %q", diff)
	}
	if !strings.Contains(diff, "base-sha-111") {
		t.Errorf("diff missing base commit SHA, got %q", diff)
	}
	if !strings.Contains(diff, "head-sha-222") {
		t.Errorf("diff missing head commit SHA, got %q", diff)
	}
}

func TestSearchCode_NotImplemented(t *testing.T) {
	handler := http.NewServeMux()
	c, _ := newTestClient(t, handler)

	results, err := c.SearchCode(context.Background(), "some query")
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("err = %v, want ErrNotImplemented", err)
	}
	if results == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}
