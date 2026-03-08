package gitea

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	gogitea "code.gitea.io/sdk/gitea"

	"github.com/herbhall/samverk/internal/forge"
)

// Compile-time interface check for RepoReader.
var _ forge.RepoReader = (*Client)(nil)

// ListFiles returns the files and directories at the given path and ref.
// If path is empty, the repository root is listed.
// If ref is empty, the default branch HEAD is used.
func (c *Client) ListFiles(_ context.Context, path, ref string) ([]forge.FileEntry, error) {
	contents, _, err := c.gt.ListContents(c.owner, c.repo, ref, path)
	if err != nil {
		return nil, fmt.Errorf("gitea: list files at %q: %w", path, err)
	}

	entries := make([]forge.FileEntry, 0, len(contents))
	for i := range contents {
		entries = append(entries, forge.FileEntry{
			Name: contents[i].Name,
			Path: contents[i].Path,
			Type: contents[i].Type,
			Size: int(contents[i].Size),
		})
	}

	return entries, nil
}

// ReadFile returns the raw bytes of the file at the given path and ref.
// If ref is empty, the default branch HEAD is used.
func (c *Client) ReadFile(_ context.Context, path, ref string) ([]byte, error) {
	data, _, err := c.gt.GetFile(c.owner, c.repo, ref, path)
	if err != nil {
		return nil, fmt.Errorf("gitea: read file %q: %w", path, err)
	}

	return data, nil
}

// GetDiff returns a human-readable summary comparing base and head refs.
// The Gitea SDK v0.23.2 does not expose a compare endpoint, so this
// fetches branch info for each ref and returns a formatted summary string.
func (c *Client) GetDiff(_ context.Context, base, head string) (string, error) {
	var b strings.Builder

	fmt.Fprintf(&b, "Comparing %s...%s\n", base, head)

	baseBranch, _, err := c.gt.GetRepoBranch(c.owner, c.repo, base)
	if err == nil && baseBranch.Commit != nil {
		fmt.Fprintf(&b, "Base commit: %s\n", baseBranch.Commit.ID)
	}

	headBranch, _, err := c.gt.GetRepoBranch(c.owner, c.repo, head)
	if err == nil && headBranch.Commit != nil {
		fmt.Fprintf(&b, "Head commit: %s\n", headBranch.Commit.ID)
	}

	return b.String(), nil
}

// ListBranches returns all branches in the repository.
func (c *Client) ListBranches(_ context.Context) ([]forge.Branch, error) {
	gtBranches, _, err := c.gt.ListRepoBranches(c.owner, c.repo, gogitea.ListRepoBranchesOptions{
		ListOptions: gogitea.ListOptions{PageSize: 100},
	})
	if err != nil {
		return nil, fmt.Errorf("gitea: list branches: %w", err)
	}

	branches := make([]forge.Branch, 0, len(gtBranches))
	for i := range gtBranches {
		branch := forge.Branch{
			Name: gtBranches[i].Name,
		}
		if gtBranches[i].Commit != nil {
			branch.LastCommit = gtBranches[i].Commit.ID
			branch.UpdatedAt = gtBranches[i].Commit.Timestamp
		}
		branches = append(branches, branch)
	}

	return branches, nil
}

// GetCommitLog returns the most recent commits on a branch.
// If branch is empty, "main" is used. Limit defaults to 20 if <= 0.
func (c *Client) GetCommitLog(_ context.Context, branch string, limit int) ([]forge.Commit, error) {
	if branch == "" {
		branch = "main"
	}
	if limit <= 0 {
		limit = 20
	}

	gtCommits, _, err := c.gt.ListRepoCommits(c.owner, c.repo, gogitea.ListCommitOptions{
		SHA:         branch,
		ListOptions: gogitea.ListOptions{PageSize: limit},
	})
	if err != nil {
		return nil, fmt.Errorf("gitea: list commits on %q: %w", branch, err)
	}

	commits := make([]forge.Commit, 0, len(gtCommits))
	for i := range gtCommits {
		commit := forge.Commit{}
		if gtCommits[i].CommitMeta != nil {
			commit.SHA = gtCommits[i].SHA
			commit.Date = gtCommits[i].Created
		}
		if gtCommits[i].RepoCommit != nil {
			commit.Message = gtCommits[i].RepoCommit.Message
			if gtCommits[i].RepoCommit.Author != nil {
				commit.Author = gtCommits[i].RepoCommit.Author.Name
			}
		}
		commits = append(commits, commit)
	}

	return commits, nil
}

// SearchCode is not supported by the Gitea SDK v0.23.2 at the repo scope.
// It always returns ErrNotImplemented.
func (c *Client) SearchCode(_ context.Context, _ string) ([]forge.SearchResult, error) {
	return []forge.SearchResult{}, ErrNotImplemented
}

// CreateBranch creates a new branch from "main" HEAD.
func (c *Client) CreateBranch(_ context.Context, name string) error {
	_, _, err := c.gt.CreateBranch(c.owner, c.repo, gogitea.CreateBranchOption{
		BranchName:    name,
		OldBranchName: "main",
	})
	if err != nil {
		return fmt.Errorf("gitea: create branch %q: %w", name, err)
	}

	return nil
}

// CreateOrUpdateFile creates or updates a file on the given branch.
// If the file already exists its SHA is fetched first so that the update succeeds.
func (c *Client) CreateOrUpdateFile(_ context.Context, branch, path, content, message string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	existing, _, err := c.gt.GetContents(c.owner, c.repo, branch, path)
	if err == nil && existing != nil {
		// File exists — update it using the existing SHA.
		_, _, updateErr := c.gt.UpdateFile(c.owner, c.repo, path, gogitea.UpdateFileOptions{
			FileOptions: gogitea.FileOptions{
				Message:    message,
				BranchName: branch,
			},
			SHA:     existing.SHA,
			Content: encoded,
		})
		if updateErr != nil {
			return fmt.Errorf("gitea: update file %q on %q: %w", path, branch, updateErr)
		}

		return nil
	}

	// File does not exist — create it.
	_, _, createErr := c.gt.CreateFile(c.owner, c.repo, path, gogitea.CreateFileOptions{
		FileOptions: gogitea.FileOptions{
			Message:    message,
			BranchName: branch,
		},
		Content: encoded,
	})
	if createErr != nil {
		return fmt.Errorf("gitea: create file %q on %q: %w", path, branch, createErr)
	}

	return nil
}
