package forge

import (
	"context"
	"time"
)

// RepoReader provides read-only access to a git repository's contents.
type RepoReader interface {
	ListFiles(ctx context.Context, path, ref string) ([]FileEntry, error)
	ReadFile(ctx context.Context, path, ref string) ([]byte, error)
	GetDiff(ctx context.Context, base, head string) (string, error)
	ListBranches(ctx context.Context) ([]Branch, error)
	GetCommitLog(ctx context.Context, branch string, limit int) ([]Commit, error)
	SearchCode(ctx context.Context, query string) ([]SearchResult, error)
}

// FileEntry represents a single file or directory in the repository tree.
type FileEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // "file", "dir", "symlink"
	Size int    `json:"size"`
}

// Branch represents a git branch in the repository.
type Branch struct {
	Name       string    `json:"name"`
	LastCommit string    `json:"last_commit"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Commit represents a single commit in the repository log.
type Commit struct {
	SHA     string    `json:"sha"`
	Message string    `json:"message"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
}

// SearchResult represents a single code search match.
type SearchResult struct {
	Path       string `json:"path"`
	LineNumber int    `json:"line_number"`
	Line       string `json:"line"`
}
