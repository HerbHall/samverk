package github

import (
	"context"
	"fmt"
	"strings"

	gogithub "github.com/google/go-github/v68/github"

	"github.com/herbhall/samverk/internal/forge"
)

// Compile-time interface checks.
var (
	_ forge.RepoReader = (*Client)(nil)
	_ forge.RepoWriter = (*Client)(nil)
)

// ListFiles returns the files and directories at the given path and ref.
// If path is empty, the repository root is listed.
// If ref is empty, the default branch is used.
func (c *Client) ListFiles(ctx context.Context, path, ref string) ([]forge.FileEntry, error) {
	opts := &gogithub.RepositoryContentGetOptions{}
	if ref != "" {
		opts.Ref = ref
	}

	_, dirContents, resp, err := c.gh.Repositories.GetContents(ctx, c.owner, c.repo, path, opts)
	if err != nil {
		return nil, fmt.Errorf("github: list files at %q: %w", path, err)
	}
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	entries := make([]forge.FileEntry, 0, len(dirContents))
	for i := range dirContents {
		entries = append(entries, forge.FileEntry{
			Name: dirContents[i].GetName(),
			Path: dirContents[i].GetPath(),
			Type: dirContents[i].GetType(),
			Size: dirContents[i].GetSize(),
		})
	}

	return entries, nil
}

// ReadFile returns the contents of the file at the given path and ref.
// If ref is empty, the default branch is used.
// Uses GetContents which returns base64-encoded content (limited to 1 MB files).
func (c *Client) ReadFile(ctx context.Context, path, ref string) ([]byte, error) {
	opts := &gogithub.RepositoryContentGetOptions{}
	if ref != "" {
		opts.Ref = ref
	}

	fileContent, _, resp, err := c.gh.Repositories.GetContents(ctx, c.owner, c.repo, path, opts)
	if err != nil {
		return nil, fmt.Errorf("github: read file %q: %w", path, err)
	}
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	if fileContent == nil {
		return nil, fmt.Errorf("github: %q is a directory, not a file", path)
	}

	content, err := fileContent.GetContent()
	if err != nil {
		return nil, fmt.Errorf("github: decode file %q content: %w", path, err)
	}

	return []byte(content), nil
}

// GetDiff returns the diff between two refs (branches, tags, or commit SHAs).
func (c *Client) GetDiff(ctx context.Context, base, head string) (string, error) {
	comparison, resp, err := c.gh.Repositories.CompareCommits(ctx, c.owner, c.repo, base, head, nil)
	if err != nil {
		return "", fmt.Errorf("github: compare %s...%s: %w", base, head, err)
	}
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Comparing %s...%s\n", base, head)
	fmt.Fprintf(&b, "Status: %s | Ahead by %d | Behind by %d\n",
		comparison.GetStatus(), comparison.GetAheadBy(), comparison.GetBehindBy())
	fmt.Fprintf(&b, "Total commits: %d\n\n", comparison.GetTotalCommits())

	for i := range comparison.Files {
		f := comparison.Files[i]
		fmt.Fprintf(&b, "--- %s\n", f.GetFilename())
		fmt.Fprintf(&b, "    Status: %s | Changes: %d (+%d -%d)\n",
			f.GetStatus(), f.GetChanges(), f.GetAdditions(), f.GetDeletions())
		if patch := f.GetPatch(); patch != "" {
			fmt.Fprintf(&b, "%s\n", patch)
		}
	}

	return b.String(), nil
}

