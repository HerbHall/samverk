package gitea

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// GetDiff returns the unified diff comparing base and head refs.
// It calls the Gitea compare API with the .diff suffix to retrieve actual
// diff content (hunks, file stats). Falls back to a summary if the raw
// diff endpoint is unavailable.
func (c *Client) GetDiff(ctx context.Context, base, head string) (string, error) {
	// Try the raw diff endpoint: GET /repos/:owner/:repo/compare/:base...:head.diff
	diffURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/compare/%s...%s",
		c.baseURL, c.owner, c.repo, base, head)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, diffURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("gitea: create diff request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "text/plain")

	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G704: URL is from trusted baseURL config
	if err != nil {
		return "", fmt.Errorf("gitea: diff request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		// Check Content-Type to determine if we got a diff or JSON.
		ct := resp.Header.Get("Content-Type")
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return "", fmt.Errorf("gitea: read diff response: %w", readErr)
		}

		content := string(body)

		// If the response looks like a unified diff or plain text, return it directly.
		if strings.Contains(ct, "text/plain") || strings.HasPrefix(content, "diff ") {
			return content, nil
		}

		// JSON response from the compare API — extract commit info and file list.
		return c.formatCompareJSON(ctx, content, base, head), nil
	}

	// Fallback: use the SDK's CompareCommits for a summary.
	return c.getDiffFallback(base, head)
}

// compareResponse is a minimal struct for parsing the Gitea compare API JSON.
type compareResponse struct {
	Commits []struct {
		SHA string `json:"sha"`
	} `json:"commits"`
}

// commitFile holds patch information for a single file in a commit.
type commitFile struct {
	Filename  string `json:"filename"`
	Patch     string `json:"patch"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// commitDetail is a minimal struct for the Gitea git/commits/{sha} response.
type commitDetail struct {
	Files []commitFile `json:"files"`
}

// formatCompareJSON extracts commit SHAs from the compare API JSON response,
// fetches the patch content for each commit, and returns a unified diff string.
// If no patches are available, it falls back to a human-readable file stats summary.
func (c *Client) formatCompareJSON(ctx context.Context, body, base, head string) string {
	var compare compareResponse
	if err := json.Unmarshal([]byte(body), &compare); err != nil || len(compare.Commits) == 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "Comparing %s...%s\n", base, head)
		fmt.Fprintf(&b, "(no commits found in range)\n")
		return b.String()
	}

	var patches strings.Builder
	var stats strings.Builder
	hasPatch := false

	for i := range compare.Commits {
		sha := compare.Commits[i].SHA
		if sha == "" {
			continue
		}

		files, err := c.fetchCommitPatch(ctx, sha)
		if err != nil {
			continue
		}

		for j := range files {
			if files[j].Patch != "" {
				patches.WriteString(files[j].Patch)
				if !strings.HasSuffix(files[j].Patch, "\n") {
					patches.WriteByte('\n')
				}
				hasPatch = true
			} else {
				fmt.Fprintf(&stats, "%s (+%d -%d)\n",
					files[j].Filename, files[j].Additions, files[j].Deletions)
			}
		}
	}

	if hasPatch {
		return patches.String()
	}

	// Fallback: no patch content available — return file stats summary.
	var b strings.Builder
	fmt.Fprintf(&b, "Comparing %s...%s\n", base, head)
	if stats.Len() > 0 {
		b.WriteString(stats.String())
	} else {
		fmt.Fprintf(&b, "(no file changes found)\n")
	}
	return b.String()
}

// fetchCommitPatch retrieves file patch content for a single commit SHA
// using the Gitea git/commits/{sha} endpoint.
func (c *Client) fetchCommitPatch(ctx context.Context, sha string) ([]commitFile, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/git/commits/%s",
		c.baseURL, c.owner, c.repo, sha)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("gitea: create commit patch request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G704: URL is from trusted baseURL config
	if err != nil {
		return nil, fmt.Errorf("gitea: fetch commit patch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitea: fetch commit patch %s: status %d", sha, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gitea: read commit patch response: %w", err)
	}

	var detail commitDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("gitea: parse commit patch response: %w", err)
	}

	return detail.Files, nil
}

// getDiffFallback returns a summary when the raw diff endpoint is unavailable.
func (c *Client) getDiffFallback(base, head string) (string, error) {
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
// It returns forge.ErrNotSupported so callers can surface a friendly message.
func (c *Client) SearchCode(_ context.Context, _ string) ([]forge.SearchResult, error) {
	return []forge.SearchResult{}, errNotImplemented
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

// DeleteTestBranch deletes a branch by name. It is NOT part of the forge
// interface and exists solely to support integration test cleanup.
func (c *Client) DeleteTestBranch(_ context.Context, name string) error {
	_, _, err := c.gt.DeleteRepoBranch(c.owner, c.repo, name)
	if err != nil {
		return fmt.Errorf("gitea: delete branch %q: %w", name, err)
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