// ListBranches returns all branches in the repository.
func (c *Client) ListBranches(ctx context.Context) ([]forge.Branch, error) {
	ghBranches, resp, err := c.gh.Repositories.ListBranches(ctx, c.owner, c.repo, &gogithub.BranchListOptions{
		ListOptions: gogithub.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, fmt.Errorf("github: list branches: %w", err)
	}
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	branches := make([]forge.Branch, 0, len(ghBranches))
	for i := range ghBranches {
		branch := forge.Branch{
			Name: ghBranches[i].GetName(),
		}
		if commit := ghBranches[i].GetCommit(); commit != nil {
			branch.LastCommit = commit.GetSHA()
		}
		branches = append(branches, branch)
	}

	return branches, nil
}

// GetCommitLog returns the most recent commits on a branch.
// If branch is empty, "main" is used. Limit defaults to 20 if <= 0.
func (c *Client) GetCommitLog(ctx context.Context, branch string, limit int) ([]forge.Commit, error) {
	if branch == "" {
		branch = "main"
	}
	if limit <= 0 {
		limit = 20
	}

	ghCommits, resp, err := c.gh.Repositories.ListCommits(ctx, c.owner, c.repo, &gogithub.CommitsListOptions{
		SHA:         branch,
		ListOptions: gogithub.ListOptions{PerPage: limit},
	})
	if err != nil {
		return nil, fmt.Errorf("github: list commits on %q: %w", branch, err)
	}
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	commits := make([]forge.Commit, 0, len(ghCommits))
	for i := range ghCommits {
		commit := forge.Commit{
			SHA: ghCommits[i].GetSHA(),
		}
		if gc := ghCommits[i].GetCommit(); gc != nil {
			commit.Message = gc.GetMessage()
			if author := gc.GetAuthor(); author != nil {
				commit.Author = author.GetName()
				commit.Date = author.GetDate().Time
			}
		}
		commits = append(commits, commit)
	}

	return commits, nil
}

// CreateBranch creates a new branch from the default branch HEAD.
func (c *Client) CreateBranch(ctx context.Context, name string) error {
	// Get the default branch SHA.
	ref, resp, err := c.gh.Git.GetRef(ctx, c.owner, c.repo, "refs/heads/main")
	if err != nil {
		return fmt.Errorf("github: get main ref: %w", err)
	}
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	newRef := &gogithub.Reference{
		Ref:    gogithub.Ptr("refs/heads/" + name),
		Object: &gogithub.GitObject{SHA: ref.Object.SHA},
	}

	_, resp2, err := c.gh.Git.CreateRef(ctx, c.owner, c.repo, newRef)
	if err != nil {
		return fmt.Errorf("github: create branch %q: %w", name, err)
	}
	if resp2 != nil && resp2.Body != nil {
		defer func() { _ = resp2.Body.Close() }()
	}

	return nil
}

// CreateOrUpdateFile creates or updates a file on the given branch.
func (c *Client) CreateOrUpdateFile(ctx context.Context, branch, path, content, message string) error {
	opts := &gogithub.RepositoryContentFileOptions{
		Message: gogithub.Ptr(message),
		Content: []byte(content),
		Branch:  gogithub.Ptr(branch),
	}

	// Check if file exists to get its SHA for updates.
	existing, _, resp, err := c.gh.Repositories.GetContents(ctx, c.owner, c.repo, path,
		&gogithub.RepositoryContentGetOptions{Ref: branch})
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err == nil && existing != nil {
		opts.SHA = gogithub.Ptr(existing.GetSHA())
	}

	_, resp2, err := c.gh.Repositories.CreateFile(ctx, c.owner, c.repo, path, opts)
	if err != nil {
		return fmt.Errorf("github: create/update file %q on %q: %w", path, branch, err)
	}
	if resp2 != nil && resp2.Body != nil {
		defer func() { _ = resp2.Body.Close() }()
	}

	return nil
}

// SearchCode searches for code matching the query within this repository.
func (c *Client) SearchCode(ctx context.Context, query string) ([]forge.SearchResult, error) {
	scopedQuery := fmt.Sprintf("%s repo:%s/%s", query, c.owner, c.repo)
	searchResult, resp, err := c.gh.Search.Code(ctx, scopedQuery, nil)
	if err != nil {
		return nil, fmt.Errorf("github: search code %q: %w", query, err)
	}
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	results := make([]forge.SearchResult, 0, len(searchResult.CodeResults))
	for i := range searchResult.CodeResults {
		cr := searchResult.CodeResults[i]
		result := forge.SearchResult{
			Path: cr.GetPath(),
		}
		// GitHub code search results include text_matches when requested,
		// but the basic response includes path and name. We return what's available.
		for j := range cr.TextMatches {
			if cr.TextMatches[j] != nil {
				result.Line = cr.TextMatches[j].GetFragment()
				break
			}
		}
		results = append(results, result)
	}

	return results, nil
}
